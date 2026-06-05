package api

import (
	"context"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// unmarshalable is a sentinel value json.Marshal cannot encode (channels
// have no JSON representation), used to deterministically trigger a
// marshalAndCapDetails failure inside buildAuditEvent.
type unmarshalableForAudit struct {
	Ch chan int `json:"ch"`
}

// TestBuildAuditEventReturnsMarshalErrorOnInvalidDetails is the regression
// guard for fix #5: previously buildAuditEvent silently swallowed marshal
// failures (return nil, false) so a tx caller would commit the surrounding
// mutation without an audit row. The new contract returns errAuditDetailsMarshal
// so tx callers can roll back.
func TestBuildAuditEventReturnsMarshalErrorOnInvalidDetails(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:alice")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")

	ev, err := srv.buildAuditEvent(ctx, domain.AuditActionRoleCreated, "role", "r-1", map[string]any{
		"bad": unmarshalableForAudit{Ch: make(chan int)},
	})
	require.Nil(t, ev)
	require.ErrorIs(
		t, err, errAuditDetailsMarshal)
}

// TestBuildAuditEventReturnsNilNilOnIntentionalSkip pins the other half
// of the contract: validation skips (config nil, unknown action, missing
// actor on authenticated request) must NOT surface as errors so
// fire-and-forget callers can keep going.
func TestBuildAuditEventReturnsNilNilOnIntentionalSkip(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	// Unknown action -> (nil, nil).
	ev, err := srv.buildAuditEvent(context.Background(), "definitely.not.a.real.action", "x", "y", nil)
	require.False(t, ev !=
		nil || err !=
		nil)

	// Authenticated request with missing actor (api_key with no ID) -> (nil, nil).
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ev, err = srv.buildAuditEvent(ctx, domain.AuditActionRoleCreated, "role", "r-1", nil)
	require.False(t, ev !=
		nil || err !=
		nil)
}

// TestEmitAuditEventDropsOnMarshalFailure regresses the fire-and-forget
// path: a marshal failure must not crash the caller and must not write a
// half-formed audit event. We assert the store is never invoked.
func TestEmitAuditEventDropsOnMarshalFailure(t *testing.T) {
	t.Parallel()

	var writes int
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			writes++
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:alice")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")

	srv.emitAuditEvent(ctx, domain.AuditActionRoleCreated, "role", "r-1", map[string]any{
		"bad": unmarshalableForAudit{Ch: make(chan int)},
	})
	require.Equal(t, 0, writes)
}
