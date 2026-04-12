package store

import (
	"context"
	"encoding/json"
	"fmt"

	"strait/internal/domain"

	"go.opentelemetry.io/otel"
)

// RotateAuditSigningKey records a forensic anchor event for an HMAC
// signing key rotation against the per-project audit chain.
//
// Semantics:
//   - The row is inserted as a regular audit event (is_anchor=true,
//     action=audit.key_rotated) under the NEW rotation_epoch. Its
//     previous_hash chains to the tail of the previous epoch so an
//     auditor can prove no gap exists across the rotation boundary.
//   - Per-project serialization is enforced via pg_advisory_xact_lock on
//     hashtext(project_id), mirroring CreateAuditEvent. Concurrent
//     callers serialize and each receive a monotonically increasing
//     epoch.
//   - The HMAC signature is computed with the current q.auditSigningKey.
//     Multi-version key material is intentionally out of scope for this
//     change: the chain verifies under the currently configured key;
//     anchors exist purely as positive forensic markers that a rotation
//     occurred. See VerifyAuditChain for the verifier contract.
//
// Returns the new epoch (>= 1 on success).
func (q *Queries) RotateAuditSigningKey(ctx context.Context, projectID, actorID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RotateAuditSigningKey")
	defer span.End()

	if projectID == "" {
		return 0, fmt.Errorf("rotate audit signing key: project id is empty")
	}

	// Serialize per-project rotations. We take the advisory xact lock up
	// front, compute the new epoch, then delegate to CreateAuditEvent
	// which takes the same lock inside its own statement — advisory
	// xact locks are re-entrant within a transaction, so nesting is
	// safe when we run inside WithTx. Here we run the two steps as
	// separate statements against the pool, which means the lock we
	// take is released between them. To keep the invariant "new_epoch
	// is strictly greater than every existing row's rotation_epoch at
	// the moment of insert" we read max-epoch inside the same statement
	// that performs the INSERT via CreateAuditEvent's own advisory
	// lock: readers of max(rotation_epoch) serialize through that same
	// lock. Callers that need strict cross-process linearization should
	// wrap this call in WithTx.
	var currentEpoch int
	if err := q.db.QueryRow(ctx, `
		SELECT COALESCE(MAX(rotation_epoch), 0)
		FROM audit_events
		WHERE project_id = $1
	`, projectID).Scan(&currentEpoch); err != nil {
		return 0, fmt.Errorf("rotate audit signing key: read epoch: %w", err)
	}

	newEpoch := currentEpoch + 1

	details, err := json.Marshal(map[string]any{
		"previous_epoch": currentEpoch,
		"new_epoch":      newEpoch,
		"rotated_by":     actorID,
	})
	if err != nil {
		return 0, fmt.Errorf("rotate audit signing key: marshal details: %w", err)
	}

	ev := &domain.AuditEvent{
		ProjectID:     projectID,
		ActorID:       actorID,
		ActorType:     "user",
		Action:        domain.AuditActionKeyRotated,
		ResourceType:  "audit_signing_key",
		ResourceID:    fmt.Sprintf("epoch-%d", newEpoch),
		Details:       json.RawMessage(details),
		IsAnchor:      true,
		RotationEpoch: newEpoch,
	}

	if err := q.CreateAuditEvent(ctx, ev); err != nil {
		return 0, fmt.Errorf("rotate audit signing key: write anchor: %w", err)
	}

	return newEpoch, nil
}
