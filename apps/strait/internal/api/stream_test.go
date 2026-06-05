package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func runStreamRequestWithEnvironment(path, runID, projectID, environmentID string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("runID", runID)
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, ctxProjectIDKey, projectID)
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, environmentID)
	return r.WithContext(ctx)
}

func TestHandleRunStream_CrossProjectReturns404(t *testing.T) {
	t.Parallel()

	// Regression: an authenticated caller in project "proj-attacker" must
	// not be able to subscribe to a run owned by "proj-victim". The handler
	// should return 404 (not 200, not 403) to avoid leaking run existence.
	// This is the SSE BOLA bug — RLS does not enforce isolation on long-lived
	// SSE connections, so the handler must check application-side.
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-victim",
				Status: domain.StatusExecuting, Attempt: 1,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/victim-run/stream", "", "proj-attacker"))
	require.Equal(t, http.StatusNotFound,

		w.Code)

	body := w.Body.String()
	require.False(t, strings.Contains(body,
		"event: ") || strings.Contains(strings.ToLower(
		body), "subscribe"))
}

func TestHandleRunLogStream_CrossProjectReturns404(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-victim",
				Status: domain.StatusExecuting, Attempt: 1,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/victim-run/stream/logs", "", "proj-attacker"))
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestHandleRunStream_EnvironmentScopedCallerCannotStreamForeignEnvironment(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-staging", ProjectID: "proj-1",
				Status: domain.StatusExecuting, Attempt: 1,
			}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			require.Equal(t, "job-staging",
				id)

			return &domain.Job{ID: id, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
		},
	}
	pub := &mockPublisher{
		subscribeFn: func(context.Context, string) (*pubsub.Subscription, error) {
			require.Fail(t,

				"mismatched environment must be rejected before subscribing")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, pub)
	w := httptest.NewRecorder()
	r := runStreamRequestWithEnvironment("/v1/runs/run-1/stream", "run-1", "proj-1", "env-prod")

	srv.handleRunStream(w, r)
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestHandleRunLogStream_EnvironmentScopedCallerCannotStreamForeignEnvironment(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-staging", ProjectID: "proj-1",
				Status: domain.StatusExecuting, Attempt: 1,
			}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
		},
	}
	pub := &mockPublisher{
		subscribeFn: func(context.Context, string) (*pubsub.Subscription, error) {
			require.Fail(t,

				"mismatched environment must be rejected before subscribing")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, pub)
	w := httptest.NewRecorder()
	r := runStreamRequestWithEnvironment("/v1/runs/run-1/stream/logs", "run-1", "proj-1", "env-prod")

	srv.handleRunLogStream(w, r)
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestHandleRunStream_RunNotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/missing-run/stream", "", "proj-1"))
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestHandleRunStream_StoreError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-123/stream", "", "proj-1"))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}

func TestHandleRunStream_TerminalRun(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusCompleted,
				Attempt:   1,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-done/stream", "", "proj-1"))
	require.Equal(t, http.StatusGone,
		w.
			Code)
}

func TestHandleRunStream_NoPubSub(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
				Attempt:   1,
			}, nil
		},
	}
	// nil pubsub
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-123/stream", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.Equal(t, "text/event-stream",

		w.Header().Get("Content-Type"))

	// When pubsub is nil, handler writes SSE headers then error event

	body := w.Body.String()
	require.Contains(
		t, body, "event: error")
}

func TestHandleRunStream_RejectsWhenProjectSSELimitExceeded(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
				Attempt:   1,
			}, nil
		},
	}
	pub := &mockPublisher{
		subscribeFn: func(context.Context, string) (*pubsub.Subscription, error) {
			require.Fail(t,

				"must reject before subscribing when SSE connection cap is exhausted")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, pub)
	srv.config.SSEMaxConnsPerProject = 1
	require.True(
		t, srv.acquireSSEConn(
			"proj-1"))

	defer srv.releaseSSEConn("proj-1")

	w := httptest.NewRecorder()
	r := runStreamRequestWithEnvironment("/v1/runs/run-123/stream", "run-123", "proj-1", "")
	srv.handleRunStream(w, r)
	require.Equal(t, http.StatusServiceUnavailable,

		w.Code)
}

func TestHandleRunLogStream_RejectsWhenProjectSSELimitExceeded(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
				Attempt:   1,
			}, nil
		},
	}
	pub := &mockPublisher{
		subscribeFn: func(context.Context, string) (*pubsub.Subscription, error) {
			require.Fail(t,

				"must reject log stream before subscribing when SSE connection cap is exhausted")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, pub)
	srv.config.SSEMaxConnsPerProject = 1
	require.True(
		t, srv.acquireSSEConn(
			"proj-1"))

	defer srv.releaseSSEConn("proj-1")

	w := httptest.NewRecorder()
	r := runStreamRequestWithEnvironment("/v1/runs/run-123/stream/logs", "run-123", "proj-1", "")
	srv.handleRunLogStream(w, r)
	require.Equal(t, http.StatusServiceUnavailable,

		w.Code)
}

func TestHandleRunStream_SubscribeError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
				Attempt:   1,
			}, nil
		},
	}
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			return nil, errors.New("subscribe failed")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, pub)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-123/stream", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	// Handler writes SSE headers then error event for subscribe failure

	body := w.Body.String()
	require.Contains(
		t, body, "event: error")
}

func TestHandleRunStream_ReceivesMessage(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
				Attempt:   1,
			}, nil
		},
	}

	// Create a channel that sends one message then closes
	ch := make(chan []byte, 1)
	ch <- []byte(`{"status":"completed"}`)
	close(ch)

	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			_, cancel := context.WithCancel(context.Background())
			return pubsub.NewSubscription(ch, cancel), nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, pub)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-123/stream", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	body := w.Body.String()
	require.Contains(
		t, body, `data: {"status":"completed"}`)
}

func TestHandleRunStream_ClientDisconnect(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
				Attempt:   1,
			}, nil
		},
	}

	// Channel that blocks forever — the context cancellation should stop the handler
	ch := make(chan []byte)
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			_, cancel := context.WithCancel(context.Background())
			return pubsub.NewSubscription(ch, cancel), nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, pub)
	w := httptest.NewRecorder()

	// Create a request with a cancelled context to simulate client disconnect
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel
	r := authedProjectRequest(http.MethodGet, "/v1/runs/run-123/stream", "", "proj-1")
	r = r.WithContext(ctx)

	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	// Handler should return after seeing cancelled context
}

func TestHandleRunStream_TerminalStatuses(t *testing.T) {
	t.Parallel()

	terminalStatuses := []domain.RunStatus{
		domain.StatusCompleted,
		domain.StatusFailed,
		domain.StatusTimedOut,
		domain.StatusCanceled,
		domain.StatusExpired,
	}

	for _, status := range terminalStatuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			ms := &APIStoreMock{
				GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
					return &domain.JobRun{
						ID:        id,
						JobID:     "job-1",
						ProjectID: "proj-1",
						Status:    status,
						Attempt:   1,
					}, nil
				},
			}
			srv := newTestServer(t, ms, &mockQueue{}, nil)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/stream", "", "proj-1"))
			require.Equal(t, http.StatusGone,
				w.
					Code)
		})
	}
}

func TestHandleRunStream_SSEHeaders(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        id,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
				Attempt:   1,
			}, nil
		},
	}

	ch := make(chan []byte)
	close(ch)
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			_, cancel := context.WithCancel(context.Background())
			return pubsub.NewSubscription(ch, cancel), nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, pub)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-123/stream", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.Equal(t, "text/event-stream",

		w.Header().Get("Content-Type"))
	require.Equal(t, "no-cache",
		w.Header().Get("Cache-Control"))
	require.Equal(t, "keep-alive",
		w.Header().Get("Connection"))
	require.Equal(t, "no",
		w.Header().Get("X-Accel-Buffering"))
}

func TestHandleRunStream_NonTerminalStatuses(t *testing.T) {
	t.Parallel()

	// These are non-terminal statuses that should allow streaming
	nonTerminalStatuses := []domain.RunStatus{
		domain.StatusQueued,
		domain.StatusDequeued,
		domain.StatusExecuting,
		domain.StatusDelayed,
	}

	for _, status := range nonTerminalStatuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			ms := &APIStoreMock{
				GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
					return &domain.JobRun{
						ID:        id,
						JobID:     "job-1",
						ProjectID: "proj-1",
						Status:    status,
						Attempt:   1,
					}, nil
				},
			}

			ch := make(chan []byte)
			close(ch) // close immediately so handler exits
			pub := &mockPublisher{
				subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
					_, cancel := context.WithCancel(context.Background())
					return pubsub.NewSubscription(ch, cancel), nil
				},
			}

			srv := newTestServer(t, ms, &mockQueue{}, pub)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/stream", "", "proj-1"))
			require.NotEqual(t, http.
				StatusGone,
				w.Code)

			// Should NOT return 410 for non-terminal statuses
		})
	}
}

// Suppress unused import warnings.
var _ = time.Second
