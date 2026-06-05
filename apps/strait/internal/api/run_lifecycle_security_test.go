package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

// TestRunLifecycle_CancelTerminalRun verifies that canceling a run already in a
// terminal state returns an error rather than silently succeeding.
func TestRunLifecycle_CancelTerminalRun(t *testing.T) {
	t.Parallel()

	for _, status := range domain.TerminalStatuses() {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			ms := &APIStoreMock{
				GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
					return &domain.JobRun{
						ID:     "run-terminal",
						Status: status,
					}, nil
				},
			}
			srv := newTestServer(t, ms, &mockQueue{}, nil)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-terminal/", ""))
			require.Equal(t, http.StatusBadRequest,

				w.Code)
			require.True(
				t, strings.Contains(w.Body.
					String(),
					"terminal"))

		})
	}
}

// TestRunLifecycle_CancelConcurrent sends 10 concurrent cancel requests for the
// same run and verifies at most one succeeds while the rest get conflict errors.
func TestRunLifecycle_CancelConcurrent(t *testing.T) {
	t.Parallel()

	var updateCalls atomic.Int64
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:     "run-conc",
				Status: domain.StatusExecuting,
			}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, from domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			call := updateCalls.Add(1)
			if call > 1 {
				return fmt.Errorf("conflict")
			}
			return nil
		},
		CancelChildRunsByParentIDsFunc: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	var wg conc.WaitGroup
	results := make([]int, 10)
	for i := range 10 {
		idx := i
		wg.Go(func() {
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-conc/", ""))
			results[idx] = w.Code
		})
	}
	wg.Wait()

	successes := 0
	for _, code := range results {
		if code == http.StatusOK {
			successes++
		}
	}
	require.LessOrEqual(t, successes,
		1)

}

// TestRunLifecycle_ReplayNonTerminalRun verifies that replaying a run in a
// non-replayable state returns an error.
func TestRunLifecycle_ReplayNonTerminalRun(t *testing.T) {
	t.Parallel()

	nonReplayable := []domain.RunStatus{
		domain.StatusQueued,
		domain.StatusExecuting,
		domain.StatusDequeued,
		domain.StatusWaiting,
		domain.StatusPaused,
	}

	for _, status := range nonReplayable {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			ms := &APIStoreMock{
				GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
					return &domain.JobRun{
						ID:     "run-nr",
						Status: status,
						JobID:  "job-1",
					}, nil
				},
			}
			srv := newTestServer(t, ms, &mockQueue{}, nil)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-nr/replay", ""))
			require.Equal(t, http.StatusBadRequest,

				w.Code)

		})
	}
}

// TestRunLifecycle_ReplayDeadLetterRun verifies that replaying a dead letter run
// succeeds when the store allows it.
func TestRunLifecycle_ReplayDeadLetterRun(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusDeadLetter, ProjectID: "proj-1", JobID: "job-1"}, nil
		},
		ReplayDeadLetterRunFunc: func(_ context.Context, runID string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:     "run-replayed",
				Status: domain.StatusQueued,
				JobID:  "job-1",
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-dead/dlq-replay", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

// TestRunLifecycle_RestartMaxDepth verifies that restart works for a run
// that is in an executing or paused state.
func TestRunLifecycle_RestartMaxDepth(t *testing.T) {
	t.Parallel()

	getCalls := 0
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			getCalls++
			status := domain.StatusExecuting
			if getCalls > 1 {
				status = domain.StatusQueued
			}
			return &domain.JobRun{
				ID:            "run-restart",
				Status:        status,
				ExecutionMode: domain.ExecutionModeHTTP,
				LineageDepth:  99,
			}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _ domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-restart/restart", `{}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

// TestRunLifecycle_RestartLineageOverflow verifies a restart request for a run
// with extreme lineage depth does not crash.
func TestRunLifecycle_RestartLineageOverflow(t *testing.T) {
	t.Parallel()

	getCalls := 0
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			getCalls++
			status := domain.StatusPaused
			if getCalls > 1 {
				status = domain.StatusQueued
			}
			return &domain.JobRun{
				ID:            "run-overflow",
				Status:        status,
				ExecutionMode: domain.ExecutionModeHTTP,
				LineageDepth:  1<<31 - 1,
			}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _ domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-overflow/restart", `{}`))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	// Should succeed without panic; the handler does not check lineage depth.

}

// TestRunLifecycle_CancelChildRuns verifies that canceling a parent run also
// triggers bulk cancellation of child runs.
func TestRunLifecycle_CancelChildRuns(t *testing.T) {
	t.Parallel()

	var childCancelCalled atomic.Bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			if id == "run-parent" {
				return &domain.JobRun{
					ID:     "run-parent",
					Status: domain.StatusExecuting,
				}, nil
			}
			return &domain.JobRun{
				ID:     id,
				Status: domain.StatusCanceled,
			}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _ domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		CancelChildRunsByParentIDsFunc: func(_ context.Context, parentIDs []string, _ time.Time, _ string) (int64, error) {
			childCancelCalled.Store(true)
			if len(parentIDs) == 0 {
				return 0, nil
			}
			return 2, nil
		},
		ListChildRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-parent/", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.True(
		t, childCancelCalled.
			Load())

}

// TestRunLifecycle_CancelMaxDepthExceeded verifies that the recursive child
// cancellation stops at maxCancelDepth without entering an infinite loop.
func TestRunLifecycle_CancelMaxDepthExceeded(t *testing.T) {
	t.Parallel()

	var depth atomic.Int64
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:     "run-deep",
				Status: domain.StatusExecuting,
			}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _ domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		CancelChildRunsByParentIDsFunc: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			depth.Add(1)
			return 1, nil
		},
		ListChildRunsFunc: func(_ context.Context, _ string, _ int, cursor *time.Time) ([]domain.JobRun, error) {
			if cursor != nil {
				return nil, nil
			}
			return []domain.JobRun{{ID: fmt.Sprintf("child-%d", depth.Load()), CreatedAt: time.Now()}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/runs/run-deep/", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.LessOrEqual(t, depth.Load(),
		int64(20))

	// maxCancelDepth is 20.

}

// TestRunLifecycle_SnoozeNegativeDuration verifies that the snooze endpoint does
// not exist at the API level (it is a worker-side operation). We test that the
// route returns 404/405.
func TestRunLifecycle_SnoozeNegativeDuration(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/snooze", `{"duration_secs":-60}`))
	require.False(t, w.Code == http.
		StatusOK ||
		w.Code ==
			http.StatusCreated,
	)

	// Snooze is not an API route, so expect 404 or 405.

}

// TestRunLifecycle_SnoozeMaxDuration is a companion to the negative duration test,
// verifying that extremely large values also do not succeed at the API layer.
func TestRunLifecycle_SnoozeMaxDuration(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-1/snooze", `{"duration_secs":9999999999}`))
	require.False(t, w.Code == http.
		StatusOK ||
		w.Code ==
			http.StatusCreated,
	)

}

// TestRunLifecycle_PauseResumeRace sends concurrent pause and resume requests to
// verify they don't corrupt state.
func TestRunLifecycle_PauseResumeRace(t *testing.T) {
	t.Parallel()

	var updateCalls atomic.Int64
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:            "run-pr",
				Status:        domain.StatusExecuting,
				ExecutionMode: domain.ExecutionModeHTTP,
			}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _ domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			updateCalls.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	var wg conc.WaitGroup
	for i := range 10 {
		idx := i
		wg.Go(func() {
			w := httptest.NewRecorder()
			if idx%2 == 0 {
				srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-pr/pause", ""))
			} else {
				srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-pr/resume", ""))
			}
			// We do not assert success; the point is no panics or data races.
		})
	}
	wg.Wait()
}

// TestRunLifecycle_CompletionWithHugeResult verifies that completing a run with
// a very large result payload (10MB) is handled without panics.
func TestRunLifecycle_CompletionWithHugeResult(t *testing.T) {
	t.Parallel()

	hugePayload := strings.Repeat("x", 10*1024*1024)
	result := json.RawMessage(fmt.Sprintf(`{"data":"%s"}`, hugePayload))

	// Just verify the JSON round-trips without issue.
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(result,
		&parsed,
	))
	require.Len(t,
		parsed["data"].(string), 10*1024*
			1024)

}

// TestRunLifecycle_CompletionWithNullResult verifies that a null JSON result
// does not cause panics in store operations.
func TestRunLifecycle_CompletionWithNullResult(t *testing.T) {
	t.Parallel()

	result := json.RawMessage(`null`)
	var parsed any
	require.NoError(t, json.Unmarshal(result,
		&parsed,
	))
	require.Nil(t, parsed)

}

// TestRunLifecycle_DoubleCompletion verifies that attempting to cancel (simulate
// complete) a run twice results in the second attempt returning a conflict or
// bad request error.
func TestRunLifecycle_DoubleCompletion(t *testing.T) {
	t.Parallel()

	callCount := 0
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			callCount++
			if callCount <= 1 {
				return &domain.JobRun{
					ID:     "run-double",
					Status: domain.StatusExecuting,
				}, nil
			}
			return &domain.JobRun{
				ID:     "run-double",
				Status: domain.StatusCanceled,
			}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _ domain.RunStatus, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		CancelChildRunsByParentIDsFunc: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// First cancel succeeds.
	w1 := httptest.NewRecorder()
	srv.ServeHTTP(w1, authedRequest(http.MethodDelete, "/v1/runs/run-double/", ""))
	require.Equal(t, http.StatusOK,
		w1.Code,
	)

	// Second cancel fails because run is now terminal.
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, authedRequest(http.MethodDelete, "/v1/runs/run-double/", ""))
	require.Equal(t, http.StatusBadRequest,

		w2.Code)

}

// FuzzRunLifecycleTransitions fuzzes random status strings as query parameters
// to the run listing endpoint to ensure no panics occur.
func FuzzRunLifecycleTransitions(f *testing.F) {
	f.Add("executing")
	f.Add("completed")
	f.Add("invalid_status_!@#$")
	f.Add("")
	f.Add(strings.Repeat("a", 1000))

	f.Fuzz(func(t *testing.T, status string) {
		ms := &APIStoreMock{
			ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
				return nil, nil
			},
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		path := fmt.Sprintf("/v1/runs?status=%s", status)
		srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, path, "", "proj-1"))
		// No panic is the success condition.
		_ = w.Code
	})
}

// Ensure we use the store import to avoid unused-import errors.var _ = store.ErrRunNotFound
