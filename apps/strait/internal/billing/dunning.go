package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"
)

// Dunning steps. The plan locks these values; do not renumber.
const (
	DunningStepNone  = 0 // not in dunning
	DunningStepEntry = 1 // Day 0 — grace begins, billing.delinquent already dispatched
	DunningStepDay3  = 2 // Day 3 reminder
	DunningStepDay7  = 3 // Day 7 reminder
	DunningStepDay14 = 4 // Day 14 — restrict access
	DunningStepDay44 = 5 // Day 44 — final warning
	DunningStepDay74 = 6 // Day 74 — suspend
)

// dunningSchedule maps each transition target step to the minimum elapsed
// duration (since dunning_entered_at) required to reach it. Ordered so the
// state machine can walk it forward and pick the highest reachable step.
var dunningSchedule = []struct {
	step    int
	elapsed time.Duration
}{
	{DunningStepEntry, 0},
	{DunningStepDay3, 3 * 24 * time.Hour},
	{DunningStepDay7, 7 * 24 * time.Hour},
	{DunningStepDay14, 14 * 24 * time.Hour},
	{DunningStepDay44, 44 * 24 * time.Hour},
	{DunningStepDay74, 74 * 24 * time.Hour},
}

// DefaultDunningTickCooldown is the minimum interval between successive ticks
// for the same row. Prevents the Dunner from spamming transitions when its
// scheduler ticks faster than the dunning cadence (one step per day at most).
const DefaultDunningTickCooldown = 24 * time.Hour

// DunningRow is the snapshot the Dunner reads to drive a transition.
type DunningRow struct {
	OrgID            string
	PlanTier         string
	PaymentStatus    string
	DunningStep      int
	DunningEnteredAt time.Time
}

// DunningTransition describes the post-state of a single row after one Tick.
type DunningTransition struct {
	OrgID         string
	NewStep       int
	PaymentStatus string // empty means leave unchanged
	TickAt        time.Time
}

// DunningStore is the narrow persistence interface the Dunner depends on.
// Kept separate from the main Store so test mocks elsewhere are not forced
// to add dunning plumbing.
type DunningStore interface {
	// StartDunning enters the org into dunning at step 1 if it is not
	// already in an active cycle. Idempotent on replays.
	StartDunning(ctx context.Context, orgID string, now time.Time) error

	// ResolveDunning clears the active cycle. Sets dunning_resolved_at to
	// NOW() and resets dunning_step / dunning_entered_at to their zero values.
	ResolveDunning(ctx context.Context, orgID string, now time.Time) error

	// ProcessDueDunningRows iterates up to `limit` active dunning rows whose
	// dunning_last_tick_at is NULL or older than `now - cooldown`, holding a
	// FOR UPDATE SKIP LOCKED row lock for each. For each row, `fn` decides
	// the next state (step + optional payment_status update); the store
	// applies the transition and stamps dunning_last_tick_at = now in the
	// same transaction. Returns the number of rows successfully processed.
	ProcessDueDunningRows(
		ctx context.Context,
		now time.Time,
		cooldown time.Duration,
		limit int,
		fn func(ctx context.Context, row DunningRow) (DunningTransition, error),
	) (int, error)
}

// DunningEmailSender is the narrow email surface the Dunner uses. Implemented
// by *BillingEmailSender; nil is treated as a no-op.
type DunningEmailSender interface {
	SendDunningStep(ctx context.Context, to []string, planName string, step int)
}

// DunningAdminLookup returns the admin notification recipient set for an org.
// Implemented by *PgStore via ListOrgAdminEmails. nil disables email sending.
type DunningAdminLookup interface {
	ListOrgAdminEmails(ctx context.Context, orgID string) ([]string, error)
}

// Dunner advances per-row dunning state. Constructed once and run periodically
// via Scheduler.WithDunner. Each Tick claims a bounded batch, computes the
// next reachable step for each row, persists the transition, and dispatches
// the billing.delinquent / billing.suspended outbound events.
type Dunner struct {
	store      DunningStore
	emails     DunningEmailSender
	admins     DunningAdminLookup
	dispatcher BillingEventDispatcher
	logger     *slog.Logger
	now        func() time.Time
	cooldown   time.Duration
	batchSize  int
}

// NewDunner constructs a Dunner. `store` is required; the rest are optional.
func NewDunner(store DunningStore, opts ...DunnerOption) *Dunner {
	d := &Dunner{
		store:     store,
		logger:    slog.Default(),
		now:       func() time.Time { return time.Now().UTC() },
		cooldown:  DefaultDunningTickCooldown,
		batchSize: 256,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// DunnerOption configures the Dunner.
type DunnerOption func(*Dunner)

func WithDunnerEmails(s DunningEmailSender) DunnerOption {
	return func(d *Dunner) { d.emails = s }
}

func WithDunnerAdminLookup(l DunningAdminLookup) DunnerOption {
	return func(d *Dunner) { d.admins = l }
}

func WithDunnerDispatcher(dp BillingEventDispatcher) DunnerOption {
	return func(d *Dunner) { d.dispatcher = dp }
}

func WithDunnerLogger(logger *slog.Logger) DunnerOption {
	return func(d *Dunner) {
		if logger != nil {
			d.logger = logger
		}
	}
}

func WithDunnerClock(now func() time.Time) DunnerOption {
	return func(d *Dunner) {
		if now != nil {
			d.now = now
		}
	}
}

func WithDunnerCooldown(cooldown time.Duration) DunnerOption {
	return func(d *Dunner) {
		if cooldown > 0 {
			d.cooldown = cooldown
		}
	}
}

// Tick processes one batch of active dunning rows.
func (d *Dunner) Tick(ctx context.Context) error {
	if d == nil || d.store == nil {
		return nil
	}
	now := d.now()
	processed, err := d.store.ProcessDueDunningRows(ctx, now, d.cooldown, d.batchSize, func(rowCtx context.Context, row DunningRow) (DunningTransition, error) {
		return d.decide(rowCtx, now, row)
	})
	if err != nil {
		return fmt.Errorf("processing dunning batch: %w", err)
	}
	if processed > 0 {
		d.logger.Info("dunning tick processed batch", "rows", processed)
	}
	return nil
}

// decide computes the next state for one row and emits side effects (emails,
// outbound events). Returning an error rolls back the row's transaction.
func (d *Dunner) decide(ctx context.Context, now time.Time, row DunningRow) (DunningTransition, error) {
	if row.DunningEnteredAt.IsZero() {
		return DunningTransition{}, errors.New("dunning row has zero entered_at")
	}
	elapsed := now.Sub(row.DunningEnteredAt)
	if elapsed < 0 {
		// Clock skew or hand-edited future timestamp: safe no-op, do not
		// regress the step. Stamp last_tick_at to throttle re-evaluation.
		d.logger.Warn("dunning row has future entered_at; skipping", "org_id", row.OrgID, "entered_at", row.DunningEnteredAt)
		return DunningTransition{OrgID: row.OrgID, NewStep: row.DunningStep, TickAt: now}, nil
	}

	target := row.DunningStep
	for _, st := range dunningSchedule {
		if elapsed >= st.elapsed && st.step > target {
			target = st.step
		}
	}
	if target == row.DunningStep {
		return DunningTransition{OrgID: row.OrgID, NewStep: row.DunningStep, TickAt: now}, nil
	}

	var nextPaymentStatus string
	switch target {
	case DunningStepDay14:
		nextPaymentStatus = "restricted"
	case DunningStepDay74:
		nextPaymentStatus = "suspended"
	}

	// Side effects happen *before* commit so a failure rolls the row back
	// and the next Tick retries. Emails and dispatches are best-effort:
	// they log on failure but do not abort the transition.
	d.sendStepEmail(ctx, row, target)
	d.dispatchTransition(ctx, row, row.DunningStep, target)

	return DunningTransition{
		OrgID:         row.OrgID,
		NewStep:       target,
		PaymentStatus: nextPaymentStatus,
		TickAt:        now,
	}, nil
}

func (d *Dunner) sendStepEmail(ctx context.Context, row DunningRow, step int) {
	if d.emails == nil || d.admins == nil {
		return
	}
	recipients, err := d.admins.ListOrgAdminEmails(ctx, row.OrgID)
	if err != nil {
		d.logger.Warn("dunning email recipient lookup failed", "org_id", row.OrgID, "err", err)
		return
	}
	if len(recipients) == 0 {
		return
	}
	d.emails.SendDunningStep(ctx, recipients, row.PlanTier, step)
}

func (d *Dunner) dispatchTransition(ctx context.Context, row DunningRow, previousStep, step int) {
	if d.dispatcher == nil {
		return
	}
	tier := domain.PlanTier(row.PlanTier)
	// Skip billing.delinquent on the entry transition. handlePaymentFailed in
	// the Stripe webhook handler already announced the delinquent state when
	// invoice.payment_failed fired; the first Dunner tick advancing 0→Entry
	// represents the same event, so re-dispatching would deliver duplicate
	// billing.delinquent webhooks to subscribers within the first day.
	// Escalation transitions (Entry→Day14, Day14→Day74) still dispatch
	// because they're genuine state changes a subscriber needs to react to.
	if previousStep != DunningStepNone {
		delinquentDetail := map[string]any{
			"dunning_step":       step,
			"dunning_entered_at": row.DunningEnteredAt.UTC().Format(time.RFC3339Nano),
		}
		if err := DispatchBillingWebhook(ctx, d.dispatcher, row.OrgID, tier, domain.WebhookEventBillingDelinquent, delinquentDetail); err != nil {
			d.logger.Warn("billing.delinquent dispatch failed", "org_id", row.OrgID, "err", err)
		}
	}
	if step == DunningStepDay74 {
		suspendDetail := map[string]any{
			"reason":             "dunning_exhausted",
			"dunning_step":       step,
			"dunning_entered_at": row.DunningEnteredAt.UTC().Format(time.RFC3339Nano),
		}
		if err := DispatchBillingWebhook(ctx, d.dispatcher, row.OrgID, tier, domain.WebhookEventBillingSuspended, suspendDetail); err != nil {
			d.logger.Warn("billing.suspended dispatch failed", "org_id", row.OrgID, "err", err)
		}
	}
}

// Run drives the Dunner on a fixed interval until ctx is cancelled. The
// per-row cooldown (default 24h) prevents over-eager transitions even if the
// outer interval is short. Suitable for scheduler.tracker.track.
func (d *Dunner) Run(ctx context.Context) {
	const tickInterval = 1 * time.Hour
	t := time.NewTicker(tickInterval)
	defer t.Stop()
	d.run(ctx, t.C)
}

func (d *Dunner) run(ctx context.Context, ticks <-chan time.Time) {
	if err := d.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
		d.logger.Error("initial dunning tick failed", "err", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			if err := d.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
				d.logger.Error("dunning tick failed", "err", err)
			}
		}
	}
}
