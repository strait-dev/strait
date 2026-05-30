package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"
)

func TestValidateSingletonConfig(t *testing.T) {
	t.Parallel()

	depth := 5
	tests := []struct {
		name    string
		expr    json.RawMessage
		policy  string
		depth   *int
		wantErr bool
	}{
		{name: "no singleton", expr: nil, policy: "", depth: nil},
		{name: "valid queue with expr and depth", expr: json.RawMessage(`{"template":"${id}"}`), policy: "queue", depth: &depth},
		{name: "valid drop with expr", expr: json.RawMessage(`{"template":"global"}`), policy: "drop"},
		{name: "valid replace with expr", expr: json.RawMessage(`{"template":"${id}"}`), policy: "replace"},
		{name: "policy without expr", expr: nil, policy: "queue", wantErr: true},
		{name: "policy with null expr", expr: json.RawMessage(`null`), policy: "queue", wantErr: true},
		{name: "expr without policy", expr: json.RawMessage(`{"template":"${id}"}`), policy: "", wantErr: true},
		{name: "unknown policy with expr", expr: json.RawMessage(`{"template":"${id}"}`), policy: "skip", wantErr: true},
		{name: "uppercase policy with expr", expr: json.RawMessage(`{"template":"${id}"}`), policy: "QUEUE", wantErr: true},
		{name: "invalid expr", expr: json.RawMessage(`{"template":""}`), policy: "queue", wantErr: true},
		{name: "malformed expr", expr: json.RawMessage(`{"template":`), policy: "queue", wantErr: true},
		{name: "depth with non-queue policy", expr: json.RawMessage(`{"template":"${id}"}`), policy: "replace", depth: &depth, wantErr: true},
		{name: "depth without policy", expr: nil, policy: "", depth: &depth, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateSingletonConfig(tt.expr, tt.policy, tt.depth)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSingletonConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckSingletonOnConflict_Gating(t *testing.T) {
	t.Parallel()

	newCloudServer := func(tier domain.PlanTier) *Server {
		return &Server{
			edition: domain.EditionCloud,
			billingEnforcer: &mockHTTPModeEnforcer{
				mockBillingEnforcer: mockBillingEnforcer{
					projectOrgMap: map[string]string{"proj-1": "org-1"},
				},
				planLimits: billing.GetPlanLimits(tier),
			},
		}
	}

	t.Run("community fails open for replace", func(t *testing.T) {
		t.Parallel()
		s := &Server{edition: domain.EditionCommunity}
		if err := s.checkSingletonOnConflict(context.Background(), "proj-1", "replace"); err != nil {
			t.Fatalf("community should allow replace, got %v", err)
		}
	})

	t.Run("cloud free plan rejects replace", func(t *testing.T) {
		t.Parallel()
		s := newCloudServer(domain.PlanFree)
		err := s.checkSingletonOnConflict(context.Background(), "proj-1", "replace")
		if err == nil {
			t.Fatal("expected free plan to reject replace")
		}
		humaErr, ok := err.(interface{ GetStatus() int })
		if !ok {
			t.Fatalf("expected huma error, got %T", err)
		}
		if humaErr.GetStatus() != http.StatusForbidden {
			t.Errorf("status = %d, want 403", humaErr.GetStatus())
		}
	})

	t.Run("cloud pro plan allows replace", func(t *testing.T) {
		t.Parallel()
		s := newCloudServer(domain.PlanPro)
		if err := s.checkSingletonOnConflict(context.Background(), "proj-1", "replace"); err != nil {
			t.Fatalf("pro plan should allow replace, got %v", err)
		}
	})

	t.Run("queue and drop are never gated", func(t *testing.T) {
		t.Parallel()
		s := newCloudServer(domain.PlanFree)
		for _, policy := range []string{"queue", "drop", ""} {
			if err := s.checkSingletonOnConflict(context.Background(), "proj-1", policy); err != nil {
				t.Errorf("policy %q should not be gated, got %v", policy, err)
			}
		}
	})
}

func TestHandleCreateJob_SingletonMapping(t *testing.T) {
	t.Parallel()

	var captured *domain.Job
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			cp := *job
			captured = &cp
			job.ID = "job-singleton"
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	depth := 4
	_, err := srv.handleCreateJob(ctx, &CreateJobInput{Body: CreateJobRequest{
		ProjectID:              "proj-1",
		Name:                   "Singleton Job",
		Slug:                   "singleton-job",
		EndpointURL:            "https://example.com/hook",
		ExecutionMode:          string(domain.ExecutionModeHTTP),
		SingletonKeyExpr:       json.RawMessage(`{"template":"${account.id}"}`),
		SingletonOnConflict:    "queue",
		SingletonMaxQueueDepth: &depth,
	}})
	if err != nil {
		t.Fatalf("handleCreateJob: %v", err)
	}
	if captured == nil {
		t.Fatal("expected CreateJob to be called")
	}
	if captured.SingletonOnConflict != domain.SingletonOnConflictQueue {
		t.Errorf("SingletonOnConflict = %q, want queue", captured.SingletonOnConflict)
	}
	if captured.SingletonMaxQueueDepth == nil || *captured.SingletonMaxQueueDepth != 4 {
		t.Errorf("SingletonMaxQueueDepth = %v, want 4", captured.SingletonMaxQueueDepth)
	}
	expr, perr := domain.ParseSingletonKeyExpr(captured.SingletonKeyExpr)
	if perr != nil {
		t.Fatalf("ParseSingletonKeyExpr: %v", perr)
	}
	if expr.Template != "${account.id}" {
		t.Errorf("template = %q, want ${account.id}", expr.Template)
	}
}

func TestHandleCreateJob_SingletonValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       CreateJobRequest
		wantStatus int
	}{
		{
			// Struct-tag (oneof) violations surface as huma 422 validation errors.
			name: "invalid on-conflict enum",
			body: CreateJobRequest{
				ProjectID:           "proj-1",
				Name:                "j",
				Slug:                "j",
				EndpointURL:         "https://example.com/hook",
				ExecutionMode:       string(domain.ExecutionModeHTTP),
				SingletonKeyExpr:    json.RawMessage(`{"template":"${id}"}`),
				SingletonOnConflict: "bogus",
			},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "policy without key expr",
			body: CreateJobRequest{
				ProjectID:           "proj-1",
				Name:                "j",
				Slug:                "j",
				EndpointURL:         "https://example.com/hook",
				ExecutionMode:       string(domain.ExecutionModeHTTP),
				SingletonOnConflict: "queue",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "key expr without policy",
			body: CreateJobRequest{
				ProjectID:        "proj-1",
				Name:             "j",
				Slug:             "j",
				EndpointURL:      "https://example.com/hook",
				ExecutionMode:    string(domain.ExecutionModeHTTP),
				SingletonKeyExpr: json.RawMessage(`{"template":"${id}"}`),
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ms := &APIStoreMock{
				CreateJobFunc: func(_ context.Context, _ *domain.Job) error {
					t.Error("CreateJob should not be called on validation failure")
					return nil
				},
			}
			srv := newTestServer(t, ms, &mockQueue{}, nil)
			ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

			_, err := srv.handleCreateJob(ctx, &CreateJobInput{Body: tt.body})
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			humaErr, ok := err.(interface{ GetStatus() int })
			if !ok {
				t.Fatalf("expected huma error, got %T: %v", err, err)
			}
			if humaErr.GetStatus() != tt.wantStatus {
				t.Errorf("status = %d, want %d", humaErr.GetStatus(), tt.wantStatus)
			}
		})
	}
}

// TestSingletonEndpointsReachability guards against the failure mode that
// shipped once already: the singleton inspection operations were registered
// with Huma (so they appeared in the OpenAPI spec) but never mounted on the
// chi router that serves requests, so every call returned chi's default
// "404 page not found". Each path is hit with an authenticated request; the
// test only requires that the response is NOT that chi default 404, proving
// the endpoint is wired. Any 4xx/5xx from the handler itself is acceptable.
func TestSingletonEndpointsReachability(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
		GetWorkflowFunc: func(_ context.Context, _ string) (*domain.Workflow, error) {
			return nil, store.ErrWorkflowNotFound
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"list-job-singletons", http.MethodGet, "/v1/jobs/job-1/singletons"},
		{"list-workflow-singletons", http.MethodGet, "/v1/workflows/wf-1/singletons"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := authedProjectRequest(tc.method, tc.path, "", "proj-a")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)

			if w.Code == http.StatusNotFound && strings.Contains(w.Body.String(), "404 page not found") {
				t.Fatalf("%s %s is unreachable: chi returned default 404\nbody: %s",
					tc.method, tc.path, w.Body.String())
			}
		})
	}
}

// TestHandleListJobSingletons_ReturnsHolders exercises the wired job endpoint
// end to end: a held lock plus its waiter count must surface in the JSON body.
func TestHandleListJobSingletons_ReturnsHolders(t *testing.T) {
	t.Parallel()

	acquired := time.Now().Add(-2 * time.Minute).UTC()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-a"}, nil
		},
		ListSingletonLocksPageFunc: func(_ context.Context, _ string, kind domain.SingletonKind, ownerID string, _ int, _ *time.Time) ([]domain.SingletonLock, error) {
			if kind != domain.SingletonKindJob {
				t.Errorf("kind = %q, want job", kind)
			}
			if ownerID != "job-1" {
				t.Errorf("ownerID = %q, want job-1", ownerID)
			}
			return []domain.SingletonLock{{
				ProjectID:   "proj-a",
				Kind:        domain.SingletonKindJob,
				OwnerID:     "job-1",
				LockKey:     "tenant-42",
				HolderRunID: "run-holder",
				AcquiredAt:  acquired,
			}}, nil
		},
		CountSingletonWaitersFunc: func(_ context.Context, kind domain.SingletonKind, _ string, lockKey string) (int, error) {
			if kind != domain.SingletonKindJob || lockKey != "tenant-42" {
				t.Errorf("CountSingletonWaiters(kind=%q, key=%q) unexpected", kind, lockKey)
			}
			return 3, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	r := authedProjectRequest(http.MethodGet, "/v1/jobs/job-1/singletons", "", "proj-a")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\nbody: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"lock_key":"tenant-42"`, `"holder_run_id":"run-holder"`, `"waiters":3`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s\nbody: %s", want, body)
		}
	}
}

// TestHandleListWorkflowSingletons_ReturnsHolders is the workflow-side parity
// check for the wired endpoint.
func TestHandleListWorkflowSingletons_ReturnsHolders(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-a"}, nil
		},
		ListSingletonLocksPageFunc: func(_ context.Context, _ string, kind domain.SingletonKind, ownerID string, _ int, _ *time.Time) ([]domain.SingletonLock, error) {
			if kind != domain.SingletonKindWorkflow {
				t.Errorf("kind = %q, want workflow", kind)
			}
			if ownerID != "wf-1" {
				t.Errorf("ownerID = %q, want wf-1", ownerID)
			}
			return []domain.SingletonLock{{
				ProjectID:   "proj-a",
				Kind:        domain.SingletonKindWorkflow,
				OwnerID:     "wf-1",
				LockKey:     "region-eu",
				HolderRunID: "wfrun-holder",
				AcquiredAt:  time.Now().UTC(),
			}}, nil
		},
		CountSingletonWaitersFunc: func(_ context.Context, _ domain.SingletonKind, _ string, _ string) (int, error) {
			return 1, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	r := authedProjectRequest(http.MethodGet, "/v1/workflows/wf-1/singletons", "", "proj-a")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\nbody: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"lock_key":"region-eu"`, `"holder_run_id":"wfrun-holder"`, `"waiters":1`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s\nbody: %s", want, body)
		}
	}
}
