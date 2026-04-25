package store

import (
	"context"
	"crypto/hmac"
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

// VerifyAuditChainIncremental verifies only the audit events that have
// been appended since the project's last recorded successful checkpoint.
// On first call (or whenever no checkpoint exists), it falls back to
// VerifyAuditChain and records a checkpoint on success. Subsequent calls
// read only rows strictly after the checkpoint's anchoring event.
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

	// Look up the checkpoint event's (rotation_epoch, created_at, signature).
	// These pin the resume position: we want every row whose (epoch, ts, id)
	// is strictly greater than the checkpoint's.
	var (
		cpEpoch         int
		cpCreatedAt     time.Time
		cpSignature     string
		cpIsAnchorValid bool
	)
	if err := q.db.QueryRow(ctx, `
		SELECT rotation_epoch, created_at, signature
		FROM audit_events
		WHERE id = $1 AND project_id = $2
	`, cp.LastVerifiedEventID, projectID).Scan(&cpEpoch, &cpCreatedAt, &cpSignature); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The checkpointed event no longer exists — retention
			// trimmed the tail since the last verify. Fall back to a
			// full verify so we re-anchor from the surviving head.
			return q.verifyAuditChainIncrementalFallback(ctx, projectID)
		}
		return nil, fmt.Errorf("incremental verify: read checkpoint event: %w", err)
	}
	cpIsAnchorValid = cpSignature != ""
	if !cpIsAnchorValid {
		// Degenerate: checkpointed event has no signature. Should not
		// happen with the atomic insert path, but be defensive and
		// fall back rather than propagating a nonsense anchor.
		return q.verifyAuditChainIncrementalFallback(ctx, projectID)
	}

	result := &domain.AuditChainVerification{
		ProjectID:   projectID,
		Valid:       true,
		Incremental: true,
	}

	// Resume predicate: strictly after the checkpoint.
	// (rotation_epoch, created_at, id) is the deterministic total
	// ordering for rows within a project.
	const query = `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id, details, signature, previous_hash, created_at,
		       remote_ip, user_agent, request_id, trace_id, schema_version,
		       is_anchor, rotation_epoch
		FROM audit_events
		WHERE project_id = $1
		  AND (
		        rotation_epoch > $2
		     OR (rotation_epoch = $2 AND created_at > $3)
		     OR (rotation_epoch = $2 AND created_at = $3 AND id > $4)
		      )
		ORDER BY rotation_epoch ASC, created_at ASC, id ASC`

	epochKeyCache, err := q.preloadEpochKeys(ctx, projectID,
		`rotation_epoch > $2
		 OR (rotation_epoch = $2 AND created_at > $3)
		 OR (rotation_epoch = $2 AND created_at = $3 AND id > $4)`,
		cpEpoch, cpCreatedAt, cp.LastVerifiedEventID)
	if err != nil {
		return nil, err
	}

	rows, err := q.db.Query(ctx, query, projectID, cpEpoch, cpCreatedAt, cp.LastVerifiedEventID)
	if err != nil {
		return nil, fmt.Errorf("incremental verify: query: %w", err)
	}
	defer rows.Close()

	expectedPrevHash := cpSignature
	result.ChainStart = cpSignature

	for rows.Next() {
		var ev domain.AuditEvent
		if err := rows.Scan(&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action, &ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.Signature, &ev.PreviousHash, &ev.CreatedAt, &ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion, &ev.IsAnchor, &ev.RotationEpoch); err != nil {
			return nil, fmt.Errorf("incremental verify scan: %w", err)
		}

		result.EventsChecked++
		if result.FirstEventID == "" {
			result.FirstEventID = ev.ID
		}
		result.LastEventID = ev.ID

		if ev.PreviousHash != expectedPrevHash {
			result.Valid = false
			result.BrokenAtID = ev.ID
			result.Error = fmt.Sprintf("chain broken at event %s: previous_hash mismatch (expected %s, got %s)", ev.ID, expectedPrevHash, ev.PreviousHash)
			return result, nil
		}

		key, keyErr := q.keyForEpoch(epochKeyCache, ev.RotationEpoch)
		if keyErr != nil {
			return nil, keyErr
		}
		expected := ComputeAuditSignature(&ev, key)
		if !hmac.Equal([]byte(ev.Signature), []byte(expected)) {
			result.Valid = false
			result.BrokenAtID = ev.ID
			result.Error = fmt.Sprintf("signature mismatch at event %s: event may have been tampered with", ev.ID)
			return result, nil
		}

		expectedPrevHash = ev.Signature
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("incremental verify rows: %w", err)
	}

	// Advance the checkpoint only on a clean verify that extended the tail.
	// EventsChecked == 0 means no new rows since the last verify; we still
	// refresh last_verified_at so dashboards see the re-check happened.
	newTail := cp.LastVerifiedEventID
	if result.LastEventID != "" {
		newTail = result.LastEventID
	}
	if err := q.upsertAuditChainCheckpoint(ctx, projectID, newTail, time.Now().UTC()); err != nil {
		return nil, err
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
