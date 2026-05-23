package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
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
