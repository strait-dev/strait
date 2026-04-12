//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestAuditDeadletter_RoundTrip(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("dlq-test-secret")
	if err != nil {
		t.Fatalf("derive signing key: %v", err)
	}
	q.SetAuditSigningKey(signingKey)

	ev := &domain.AuditEvent{
		ID:           "dlq-ev-1",
		ProjectID:    "proj-dlq",
		ActorID:      "actor-1",
		ActorType:    "user",
		Action:       domain.AuditActionJobTriggered,
		ResourceType: "job",
		ResourceID:   "job-1",
		Details:      json.RawMessage(`{"run_id":"r1"}`),
		CreatedAt:    time.Now().UTC(),
	}

	if err := q.CreateAuditEventDeadletter(ctx, ev, "db down", 3); err != nil {
		t.Fatalf("CreateAuditEventDeadletter: %v", err)
	}

	count, err := q.CountAuditEventsDeadletter(ctx)
	if err != nil {
		t.Fatalf("CountAuditEventsDeadletter: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Direct SELECT to verify the stored fields.
	var storedAction, lastErr string
	var retryCount int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT action, last_error, retry_count
		FROM audit_events_deadletter WHERE id = $1
	`, "dlq-ev-1").Scan(&storedAction, &lastErr, &retryCount); err != nil {
		t.Fatalf("query deadletter row: %v", err)
	}

	if storedAction != domain.AuditActionJobTriggered {
		t.Errorf("action = %q, want %q", storedAction, domain.AuditActionJobTriggered)
	}
	if lastErr != "db down" {
		t.Errorf("last_error = %q, want 'db down'", lastErr)
	}
	if retryCount != 3 {
		t.Errorf("retry_count = %d, want 3", retryCount)
	}

	// Round-trip via the main chain is unaffected — deadletter does not
	// participate in the signed chain.
	vc, err := q.VerifyAuditChain(ctx, "proj-dlq")
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !vc.Valid {
		t.Errorf("main chain should be valid (empty) despite deadletter rows: %s", vc.Error)
	}
	if vc.EventsChecked != 0 {
		t.Errorf("events_checked = %d, want 0 (deadletter is separate)", vc.EventsChecked)
	}

}
