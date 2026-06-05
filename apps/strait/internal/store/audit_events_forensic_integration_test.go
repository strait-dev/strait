//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditForensic_RoundTrip(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("forensic-roundtrip-secret")
	require.NoError(t, err)

	q.SetAuditSigningKey(signingKey)

	projectID := "proj-forensic-" + newID()

	ev := &domain.AuditEvent{
		ProjectID:     projectID,
		ActorID:       "user:u-1",
		ActorType:     "user",
		Action:        domain.AuditActionJobCreated,
		ResourceType:  "job",
		ResourceID:    "job-1",
		Details:       json.RawMessage(`{"name":"test-job"}`),
		RemoteIP:      "198.51.100.42",
		UserAgent:     "TestAgent/1.0",
		RequestID:     "req-abc-123",
		TraceID:       "trace-def-456",
		SchemaVersion: 2,
	}
	require.NoError(t, q.CreateAuditEvent(ctx, ev))
	require.NotEqual(t, "",

		ev.ID)

	events, err := q.ListAuditEvents(ctx, projectID, "", "", "", 10, nil, nil, nil, false)
	require.NoError(t, err)
	require.Len(t, events,
		1,
	)

	got := events[0]
	assert.Equal(t, ev.RemoteIP,

		got.
			RemoteIP)
	assert.Equal(t, ev.UserAgent,

		got.
			UserAgent)
	assert.Equal(t, ev.RequestID,

		got.
			RequestID)
	assert.Equal(t, ev.TraceID,

		got.TraceID,
	)
	assert.EqualValues(t, 4, got.
		SchemaVersion,
	)
	assert.NotEqual(t, "",
		got.
			Signature,
	)

	// CreateAuditEvent auto-derives shard_id from resource_type for
	// non-anchor events and force-bumps the canonical schema to v4 so
	// the HMAC binds shard_id. Callers cannot opt out of the bump; it is
	// the invariant that prevents a shard_id flip from leaving a v3
	// signature intact. Asserting v4 here pins that contract.

}

func TestAuditForensic_ChainVerifiesWithForensicFields(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("forensic-chain-secret")
	require.NoError(t, err)

	q.SetAuditSigningKey(signingKey)

	projectID := "proj-forensic-chain-" + newID()

	for i := range 5 {
		ev := &domain.AuditEvent{
			ProjectID:     projectID,
			ActorID:       "user:u-1",
			ActorType:     "user",
			Action:        domain.AuditActionJobCreated,
			ResourceType:  "job",
			ResourceID:    "job-" + newID(),
			Details:       json.RawMessage(`{"seq":` + strconv.Itoa(i) + `}`),
			RemoteIP:      "10.0.0.1",
			UserAgent:     "ChainTest/2.0",
			RequestID:     "req-" + newID(),
			TraceID:       "trace-" + newID(),
			SchemaVersion: 2,
		}
		require.NoError(t, q.CreateAuditEvent(ctx, ev))

	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t, result.
		Valid,
	)
	assert.EqualValues(t, 5, result.
		EventsChecked,
	)

}

func TestAuditForensic_DefaultsForOldEvents(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("forensic-compat-secret")
	require.NoError(t, err)

	q.SetAuditSigningKey(signingKey)

	projectID := "proj-forensic-compat-" + newID()

	ev := &domain.AuditEvent{
		ProjectID:     projectID,
		ActorID:       "user:u-1",
		ActorType:     "user",
		Action:        domain.AuditActionJobCreated,
		ResourceType:  "job",
		ResourceID:    "job-compat",
		Details:       json.RawMessage(`{"legacy":true}`),
		SchemaVersion: 1,
	}
	require.NoError(t, q.CreateAuditEvent(ctx, ev))

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t, result.
		Valid,
	)

}
