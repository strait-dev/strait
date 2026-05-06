package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// GetAuditChainCheckpoint returns the most recent successful verification
// checkpoint for a project, or (nil, nil) when none exists. The returned
// struct is used to seed the incremental scan — its lastEventID pins
// where the incremental verify resumes from.
func (q *Queries) GetAuditChainCheckpoint(ctx context.Context, projectID string) (*AuditChainCheckpoint, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAuditChainCheckpoint")
	defer span.End()

	const query = `
		SELECT project_id, last_verified_event_id, last_verified_at
		FROM audit_chain_checkpoints
		WHERE project_id = $1`

	var cp AuditChainCheckpoint
	err := q.db.QueryRow(ctx, query, projectID).Scan(
		&cp.ProjectID, &cp.LastVerifiedEventID, &cp.LastVerifiedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get audit chain checkpoint: %w", err)
	}
	return &cp, nil
}

// AuditChainCheckpoint is the public form of the audit_chain_checkpoints
// row. Exported so health-style dashboards and API handlers can surface
// "last verified at" without exposing the raw table schema.
type AuditChainCheckpoint struct {
	ProjectID           string
	LastVerifiedEventID string
	LastVerifiedAt      time.Time
}

// upsertAuditChainCheckpoint writes or replaces the per-project checkpoint
// inside the caller's transaction. Called by VerifyAuditChainIncremental
// on successful completion so the next incremental verify resumes from
// the newly-extended tail.
func (q *Queries) upsertAuditChainCheckpoint(ctx context.Context, projectID, lastEventID string, at time.Time) error {
	if _, err := q.db.Exec(ctx, `
		INSERT INTO audit_chain_checkpoints (project_id, last_verified_event_id, last_verified_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id) DO UPDATE
			SET last_verified_event_id = EXCLUDED.last_verified_event_id,
			    last_verified_at       = EXCLUDED.last_verified_at
	`, projectID, lastEventID, at); err != nil {
		return fmt.Errorf("upsert audit chain checkpoint: %w", err)
	}
	return nil
}

// VerifyAuditChainIncremental verifies the full surviving audit-event chain and
// refreshes the project's last successful checkpoint. The checkpoint is only a
// progress cursor for dashboards and future implementation work, never a
// cryptographic trust root; every successful call revalidates the prefix so
// pre-checkpoint tampering cannot be hidden by a clean appended tail.
//
// Ordering contract (must match VerifyAuditChain): rotation_epoch ASC,
// then created_at ASC, then id ASC. The checkpoint stores the tail
// event's id; the resume predicate advances past every row up to and
// including that id.
//
// Failure policy: any invalid chain outcome is reported to the caller
// with Incremental=true set, but the checkpoint is NOT advanced. A
// subsequent call will re-read the same tail and reproduce the same
// failure until the operator trims or restores the chain. Transient
// store errors bubble up without a checkpoint write for the same reason.
func (q *Queries) VerifyAuditChainIncremental(ctx context.Context, projectID string) (*domain.AuditChainVerification, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.VerifyAuditChainIncremental")
	defer span.End()

	if q.auditSigningKey == nil {
		return nil, fmt.Errorf("audit signing key is not configured")
	}

	cp, err := q.GetAuditChainCheckpoint(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if cp == nil {
		// Cold path: no checkpoint yet. Full verify, then plant the
		// checkpoint at the tail on success. This is structurally
		// identical to a non-incremental call except for the flag and
		// the post-success upsert.
		result, ferr := q.VerifyAuditChain(ctx, projectID)
		if ferr != nil {
			return nil, ferr
		}
		result.Incremental = true
		if result.Valid && result.LastEventID != "" {
			if err := q.upsertAuditChainCheckpoint(ctx, projectID, result.LastEventID, time.Now().UTC()); err != nil {
				return nil, err
			}
		}
		return result, nil
	}

	// A checkpoint is a cursor, not a cryptographic trust root. Re-verify the
	// full surviving chain before refreshing it so historical tampering before
	// the checkpoint cannot be hidden by an otherwise-valid appended tail.
	result, err := q.VerifyAuditChain(ctx, projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return q.verifyAuditChainIncrementalFallback(ctx, projectID)
		}
		return nil, err
	}
	result.Incremental = true

	if result.Valid && result.LastEventID != "" {
		if err := q.upsertAuditChainCheckpoint(ctx, projectID, result.LastEventID, time.Now().UTC()); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// verifyAuditChainIncrementalFallback is called when the stored checkpoint
// references an event that no longer exists (retention trimmed it out).
// Delegates to VerifyAuditChain, sets Incremental=true, and replants the
// checkpoint on success.
func (q *Queries) verifyAuditChainIncrementalFallback(ctx context.Context, projectID string) (*domain.AuditChainVerification, error) {
	result, err := q.VerifyAuditChain(ctx, projectID)
	if err != nil {
		return nil, err
	}
	result.Incremental = true
	if result.Valid && result.LastEventID != "" {
		if uerr := q.upsertAuditChainCheckpoint(ctx, projectID, result.LastEventID, time.Now().UTC()); uerr != nil {
			return nil, uerr
		}
	}
	return result, nil
}
