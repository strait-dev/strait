//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestAuditRLS_CrossTenantBlocked(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("rls-test-secret")
	if err != nil {
		t.Fatalf("derive signing key: %v", err)
	}
	q.SetAuditSigningKey(signingKey)

	projA := "proj-rls-audit-a-" + newID()
	projB := "proj-rls-audit-b-" + newID()

	evA := &domain.AuditEvent{
		ProjectID:    projA,
		ActorID:      "user:u-a",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "job-a",
		Details:      json.RawMessage(`{"project":"a"}`),
	}
	if err := q.CreateAuditEvent(ctx, evA); err != nil {
		t.Fatalf("CreateAuditEvent(A): %v", err)
	}

	evB := &domain.AuditEvent{
		ProjectID:    projB,
		ActorID:      "user:u-b",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "job-b",
		Details:      json.RawMessage(`{"project":"b"}`),
	}
	if err := q.CreateAuditEvent(ctx, evB); err != nil {
		t.Fatalf("CreateAuditEvent(B): %v", err)
	}

	count := countAsProject(t, ctx, testDB.Pool, projA,
		"SELECT COUNT(*) FROM audit_events WHERE project_id = $1", projB)
	if count != 0 {
		t.Fatalf("project A can see %d events from project B, want 0 (RLS bypass)", count)
	}

	countOwn := countAsProject(t, ctx, testDB.Pool, projB,
		"SELECT COUNT(*) FROM audit_events WHERE project_id = $1", projB)
	if countOwn != 1 {
		t.Fatalf("project B sees %d of its own events, want 1", countOwn)
	}
}

func TestAuditRLS_SameProjectAllowed(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("rls-same-secret")
	if err != nil {
		t.Fatalf("derive signing key: %v", err)
	}
	q.SetAuditSigningKey(signingKey)

	projA := "proj-rls-same-" + newID()

	for i := range 3 {
		ev := &domain.AuditEvent{
			ProjectID:    projA,
			ActorID:      "user:u-a",
			ActorType:    "user",
			Action:       domain.AuditActionJobCreated,
			ResourceType: "job",
			ResourceID:   "job-" + newID(),
			Details:      json.RawMessage(`{"i":` + strconv.Itoa(i) + `}`),
		}
		if err := q.CreateAuditEvent(ctx, ev); err != nil {
			t.Fatalf("CreateAuditEvent[%d]: %v", i, err)
		}
	}

	count := countAsProject(t, ctx, testDB.Pool, projA,
		"SELECT COUNT(*) FROM audit_events WHERE project_id = $1", projA)
	if count != 3 {
		t.Fatalf("project A sees %d of its own events, want 3", count)
	}
}
