package api

import (
	"context"
	"net/http"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestTenantIso_EventSources_ListSubs_EmptyProjectCtx_Rejected(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	_, err := srv.handleListEventSourceSubscriptions(context.Background(), &ListEventSourceSubscriptionsInput{SourceID: "src-1"})
	if !isHumaStatusError(err, http.StatusBadRequest) {
		t.Fatalf("expected 400, got %v", err)
	}
}

func TestTenantIso_EventSources_DeleteSub_EmptyProjectCtx_Rejected(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	_, err := srv.handleDeleteEventSubscription(context.Background(), &DeleteEventSubscriptionInput{SourceID: "src-1", SubID: "sub-1"})
	if !isHumaStatusError(err, http.StatusBadRequest) {
		t.Fatalf("expected 400, got %v", err)
	}
}

func TestTenantIso_EventSources_DeleteSub_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, _, _ string) (*domain.EventSource, error) {
			return nil, store.ErrEventSourceNotFound
		},
		DeleteEventSubscriptionFunc: func(_ context.Context, _ string) error {
			t.Fatal("DeleteEventSubscription must not be called for cross-project delete")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleDeleteEventSubscription(ctx, &DeleteEventSubscriptionInput{SourceID: "src-foreign", SubID: "sub-1"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404, got %v", err)
	}
}
