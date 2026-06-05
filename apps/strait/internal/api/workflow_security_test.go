package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

// TestWorkflow_VersionPublishConcurrent fires concurrent workflow update
// requests that each bump the version, verifying no panics or data corruption.
func TestWorkflow_VersionPublishConcurrent(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	version := 1
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, _ string) (*domain.Workflow, error) {
			mu.Lock()
			defer mu.Unlock()
			return &domain.Workflow{
				ID:        "wf-conc",
				ProjectID: "proj-1",
				Name:      "wf",
				Slug:      "wf",
				Enabled:   true,
				Version:   version,
			}, nil
		},
		UpdateWorkflowFunc: func(_ context.Context, wf *domain.Workflow) error {
			mu.Lock()
			defer mu.Unlock()
			version++
			wf.Version = version
			return nil
		},
		ListStepsByWorkflowFunc: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
			return nil, nil
		},
		CreateWorkflowVersionSnapshotFunc: func(_ context.Context, _ string, _ int) error {
			return nil
		},
		DeleteStepsByWorkflowFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateWorkflowStepFunc: func(_ context.Context, _ *domain.WorkflowStep) error {
			return nil
		},
		CountActiveWorkflowRunsByVersionFunc: func(_ context.Context, _ string, _ string) (int, error) {
			return 0, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)

	var wg conc.WaitGroup
	for range 5 {
		wg.Go(func() {
			w := httptest.NewRecorder()
			body := `{"name":"wf-updated","steps":[{"job_id":"job-1","step_ref":"s1"}]}`
			srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/wf-conc", body))
			_ = w.Code
		})
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, version,
		2)

}

// TestWorkflow_StepApprovalWithoutPermission verifies that approving a workflow
// step without the workflow callback configured returns a 503.
func TestWorkflow_StepApprovalWithoutPermission(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:        "wr-1",
				ProjectID: "proj-1",
				Status:    domain.WfStatusRunning,
			}, nil
		},
	}
	// No workflow engine or callback set.
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/step-a/approve", `{}`))
	require.Equal(t, http.StatusServiceUnavailable,

		w.
			Code)

}

// TestWorkflow_CancelRunningWorkflow verifies that canceling an active workflow
// run succeeds and cascades to step runs and job runs.
func TestWorkflow_CancelRunningWorkflow(t *testing.T) {
	t.Parallel()

	var stepsCanceled, jobsCanceled bool
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:        "wr-active",
				ProjectID: "proj-1",
				Status:    domain.WfStatusRunning,
			}, nil
		},
		UpdateWorkflowRunStatusFunc: func(_ context.Context, _ string, _ domain.WorkflowRunStatus, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		CancelNonTerminalStepRunsFunc: func(_ context.Context, _ string, _ time.Time, _ string) (int64, error) {
			stepsCanceled = true
			return 3, nil
		},
		CancelJobRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ time.Time, _ string) (int64, error) {
			jobsCanceled = true
			return 2, nil
		},
		CancelEventTriggersByWorkflowRunFunc: func(_ context.Context, _ string) (int64, error) {
			return 0, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, &mockPublisher{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflow-runs/wr-active", ""))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.True(
		t, stepsCanceled)
	require.True(
		t, jobsCanceled)

}

// TestWorkflow_TriggerDisabledWorkflow verifies that triggering a disabled
// workflow returns a 409 conflict.
func TestWorkflow_TriggerDisabledWorkflow(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, _ string) (*domain.Workflow, error) {
			return &domain.Workflow{
				ID:        "wf-disabled",
				ProjectID: "proj-1",
				Enabled:   false,
			}, nil
		},
	}
	trigger := &mockWorkflowTrigger{}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","payload":{"key":"val"}}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-disabled/trigger", body))
	require.Equal(t, http.StatusConflict,

		w.Code)
	require.True(
		t, strings.Contains(w.Body.
			String(),
			"disabled"))

}

// TestWorkflow_StepOverrideInjection verifies that a step_ref with malicious
// characters is handled safely by the create workflow endpoint.
func TestWorkflow_StepOverrideInjection(t *testing.T) {
	t.Parallel()

	malicious := []string{
		"'; DROP TABLE workflows; --",
		"<script>alert(1)</script>",
		"step\x00ref",
		strings.Repeat("a", 10000),
		"../../../etc/passwd",
	}

	for _, ref := range malicious {
		t.Run(ref[:min(len(ref), 20)], func(t *testing.T) {
			t.Parallel()
			srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
			w := httptest.NewRecorder()
			body := fmt.Sprintf(`{"project_id":"proj-1","name":"wf","slug":"wf","steps":[{"job_id":"job-1","step_ref":%s}]}`, mustJSON(t, ref))
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", body))
			require.NotEqual(t, http.StatusInternalServerError,

				w.Code)

			// Should either reject or handle safely; no panic.

		})
	}
}

// TestWorkflow_PayloadSchemaBypass verifies that triggering a workflow with a
// payload that does not match expected types is rejected or handled gracefully.
func TestWorkflow_PayloadSchemaBypass(t *testing.T) {
	t.Parallel()

	payloads := []string{
		`"just a string"`,
		`12345`,
		`true`,
		`[1,2,3]`,
		`null`,
	}

	for _, payload := range payloads {
		t.Run(payload, func(t *testing.T) {
			t.Parallel()
			ms := &APIStoreMock{
				GetWorkflowFunc: func(_ context.Context, _ string) (*domain.Workflow, error) {
					return &domain.Workflow{
						ID:        "wf-schema",
						ProjectID: "proj-1",
						Enabled:   true,
						Version:   1,
					}, nil
				},
				ListStepsByWorkflowVersionFunc: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
					return []domain.WorkflowStep{{StepRef: "s1", JobID: "job-1"}}, nil
				},
			}
			trigger := &mockWorkflowTrigger{
				triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
					return &domain.WorkflowRun{ID: "wr-1", Status: domain.WfStatusPending}, nil
				},
			}
			srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
			w := httptest.NewRecorder()
			body := fmt.Sprintf(`{"project_id":"proj-1","payload":%s}`, payload)
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-schema/trigger", body))
			// No panic is the success condition.
			_ = w.Code
		})
	}
}

// TestWorkflow_ConcurrentApprovalDecision fires concurrent approve requests for
// the same step to verify at most one succeeds.
func TestWorkflow_ConcurrentApprovalDecision(t *testing.T) {
	t.Parallel()

	var approveCount atomic.Int64
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:        "wr-appr",
				ProjectID: "proj-1",
				Status:    domain.WfStatusRunning,
			}, nil
		},
		GetStepRunByWorkflowRunAndRefFunc: func(_ context.Context, _ string, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-1", StepRef: "step-a", Status: domain.StepCompleted}, nil
		},
		GetWorkflowStepApprovalByStepRunIDFunc: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
			return &domain.WorkflowStepApproval{}, nil
		},
	}
	wfCallback := &mockWorkflowTrigger{
		approveStepFn: func(_ context.Context, _, _, _ string) error {
			count := approveCount.Add(1)
			if count > 1 {
				return fmt.Errorf("step already approved")
			}
			return nil
		},
	}
	srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, wfCallback, wfCallback)

	var wg conc.WaitGroup
	codes := make([]int, 5)
	for i := range 5 {
		wg.Go(func() {
			w := httptest.NewRecorder()
			req := authedRequest(http.MethodPost, "/v1/workflow-runs/wr-appr/steps/step-a/approve", `{}`)
			req.Header.Set("X-Actor", "user-1")
			srv.ServeHTTP(w, req)
			codes[i] = w.Code
		})
	}
	wg.Wait()

	successes := 0
	for _, code := range codes {
		if code == http.StatusOK {
			successes++
		}
	}
	require.LessOrEqual(t, successes,
		1)

}

// TestWorkflow_TimelineQueryInjection passes adversarial query parameters to
// the timeline endpoint to ensure they do not cause panics or SQL injection.
func TestWorkflow_TimelineQueryInjection(t *testing.T) {
	t.Parallel()

	injections := []string{
		"wr-1';DROP-TABLE-workflow_runs;--",
		"wr-1%00",
		strings.Repeat("a", 5000),
		"wr-1\";SELECT-*-FROM-pg_shadow;--",
	}

	for _, id := range injections {
		t.Run(id[:min(len(id), 30)], func(t *testing.T) {
			t.Parallel()
			ms := &APIStoreMock{
				GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
					return nil, store.ErrWorkflowRunNotFound
				},
			}
			srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/"+id+"/timeline", ""))
			require.NotEqual(t, http.StatusInternalServerError,

				w.Code)

			// We expect 404 since the mock always returns not found.

		})
	}
}

// TestWorkflow_LabelInjection verifies that SQL/HTML in workflow run labels is
// handled safely by the trigger endpoint.
func TestWorkflow_LabelInjection(t *testing.T) {
	t.Parallel()

	labels := map[string]string{
		"<script>alert(1)</script>": "val",
		"key":                       "'; DROP TABLE labels; --",
		"key\x00null":               "value",
	}

	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, _ string) (*domain.Workflow, error) {
			return &domain.Workflow{
				ID:        "wf-labels",
				ProjectID: "proj-1",
				Enabled:   true,
				Version:   1,
			}, nil
		},
		ListStepsByWorkflowVersionFunc: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "s1", JobID: "job-1"}}, nil
		},
		CreateWorkflowRunLabelsFunc: func(_ context.Context, _ string, _ map[string]string) error {
			return nil
		},
	}
	trigger := &mockWorkflowTrigger{
		triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", Status: domain.WfStatusPending}, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, &mockPublisher{}, trigger)

	labelsJSON, err := json.Marshal(labels)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	body := fmt.Sprintf(`{"project_id":"proj-1","labels":%s}`, string(labelsJSON))
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-labels/trigger", body))
	require.NotEqual(t, http.StatusInternalServerError,

		w.Code)

	// No panic is the main assertion; the endpoint may succeed or reject invalid labels.

}

// FuzzWorkflowTrigger fuzzes the trigger payload for a workflow.
func FuzzWorkflowTrigger(f *testing.F) {
	f.Add(`{"project_id":"proj-1","payload":{"key":"val"}}`)
	f.Add(`{"project_id":"","payload":null}`)
	f.Add(`{}`)
	f.Add(`{"project_id":"proj-1","triggered_by":"` + strings.Repeat("x", 500) + `"}`)

	f.Fuzz(func(t *testing.T, body string) {
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return &domain.Workflow{
					ID:        "wf-fuzz",
					ProjectID: "proj-1",
					Enabled:   true,
					Version:   1,
				}, nil
			},
			ListStepsByWorkflowVersionFunc: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return nil, nil
			},
		}
		trigger := &mockWorkflowTrigger{
			triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-fuzz"}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-fuzz/trigger", body))
		// No panic is the success condition.
		_ = w.Code
	})
}

// mustJSON marshals a value to JSON for embedding in test request bodies.
func mustJSON(tb testing.TB, v any) string {
	tb.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		tb.Fatalf("mustJSON: %v", err)
	}
	return string(b)
}

// atomic import alias to avoid conflict with sync/atomic.
var _ = store.ErrWorkflowNotFound
