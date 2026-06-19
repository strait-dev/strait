package billing

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"strait/internal/domain"
)

// UptimeSource resolves the platform uptime percentage observed by an
// org over a billing period. Implementations may pull from health-check
// history, an external monitoring vendor, or a synthetic probe. Returning
// (uptime, nil) where uptime is outside [0, 100] is tolerated; the
// calculator clamps it before applying the SLA bands.
type UptimeSource interface {
	MonthlyUptimePct(ctx context.Context, orgID string, periodStart, periodEnd time.Time) (float64, error)
}

// StaticUptimeSource is a placeholder UptimeSource that always returns the
// same percentage. Used to wire the calculator end-to-end before a real
// uptime feed (Sift / Prometheus / health-history) is plumbed in.
type StaticUptimeSource struct {
	pct float64
}

// NewStaticUptimeSource constructs a StaticUptimeSource that reports pct.
func NewStaticUptimeSource(pct float64) *StaticUptimeSource {
	return &StaticUptimeSource{pct: pct}
}

// MonthlyUptimePct returns the configured fixed uptime.
func (s *StaticUptimeSource) MonthlyUptimePct(_ context.Context, _ string, _, _ time.Time) (float64, error) {
	return s.pct, nil
}

// SLACreditIssuer issues a Stripe credit note (or customer-balance
// adjustment) for the org. Returning a non-empty credit-note ID lets the
// calculator persist the link to the Stripe object for the
// finance/audit trail; an empty string is treated as "issued without a
// Stripe-side artifact" (e.g. operator escape hatch).
type SLACreditIssuer interface {
	IssueCredit(ctx context.Context, orgID string, creditMicrousd int64, periodEnd time.Time) (creditNoteID string, err error)
}

// CustomerLookupStore is the narrow data-access surface a Stripe-backed
// SLACreditIssuer needs to resolve an org to its Stripe customer. Lives
// here (not in stripe_sla_issuer_cloud.go) so both editions can share
// the type without dragging stripe-go into community builds.
type CustomerLookupStore interface {
	GetOrgSubscription(ctx context.Context, orgID string) (*OrgSubscription, error)
}

// SLACalculatorStore is the data-access surface the calculator needs.
// Kept as a single interface so tests can stub the contract listing and
// credit persistence with one mock.
type SLACalculatorStore interface {
	SLACreditStore
	ListEnterpriseContractsActiveAt(ctx context.Context, at time.Time) ([]EnterpriseContract, error)
	ListEnterpriseContractsOverlappingPeriod(ctx context.Context, periodStart, periodEnd time.Time) ([]EnterpriseContract, error)
}

// SLACalculator runs the periodic SLA credit pipeline: for each
// enterprise contract whose monthly uptime fell below the tier's SLA
// target, issue a credit (Stripe-side, when an issuer is wired) and
// dispatch sla.credit_issued.
type SLACalculator struct {
	store      SLACalculatorStore
	uptime     UptimeSource
	issuer     SLACreditIssuer
	dispatcher BillingEventDispatcher
	dispatchMu sync.Mutex
	interval   time.Duration
	logger     *slog.Logger
	clock      func() time.Time
}

// NewSLACalculator wires the calculator. The interval typically lands on
// ~24h: each tick is idempotent against the unique constraint on
// sla_credits, so the cadence only controls how soon a missed run gets
// retried.
func NewSLACalculator(store SLACalculatorStore, uptime UptimeSource, interval time.Duration, logger *slog.Logger) *SLACalculator {
	if logger == nil {
		logger = slog.Default()
	}
	return &SLACalculator{
		store:    store,
		uptime:   uptime,
		interval: interval,
		logger:   logger,
		clock:    time.Now,
	}
}

// WithIssuer plumbs a Stripe-credit-note issuer. When nil, the calculator
// still records the credit row + dispatches sla.credit_issued but skips
// any Stripe-side write (useful for self-hosted/community builds and for
// the operator escape hatch).
func (c *SLACalculator) WithIssuer(issuer SLACreditIssuer) *SLACalculator {
	c.issuer = issuer
	return c
}

// WithDispatcher plumbs the outbound billing webhook dispatcher used to
// emit sla.credit_issued events.
func (c *SLACalculator) WithDispatcher(d BillingEventDispatcher) *SLACalculator {
	c.dispatcher = d
	return c
}

// WithClock overrides time.Now for deterministic tests.
func (c *SLACalculator) WithClock(clock func() time.Time) *SLACalculator {
	c.clock = clock
	return c
}

// Run is the scheduler entrypoint: ticks once on the configured interval.
// Per-tick errors are logged; the loop never exits on its own.
func (c *SLACalculator) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.Tick(ctx); err != nil {
				c.logger.Warn("sla calculator tick failed", "error", err)
			}
		}
	}
}

// Tick is one iteration: list active enterprise contracts, compute the
// previous calendar month's uptime per org, and issue credit for any
// org that breached its SLA target. Idempotent against the
// (org_id, period_start, period_end) unique key on sla_credits.
func (c *SLACalculator) Tick(ctx context.Context) error {
	now := c.clock()
	periodStart, periodEnd := previousCalendarMonth(now)

	contracts, err := c.store.ListEnterpriseContractsOverlappingPeriod(ctx, periodStart, periodEnd)
	if err != nil {
		return err
	}

	for _, contract := range contracts {
		c.processContract(ctx, contract, periodStart, periodEnd, now)
	}
	return nil
}

// processContract isolates the per-org pipeline so a single failure does
// not block the rest of the batch. All errors are logged and swallowed.
func (c *SLACalculator) processContract(ctx context.Context, contract EnterpriseContract, periodStart, periodEnd, now time.Time) {
	cfg := GetEnterpriseConfig(contract.EnterpriseTier)
	if cfg.UptimeSLAPct <= 0 {
		return
	}

	existing, err := c.store.GetSLACredit(ctx, contract.OrgID, periodStart, periodEnd)
	if err != nil {
		c.logger.Warn("sla credit lookup failed", "org_id", contract.OrgID, "error", err)
		return
	}
	if existing != nil {
		if c.dispatcher != nil && existing.WebhookDispatchedAt == nil {
			c.dispatchSLACreditWebhook(ctx, contract.OrgID, *existing)
		}
		return
	}

	uptime, err := c.uptime.MonthlyUptimePct(ctx, contract.OrgID, periodStart, periodEnd)
	if err != nil {
		c.logger.Warn("uptime read failed", "org_id", contract.OrgID, "error", err)
		return
	}
	uptime = clampUptime(uptime)

	creditPct := CalculateSLACredit(uptime, cfg.UptimeSLAPct)
	if creditPct == 0 {
		return
	}

	creditMicrousd := monthlyCreditMicrousd(contract.AnnualCommitmentCents, creditPct)
	if creditMicrousd <= 0 {
		return
	}

	var creditNoteID string
	if c.issuer != nil {
		id, issueErr := c.issuer.IssueCredit(ctx, contract.OrgID, creditMicrousd, periodEnd)
		if issueErr != nil {
			c.logger.Warn("sla credit issuance failed; not persisting", "org_id", contract.OrgID, "error", issueErr)
			return
		}
		creditNoteID = id
	}

	row := SLACreditRow{
		ID:                 uuid.Must(uuid.NewV7()).String(),
		OrgID:              contract.OrgID,
		PeriodStart:        periodStart,
		PeriodEnd:          periodEnd,
		UptimePct:          uptime,
		TargetPct:          cfg.UptimeSLAPct,
		CreditPct:          creditPct,
		CreditMicrousd:     creditMicrousd,
		StripeCreditNoteID: creditNoteID,
		IssuedAt:           now,
	}
	if err := c.store.InsertSLACredit(ctx, row); err != nil {
		if errors.Is(err, ErrSLACreditAlreadyIssued) {
			return
		}
		c.logger.Warn("persisting sla credit failed", "org_id", contract.OrgID, "error", err)
		return
	}
	if c.dispatcher != nil {
		c.dispatchSLACreditWebhook(ctx, contract.OrgID, row)
	}

	c.logger.Info("sla credit issued",
		"org_id", contract.OrgID,
		"period_start", periodStart,
		"period_end", periodEnd,
		"uptime_pct", uptime,
		"target_pct", cfg.UptimeSLAPct,
		"credit_pct", creditPct,
		"credit_microusd", creditMicrousd,
		"stripe_credit_note_id", creditNoteID,
	)
}

func (c *SLACalculator) dispatchSLACreditWebhook(ctx context.Context, orgID string, row SLACreditRow) {
	c.dispatchMu.Lock()
	defer c.dispatchMu.Unlock()

	current, err := c.store.GetSLACredit(ctx, orgID, row.PeriodStart, row.PeriodEnd)
	if err != nil {
		c.logger.Warn("sla credit dispatch lookup failed", "org_id", orgID, "error", err)
		return
	}
	if current == nil {
		c.logger.Warn("sla credit disappeared before webhook dispatch", "org_id", orgID)
		return
	}
	if current.WebhookDispatchedAt != nil {
		c.logger.Debug("sla.credit_issued already dispatched", "org_id", orgID)
		return
	}
	row = *current

	detail := map[string]any{
		"period_start":          row.PeriodStart.UTC().Format(time.RFC3339),
		"period_end":            row.PeriodEnd.UTC().Format(time.RFC3339),
		"uptime_pct":            row.UptimePct,
		"target_pct":            row.TargetPct,
		"credit_pct":            row.CreditPct,
		"credit_microusd":       row.CreditMicrousd,
		"stripe_credit_note_id": row.StripeCreditNoteID,
	}
	if err := DispatchBillingWebhook(ctx, c.dispatcher, orgID, domain.PlanEnterprise, domain.WebhookEventSLACreditIssued, detail); err != nil {
		c.logger.Warn("dispatch sla.credit_issued failed", "org_id", orgID, "error", err)
		return
	}
	dispatchedAt := c.clock().UTC()
	marked, err := c.store.MarkSLACreditWebhookDispatched(ctx, orgID, row.PeriodStart, row.PeriodEnd, dispatchedAt)
	if err != nil {
		c.logger.Warn("mark sla.credit_issued dispatched failed", "org_id", orgID, "error", err)
		return
	}
	if !marked {
		c.logger.Debug("sla.credit_issued already marked dispatched", "org_id", orgID)
	}
}

// previousCalendarMonth returns [start, end) for the calendar month
// preceding the reference instant. End is the first instant of the
// reference's calendar month — exclusive — which keeps the window
// consistent regardless of when in the month the tick fires.
func previousCalendarMonth(ref time.Time) (time.Time, time.Time) {
	end := time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, time.UTC)
	start := end.AddDate(0, -1, 0)
	return start, end
}

// clampUptime coerces a reading into [0, 100]. Out-of-range values
// usually mean a broken uptime source rather than a real reading;
// clamping prevents a negative value from sliding into the bottom band
// and silently triggering a 50% credit.
func clampUptime(uptime float64) float64 {
	if uptime < 0 {
		return 0
	}
	if uptime > 100 {
		return 100
	}
	return uptime
}

// monthlyCreditMicrousd computes the credit in micro-USD: the monthly
// share of the annual commitment, multiplied by the credit percentage.
// Cents → micro-USD = ×10_000, annual → monthly = ÷12.
func monthlyCreditMicrousd(annualCents int64, creditPct int) int64 {
	monthlyMicrousd := (annualCents * 10_000) / 12
	return monthlyMicrousd * int64(creditPct) / 100
}
