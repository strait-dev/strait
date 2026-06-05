//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestAuditRLS_CrossTenantBlocked(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("rls-test-secret")
	require.NoError(t, err)

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
	require.NoError(t, q.CreateAuditEvent(ctx, evA))

	evB := &domain.AuditEvent{
		ProjectID:    projB,
		ActorID:      "user:u-b",
		ActorType:    "user",
		Action:       domain.AuditActionJobCreated,
		ResourceType: "job",
		ResourceID:   "job-b",
		Details:      json.RawMessage(`{"project":"b"}`),
	}
	require.NoError(t, q.CreateAuditEvent(ctx, evB))

	count := countAsProject(t, ctx, testDB.Pool, projA,
		"SELECT COUNT(*) FROM audit_events WHERE project_id = $1", projB)
	require.EqualValues(t, 0, count)

	countOwn := countAsProject(t, ctx, testDB.Pool, projB,
		"SELECT COUNT(*) FROM audit_events WHERE project_id = $1", projB)
	require.EqualValues(t, 1, countOwn)

}

func TestAuditRLS_SameProjectAllowed(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("rls-same-secret")
	require.NoError(t, err)

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
		require.NoError(t, q.CreateAuditEvent(ctx, ev))

	}

	count := countAsProject(t, ctx, testDB.Pool, projA,
		"SELECT COUNT(*) FROM audit_events WHERE project_id = $1", projA)
	require.EqualValues(t, 3, count)

}
