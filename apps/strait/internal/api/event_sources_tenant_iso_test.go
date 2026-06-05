package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestTenantIso_EventSources_ListSubs_EmptyProjectCtx_Rejected(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	_, err := srv.handleListEventSourceSubscriptions(context.Background(), &ListEventSourceSubscriptionsInput{SourceID: "src-1"})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusBadRequest,
		))
}

func TestTenantIso_EventSources_DeleteSub_EmptyProjectCtx_Rejected(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	_, err := srv.handleDeleteEventSubscription(context.Background(), &DeleteEventSubscriptionInput{SourceID: "src-1", SubID: "sub-1"})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusBadRequest,
		))
}

func TestTenantIso_EventSources_DeleteSub_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, _, _ string) (*domain.EventSource, error) {
			return nil, store.ErrEventSourceNotFound
		},
		DeleteEventSubscriptionFunc: func(_ context.Context, _ string) error {
			require.Fail(t,

				"DeleteEventSubscription must not be called for cross-project delete")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleDeleteEventSubscription(ctx, &DeleteEventSubscriptionInput{SourceID: "src-foreign", SubID: "sub-1"})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusNotFound,
		))
}

func TestEventDispatch_JobsWriteDoesNotTriggerJobSubscription(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
			return &domain.EventSource{ID: "src-1", ProjectID: projectID, Name: name, Enabled: true}, nil
		},
		ListEventSubscriptionsBySourceFunc: func(context.Context, string) ([]domain.EventSubscription, error) {
			return []domain.EventSubscription{{
				ID:         "sub-1",
				SourceID:   "src-1",
				TargetType: "job",
				TargetID:   "job-1",
				Enabled:    true,
			}}, nil
		},
		GetJobFunc: func(context.Context, string) (*domain.Job, error) {
			require.Fail(t,

				"GetJob must not be called when caller lacks jobs:trigger")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(context.Context, *domain.JobRun) error {
			require.Fail(t,

				"event dispatch must not enqueue with only jobs:write")
			return nil
		},
	}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:jobs-write")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsWrite})

	out, err := srv.handleDispatchEvent(ctx, &DispatchEventInput{Body: DispatchEventRequest{
		Source:    "source-1",
		ProjectID: "proj-1",
		Payload:   []byte(`{"kind":"deploy"}`),
	}})
	require.NoError(t, err)
	require.EqualValues(t, 0, out.Body["dispatched"])
}

func TestEventDispatch_EnforcesJobRateLimitGuardrail(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
			return &domain.EventSource{ID: "src-1", ProjectID: projectID, Name: name, Enabled: true}, nil
		},
		ListEventSubscriptionsBySourceFunc: func(context.Context, string) ([]domain.EventSubscription, error) {
			return []domain.EventSubscription{{
				ID:         "sub-1",
				SourceID:   "src-1",
				TargetType: "job",
				TargetID:   "job-1",
				Enabled:    true,
			}}, nil
		},
		GetJobFunc: func(context.Context, string) (*domain.Job, error) {
			return &domain.Job{
				ID:                  "job-1",
				ProjectID:           "proj-1",
				Enabled:             true,
				RateLimitMax:        1,
				RateLimitWindowSecs: 60,
			}, nil
		},
		CountRunsForJobSinceFunc: func(context.Context, string, time.Time) (int, error) {
			return 1, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(context.Context, *domain.JobRun) error {
			require.Fail(t,

				"event dispatch must not enqueue when job rate limit is exceeded")
			return nil
		},
	}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:jobs-trigger")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsTrigger})

	out, err := srv.handleDispatchEvent(ctx, &DispatchEventInput{Body: DispatchEventRequest{
		Source:    "source-1",
		ProjectID: "proj-1",
		Payload:   []byte(`{"kind":"deploy"}`),
	}})
	require.NoError(t, err)
	require.EqualValues(t, 0, out.Body["dispatched"])
}
