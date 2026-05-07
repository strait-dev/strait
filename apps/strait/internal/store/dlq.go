package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// ReplayDeadLetterRunWithAudit atomically (a) CASes a dead-letter run
// back to queued, (b) marks the original row's replayed_run_id for
// lineage, and (c) writes the supplied audit event. All three writes
// share a single transaction so a crash midway cannot leave half-applied
// state (the prior sequential path could leave the run queued without an
// audit row when the final CreateAuditEvent failed).
//
// The caller pre-builds the audit row (actor, action, resource, project)
// before entering the transaction; this helper fills in the before/after
// details payload using the CAS result. Returns ErrRunNotFound when the
// row does not exist and ErrRunConflict when the row exists but is not in
// dead_letter status or already carries a replayed_run_id.
func (q *Queries) ReplayDeadLetterRunWithAudit(ctx context.Context, runID string, audit *domain.AuditEvent) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReplayDeadLetterRunWithAudit")
	defer span.End()

	if audit == nil {
		return nil, fmt.Errorf("replay dlq with audit: audit event is required")
	}

	beginner, ok := q.db.(TxBeginner)
	if !ok {
		return nil, fmt.Errorf("replay dlq with audit: db does not support transactions")
	}

	var result *domain.JobRun
	err := WithTx(ctx, beginner, func(txQ *Queries) error {
		// Carry the outer Queries' audit signing and secret keys into the
		// tx scope so CreateAuditEvent signs this event under the same
		// chain as the non-transactional path.
		txQ.auditSigningKey = q.auditSigningKey
		txQ.secretEncryptionKey = q.secretEncryptionKey
		txQ.oldSecretEncryptionKeys = append([]string(nil), q.oldSecretEncryptionKeys...)

		run, err := txQ.ReplayDeadLetterRun(ctx, runID)
		if err != nil {
			return err
		}

		if err := txQ.MarkRunReplayed(ctx, runID, run.ID); err != nil {
			return fmt.Errorf("mark run replayed: %w", err)
		}

		details := map[string]any{
			"run_id": runID,
			"before": map[string]any{"status": domain.StatusDeadLetter},
			"after":  map[string]any{"status": run.Status, "replayed_run_id": run.ID},
		}
		raw, mErr := json.Marshal(details)
		if mErr != nil {
			return fmt.Errorf("marshal audit details: %w", mErr)
		}
		audit.Details = raw
		if audit.ResourceID == "" {
			audit.ResourceID = runID
		}
		if audit.ResourceType == "" {
			audit.ResourceType = "job_run"
		}
		// Derive project id from the CAS result so callers don't have to
		// pre-load the run just to populate the audit envelope.
		if audit.ProjectID == "" {
			audit.ProjectID = run.ProjectID
		}

		if err := txQ.CreateAuditEvent(ctx, audit); err != nil {
			return fmt.Errorf("create audit event: %w", err)
		}

		result = run
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("replay dlq with audit: %w", err)
	}
	return result, nil
}

// DLQ admin helpers power the /v1/admin/dlq HTTP endpoints. Listing and
// replay reuse the existing ListDeadLetterRuns / ReplayDeadLetterRun
// helpers on Queries; this file adds the mutations that did not previously
// have dedicated entry points (unmask + purge).

// UnmaskDLQRun clears visible_until on a dead-letter run so it is no
// longer hidden by the DLQ age-out process. The run must already be in
// dead_letter status; otherwise ErrRunConflict is returned. Returns
// ErrRunNotFound if the run does not exist.
func (q *Queries) UnmaskDLQRun(ctx context.Context, runID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UnmaskDLQRun")
	defer span.End()

	// Single-statement mutation with guard: avoids a check-then-act race
	// where two concurrent callers both observe status=dead_letter before
	// either lands its UPDATE. The loser's RETURNING comes back empty; we
	// then do a follow-up SELECT to disambiguate ErrRunNotFound (row is
	// gone) from ErrRunConflict (row exists but is no longer dead_letter).
	var id string
	err := q.db.QueryRow(ctx, `UPDATE job_runs SET visible_until = NULL WHERE id = $1 AND status = 'dead_letter' RETURNING id`, runID).Scan(&id)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("unmask dlq run: %w", err)
	}

	var status domain.RunStatus
	loadErr := q.db.QueryRow(ctx, `SELECT status FROM job_runs WHERE id = $1`, runID).Scan(&status)
	if loadErr != nil {
		if errors.Is(loadErr, pgx.ErrNoRows) {
			return ErrRunNotFound
		}
		return fmt.Errorf("unmask dlq run: disambiguate: %w", loadErr)
	}
	return fmt.Errorf("%w: run %s has status %s, expected dead_letter", ErrRunConflict, runID, status)
}

// PurgeDLQRun hard-deletes a dead-letter run. Requires the caller to
// already hold the dlq:purge scope. Returns ErrRunNotFound if the run
// does not exist or is not in dead_letter status.
func (q *Queries) PurgeDLQRun(ctx context.Context, runID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.PurgeDLQRun")
	defer span.End()

	// Single-statement DELETE with status guard; see UnmaskDLQRun for the
	// rationale. On empty RETURNING we disambiguate not-found from
	// conflict via a follow-up SELECT.
	var id string
	err := q.db.QueryRow(ctx, `DELETE FROM job_runs WHERE id = $1 AND status = 'dead_letter' RETURNING id`, runID).Scan(&id)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("purge dlq run: %w", err)
	}

	var status domain.RunStatus
	loadErr := q.db.QueryRow(ctx, `SELECT status FROM job_runs WHERE id = $1`, runID).Scan(&status)
	if loadErr != nil {
		if errors.Is(loadErr, pgx.ErrNoRows) {
			return ErrRunNotFound
		}
		return fmt.Errorf("purge dlq run: disambiguate: %w", loadErr)
	}
	return fmt.Errorf("%w: run %s has status %s, expected dead_letter", ErrRunConflict, runID, status)
}

// MarkRunReplayed sets replayed_run_id on an original dead-letter run to
// record the lineage of a replay. Called by the admin DLQ replay handler
// after a successful re-enqueue. replayedByRunID may equal originalRunID
// when the existing ReplayDeadLetterRun behavior reuses the same run ID.
//
// Stamp-once semantics: the WHERE clause guards on replayed_run_id IS
// NULL so a second call cannot overwrite the first replay's lineage.
// Status is not part of the guard because ReplayDeadLetterRun has
// already CAS'd the row out of dead_letter by the time this runs.
//
// Returns ErrRunNotFound when the row does not exist, and
// ErrRunConflict when the row exists but already carries a
// replayed_run_id (out-of-order or duplicate call).
func (q *Queries) MarkRunReplayed(ctx context.Context, originalRunID, replayedByRunID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkRunReplayed")
	defer span.End()

	var id string
	err := q.db.QueryRow(ctx,
		`UPDATE job_runs SET replayed_run_id = $1 WHERE id = $2 AND replayed_run_id IS NULL RETURNING id`,
		replayedByRunID, originalRunID,
	).Scan(&id)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("mark run replayed: %w", err)
	}

	// Disambiguate: row absent vs already stamped.
	var existing *string
	loadErr := q.db.QueryRow(ctx, `SELECT replayed_run_id FROM job_runs WHERE id = $1`, originalRunID).Scan(&existing)
	if loadErr != nil {
		if errors.Is(loadErr, pgx.ErrNoRows) {
			return ErrRunNotFound
		}
		return fmt.Errorf("mark run replayed: disambiguate: %w", loadErr)
	}
	return fmt.Errorf("%w: run %s already has replayed_run_id set", ErrRunConflict, originalRunID)
}
