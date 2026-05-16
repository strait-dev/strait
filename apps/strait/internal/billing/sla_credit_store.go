package billing

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SLACreditRow mirrors the sla_credits row used by the SLA credit
// calculator. The (org_id, period_start, period_end) tuple is the
// idempotency key: the table's UNIQUE constraint guarantees that a
// re-tick within the same billing period observes the existing row
// and skips re-issuance.
type SLACreditRow struct {
	ID                 string
	OrgID              string
	PeriodStart        time.Time
	PeriodEnd          time.Time
	UptimePct          float64
	TargetPct          float64
	CreditPct          int
	CreditMicrousd     int64
	StripeCreditNoteID string
	IssuedAt           time.Time
}

// ErrSLACreditAlreadyIssued is returned by InsertSLACredit when a credit
// row already exists for the same (org_id, period_start, period_end).
// Callers treat this as a no-op success (idempotency win).
var ErrSLACreditAlreadyIssued = errors.New("sla credit already issued for period")

// SLACreditStore is the persistence boundary for the SLA credit pipeline.
// The interface stays narrow so test doubles can stub a single method.
type SLACreditStore interface {
	InsertSLACredit(ctx context.Context, row SLACreditRow) error
	GetSLACredit(ctx context.Context, orgID string, periodStart, periodEnd time.Time) (*SLACreditRow, error)
}

// PgSLACreditStore is the production SLACreditStore backed by Postgres.
type PgSLACreditStore struct {
	pool *pgxpool.Pool
}

// NewPgSLACreditStore wires a pgxpool-backed SLA credit store.
func NewPgSLACreditStore(pool *pgxpool.Pool) *PgSLACreditStore {
	return &PgSLACreditStore{pool: pool}
}

// InsertSLACredit persists a credit row. The unique constraint on
// (org_id, period_start, period_end) is the idempotency guard:
// concurrent ticks lose the race cleanly and surface
// ErrSLACreditAlreadyIssued so the calculator can short-circuit.
//
// Concurrency note (TOCTOU between Stripe call and Insert): the
// SLACalculator already calls IssueCredit upstream before reaching this
// method. If two ticks race for the same (org_id, period) and both pass
// the pre-check GetSLACredit at sla_credit.go, both will issue a Stripe
// credit note. The Stripe SDK's idempotency key
// (sla-credit-{orgID}-{YYYY-MM}) guarantees Stripe itself dedups, so no
// double-charge can occur — the loser just wastes one API call. The
// INSERT ... ON CONFLICT DO NOTHING + second-read GetSLACredit pattern
// below is what makes the loser observable: when the row in the DB has a
// different ID than the one we tried to insert, we return
// ErrSLACreditAlreadyIssued and the caller silently moves on. Acceptable
// trade-off; documented here so the next person who reads this code
// doesn't try to "fix" the wasted call by holding a row lock across the
// Stripe call (which would block other ticks for the duration of an
// external network round-trip).
func (s *PgSLACreditStore) InsertSLACredit(ctx context.Context, row SLACreditRow) error {
	_, err := s.pool.Exec(ctx, `
        INSERT INTO sla_credits
            (id, org_id, period_start, period_end,
             uptime_pct, target_pct, credit_pct,
             credit_microusd, stripe_credit_note_id, issued_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), $10)
        ON CONFLICT (org_id, period_start, period_end) DO NOTHING
    `,
		row.ID,
		row.OrgID,
		row.PeriodStart,
		row.PeriodEnd,
		row.UptimePct,
		row.TargetPct,
		row.CreditPct,
		row.CreditMicrousd,
		row.StripeCreditNoteID,
		row.IssuedAt,
	)
	if err != nil {
		return err
	}
	existing, err := s.GetSLACredit(ctx, row.OrgID, row.PeriodStart, row.PeriodEnd)
	if err != nil {
		return err
	}
	if existing != nil && existing.ID != row.ID {
		return ErrSLACreditAlreadyIssued
	}
	return nil
}

// GetSLACredit returns the credit row for an org+period if one exists.
func (s *PgSLACreditStore) GetSLACredit(ctx context.Context, orgID string, periodStart, periodEnd time.Time) (*SLACreditRow, error) {
	row := s.pool.QueryRow(ctx, `
        SELECT id, org_id, period_start, period_end,
               uptime_pct, target_pct, credit_pct,
               credit_microusd, COALESCE(stripe_credit_note_id, ''), issued_at
        FROM sla_credits
        WHERE org_id = $1 AND period_start = $2 AND period_end = $3
    `, orgID, periodStart, periodEnd)
	var r SLACreditRow
	if err := row.Scan(
		&r.ID,
		&r.OrgID,
		&r.PeriodStart,
		&r.PeriodEnd,
		&r.UptimePct,
		&r.TargetPct,
		&r.CreditPct,
		&r.CreditMicrousd,
		&r.StripeCreditNoteID,
		&r.IssuedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil //nolint:nilnil // "no row" is a valid result the caller distinguishes from error
		}
		return nil, err
	}
	return &r, nil
}
