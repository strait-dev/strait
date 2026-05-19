package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// dispatchRecordingEnforcer captures DispatchBilling calls so plan-gate tests
// can assert that workflow.registration_rejected fires at each rejection site.
type dispatchRecordingEnforcer struct {
	tunableLimitsEnforcer

	mu    sync.Mutex
	calls []dispatchCall
}

type dispatchCall struct {
	orgID     string
	planTier  domain.PlanTier
	eventType string
	detail    map[string]any
}

func (d *dispatchRecordingEnforcer) DispatchBilling(_ context.Context, orgID string, tier domain.PlanTier, eventType string, detail map[string]any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, dispatchCall{
		orgID:     orgID,
		planTier:  tier,
		eventType: eventType,
		detail:    detail,
	})
}

func (d *dispatchRecordingEnforcer) snapshot() []dispatchCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]dispatchCall, len(d.calls))
	copy(out, d.calls)
	return out
}

// TestPlanGate_DispatchesWorkflowRegistrationRejected_RunTTL exercises the
// run-TTL gate over its cap and asserts a workflow.registration_rejected
// event is dispatched with reason=run_ttl_limit and the requested+cap pair.
func TestPlanGate_DispatchesWorkflowRegistrationRejected_RunTTL(t *testing.T) {
	t.Parallel()

	limits := freeLimits()
	enforcer := &dispatchRecordingEnforcer{
		tunableLimitsEnforcer: tunableLimitsEnforcer{limits: limits},
	}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	maxTTL := limits.RetentionDays * 86400
	if err := srv.checkRunTTLLimit(context.Background(), "proj-1", maxTTL+1); err == nil {
		t.Fatal("expected rejection above run TTL cap")
	}

	calls := enforcer.snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(calls))
	}
	c := calls[0]
	if c.eventType != domain.WebhookEventWorkflowRegistrationRejected {
		t.Errorf("event type: got %q, want %q", c.eventType, domain.WebhookEventWorkflowRegistrationRejected)
	}
	if c.planTier != limits.PlanTier {
		t.Errorf("plan tier: got %q, want %q", c.planTier, limits.PlanTier)
	}
	if c.detail["reason"] != "run_ttl_limit" {
		t.Errorf("reason: got %v, want run_ttl_limit", c.detail["reason"])
	}
	if c.detail["requested_value"] != maxTTL+1 {
		t.Errorf("requested_value: got %v, want %d", c.detail["requested_value"], maxTTL+1)
	}
	if c.detail["cap"] != maxTTL {
		t.Errorf("cap: got %v, want %d", c.detail["cap"], maxTTL)
	}
}

// TestPlanGate_DispatchesWorkflowRegistrationRejected_PerJobConcurrency walks
// the per-job concurrency gate past the org cap and asserts the dispatch.
func TestPlanGate_DispatchesWorkflowRegistrationRejected_PerJobConcurrency(t *testing.T) {
	t.Parallel()

	limits := freeLimits()
	enforcer := &dispatchRecordingEnforcer{
		tunableLimitsEnforcer: tunableLimitsEnforcer{limits: limits},
	}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	overCap := limits.MaxConcurrentRuns + 1
	if err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", overCap, 0); err == nil {
		t.Fatal("expected rejection above per-job concurrency cap")
	}

	calls := enforcer.snapshot()
	if len(calls) != 1 || calls[0].detail["reason"] != "per_job_concurrency" {
		t.Fatalf("expected per_job_concurrency dispatch, got %+v", calls)
	}
	if calls[0].detail["cap"] != limits.MaxConcurrentRuns {
		t.Errorf("cap: got %v, want %d", calls[0].detail["cap"], limits.MaxConcurrentRuns)
	}
}

// TestPlanGate_AllowedRequest_NoDispatch confirms that a request that passes
// the gate does not fire a dispatch (negative path; guards against accidental
// every-request dispatch).
func TestPlanGate_AllowedRequest_NoDispatch(t *testing.T) {
	t.Parallel()

	limits := freeLimits()
	enforcer := &dispatchRecordingEnforcer{
		tunableLimitsEnforcer: tunableLimitsEnforcer{limits: limits},
	}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	maxTTL := limits.RetentionDays * 86400
	if err := srv.checkRunTTLLimit(context.Background(), "proj-1", maxTTL); err != nil {
		t.Fatalf("at-cap TTL must not error: %v", err)
	}
	if got := len(enforcer.snapshot()); got != 0 {
		t.Errorf("expected zero dispatches on allowed request, got %d", got)
	}
}

// TestPlanGate_DispatchesWorkflowRegistrationRejected_CronOverlapPolicy
// rejects a non-"allow" policy on Free tier and asserts the dispatch carries
// the requested policy value.
func TestPlanGate_DispatchesWorkflowRegistrationRejected_CronOverlapPolicy(t *testing.T) {
	t.Parallel()

	limits := billing.GetPlanLimits(domain.PlanFree)
	enforcer := &dispatchRecordingEnforcer{
		tunableLimitsEnforcer: tunableLimitsEnforcer{limits: limits},
	}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	if err := srv.checkCronOverlapPolicy(context.Background(), "proj-1", "skip"); err == nil {
		t.Fatal("free tier must reject non-allow overlap policy")
	}
	calls := enforcer.snapshot()
	if len(calls) != 1 || calls[0].detail["reason"] != "cron_overlap_policy" || calls[0].detail["requested_value"] != "skip" {
		t.Fatalf("expected cron_overlap_policy dispatch with requested=skip, got %+v", calls)
	}
}

func TestPlanGate_BatchCreateRejectsCronOverlapPolicy(t *testing.T) {
	t.Parallel()

	limits := billing.GetPlanLimits(domain.PlanFree)
	enforcer := &dispatchRecordingEnforcer{
		tunableLimitsEnforcer: tunableLimitsEnforcer{limits: limits},
	}
	ms := &APIStoreMock{
		CreateJobFunc: func(context.Context, *domain.Job) error {
			t.Fatal("CreateJob must not be called for a batch item rejected by cron overlap policy")
			return nil
		},
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	_, err := srv.handleBatchCreateJobs(context.Background(), &BatchCreateJobsInput{Body: BatchCreateJobsRequest{
		Jobs: []CreateJobRequest{{
			ProjectID:         "proj-1",
			Name:              "batch overlap",
			Slug:              "batch-overlap",
			EndpointURL:       "https://example.com/run",
			CronOverlapPolicy: "skip",
		}},
	}})
	if err == nil {
		t.Fatal("expected batch create to fail when every item violates cron overlap policy")
	}
	var rse *rawStatusError
	if !errors.As(err, &rse) || rse.status != http.StatusBadRequest {
		t.Fatalf("expected raw 400 error, got %T %v", err, err)
	}
	calls := enforcer.snapshot()
	if len(calls) != 1 || calls[0].detail["reason"] != "cron_overlap_policy" || calls[0].detail["requested_value"] != "skip" {
		t.Fatalf("expected cron_overlap_policy dispatch with requested=skip, got %+v", calls)
	}
}

func TestPlanGate_CreateJobRejectsOnFailureChaining(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		triggerJob      string
		triggerWorkflow string
	}{
		{name: "failure job", triggerJob: "job-fallback"},
		{name: "failure workflow", triggerWorkflow: "workflow-fallback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			enforcer := &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanFree)}
			ms := &APIStoreMock{
				CreateJobFunc: func(context.Context, *domain.Job) error {
					t.Fatal("CreateJob must not be called when on_failure chaining is not available")
					return nil
				},
			}
			srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

			_, err := srv.handleCreateJob(context.Background(), &CreateJobInput{Body: CreateJobRequest{
				ProjectID:                "proj-1",
				Name:                     "failure chain",
				Slug:                     "failure-chain",
				EndpointURL:              "https://example.com/run",
				OnFailureTriggerJob:      tt.triggerJob,
				OnFailureTriggerWorkflow: tt.triggerWorkflow,
			}})
			if !isHumaStatusError(err, http.StatusForbidden) {
				t.Fatalf("expected 403 for unavailable on_failure chaining, got %v", err)
			}
		})
	}
}

func TestPlanGate_CloneJobRejectsOnFailureChaining(t *testing.T) {
	t.Parallel()

	source := &domain.Job{
		ID:                       "job-source",
		ProjectID:                "proj-1",
		Name:                     "Source",
		Slug:                     "source",
		EndpointURL:              "https://example.com/run",
		OnFailureTriggerJob:      "job-fallback",
		OnFailureTriggerWorkflow: "workflow-fallback",
	}
	enforcer := &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanFree)}
	ms := &APIStoreMock{
		GetJobFunc: func(context.Context, string) (*domain.Job, error) {
			return source, nil
		},
		CreateJobFunc: func(context.Context, *domain.Job) error {
			t.Fatal("CreateJob must not be called when cloned on_failure chaining is not available")
			return nil
		},
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleCloneJob(ctx, &CloneJobInput{
		JobID: "job-source",
		Body:  CloneJobRequest{Name: "Clone", Slug: "clone"},
	})
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403 for cloned on_failure chaining, got %v", err)
	}
}
