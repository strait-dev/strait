package api

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRotateAuditSigningKey_RequiresAdmin(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		RotateAuditSigningKeyFunc: func(_ context.Context, _, _ string) (int, error) {
			assert.Fail(t,

				"store must not be called when caller is not admin")
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleRotateAuditSigningKey(nonAdminCtx("proj-a"), &RotateAuditSigningKeyInput{ID: "proj-a"})
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.
			Error(), "admin",
		))

}

func TestRotateAuditSigningKey_RejectsCrossTenant(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		RotateAuditSigningKeyFunc: func(_ context.Context, _, _ string) (int, error) {
			assert.Fail(t,

				"store must not be called on cross-tenant request")
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleRotateAuditSigningKey(adminCtx("proj-a"), &RotateAuditSigningKeyInput{ID: "proj-b"})
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.
			Error(), "not found",
		))

}

func TestRotateAuditSigningKey_ReturnsEpochs(t *testing.T) {
	t.Parallel()

	var (
		seenProject atomic.Value
		seenActor   atomic.Value
	)
	ms := &APIStoreMock{
		RotateAuditSigningKeyFunc: func(_ context.Context, projectID, actorID string) (int, error) {
			seenProject.Store(projectID)
			seenActor.Store(actorID)
			// Bootstrap case: previous epoch 0, new epoch 1.
			return 1, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	out, err := srv.handleRotateAuditSigningKey(adminCtx("proj-a"), &RotateAuditSigningKeyInput{ID: "proj-a"})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.EqualValues(t, 1, out.
		Body.NewEpoch,
	)
	assert.EqualValues(t, 0, out.
		Body.PreviousEpoch,
	)

	if got, _ := seenProject.Load().(string); got != "proj-a" {
		assert.Failf(t, "test failure",

			"store called with project %q, want proj-a", got)
	}
	if got, _ := seenActor.Load().(string); got != "internal:admin" {
		assert.Failf(t, "test failure",

			"store called with actor %q, want internal:admin", got)
	}
}

func TestRotateAuditSigningKey_StorePropagatesError(t *testing.T) {
	t.Parallel()

	// Simulate a store-layer SQL-ish failure. The handler must surface a
	// generic 500 without leaking the underlying error message into the
	// response body.
	sqlLike := errors.New("pq: duplicate key value violates unique constraint audit_events_pkey")
	ms := &APIStoreMock{
		RotateAuditSigningKeyFunc: func(_ context.Context, _, _ string) (int, error) {
			return 0, sqlLike
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleRotateAuditSigningKey(adminCtx("proj-a"), &RotateAuditSigningKeyInput{ID: "proj-a"})
	require.Error(t, err)
	assert.False(
		t, strings.Contains(err.
			Error(), "pq:",
		) || strings.Contains(err.Error(), "audit_events_pkey"))

}
