package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Error(t, srv.checkRunTTLLimit(context.
		Background(), "proj-1", maxTTL+1))

	calls := enforcer.snapshot()
	require.Len(t,
		calls, 1,
	)

	c := calls[0]
	assert.Equal(
		t, domain.
			WebhookEventWorkflowRegistrationRejected,

		c.eventType)
	assert.Equal(
		t, limits.
			PlanTier, c.
			planTier,
	)
	assert.Equal(
		t, "run_ttl_limit",
		c.
			detail["reason"])
	assert.Equal(
		t, maxTTL+
			1, c.detail["requested_value"],
	)
	assert.Equal(
		t, maxTTL,
		c.detail["cap"])

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
	require.Error(t, srv.checkPerJobConcurrencyLimit(context.
		Background(), "proj-1", overCap,
		0))

	calls := enforcer.snapshot()
	require.False(t, len(calls) != 1 ||
		calls[0].detail["reason"] != "per_job_concurrency",
	)
	assert.Equal(
		t, limits.
			MaxConcurrentRuns,
		calls[0].detail["cap"])

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
	require.NoError(t, srv.
		checkRunTTLLimit(context.
			Background(), "proj-1", maxTTL))
	assert.EqualValues(t, 0, len(
		enforcer.snapshot()),
	)

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
	require.Error(t, srv.checkCronOverlapPolicy(context.Background(), "proj-1", "skip"))

	calls := enforcer.snapshot()
	require.False(t, len(calls) != 1 ||
		calls[0].detail["reason"] != "cron_overlap_policy" ||
		calls[0].detail["requested_value"] != "skip")

}

func TestPlanGate_BatchCreateRejectsCronOverlapPolicy(t *testing.T) {
	t.Parallel()

	limits := billing.GetPlanLimits(domain.PlanFree)
	enforcer := &dispatchRecordingEnforcer{
		tunableLimitsEnforcer: tunableLimitsEnforcer{limits: limits},
	}
	ms := &APIStoreMock{
		CreateJobFunc: func(context.Context, *domain.Job) error {
			require.Fail(t,

				"CreateJob must not be called for a batch item rejected by cron overlap policy")
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
	require.Error(t, err)

	var rse *rawStatusError
	require.False(t, !errors.As(err, &rse) || rse.
		status !=

		http.StatusBadRequest)

	calls := enforcer.snapshot()
	require.False(t, len(calls) != 1 ||
		calls[0].detail["reason"] != "cron_overlap_policy" ||
		calls[0].detail["requested_value"] != "skip")

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
					require.Fail(t,

						"CreateJob must not be called when on_failure chaining is not available")
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
			require.True(
				t, isHumaStatusError(err,
					http.
						StatusForbidden,
				))

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
			require.Fail(t,

				"CreateJob must not be called when cloned on_failure chaining is not available")
			return nil
		},
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleCloneJob(ctx, &CloneJobInput{
		JobID: "job-source",
		Body:  CloneJobRequest{Name: "Clone", Slug: "clone"},
	})
	require.True(
		t, isHumaStatusError(err,
			http.
				StatusForbidden,
		))

}
