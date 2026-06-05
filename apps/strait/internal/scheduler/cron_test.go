package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWorkflowTrigger struct {
	triggerWorkflowFn func(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, stepOverrides []domain.StepOverride) (*domain.WorkflowRun, error)
}

func (m *mockWorkflowTrigger) TriggerWorkflow(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, stepOverrides []domain.StepOverride, extraTags map[string]string) (*domain.WorkflowRun, error) {
	if m.triggerWorkflowFn != nil {
		return m.triggerWorkflowFn(ctx, workflowID, projectID, payload, triggeredBy, stepOverrides)
	}
	return nil, nil
}

func TestNewCronScheduler(t *testing.T) {
	t.Parallel()
	cs := NewCronScheduler(context.Background(), &mockCronStore{}, &mockQueue{}, nil)
	require.NotNil(t, cs)
}

func TestCronExpressionWithTimezone(t *testing.T) {
	t.Parallel()

	got := cronExpressionWithTimezone("0 9 * * *", "America/New_York")
	require.Equal(t, "CRON_TZ=America/New_York 0 9 * * *",

		got,
	)
}

func TestCronFireKey_TruncatesToMinute(t *testing.T) {
	t.Parallel()

	a := cronFireKey("job", "job-1", time.Date(2026, 5, 16, 7, 35, 1, 0, time.UTC))
	b := cronFireKey("job", "job-1", time.Date(2026, 5, 16, 7, 35, 59, 0, time.UTC))
	c := cronFireKey("job", "job-1", time.Date(2026, 5, 16, 7, 36, 0, 0, time.UTC))
	require.Equal(t, b,
		a)
	require.NotEqual(t,
		c, a)
}

func TestCronScheduler_LoadJobs_Success(t *testing.T) {
	t.Parallel()
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return []domain.Job{
				{ID: "job-1", ProjectID: "proj-1", Cron: "* * * * *"},
				{ID: "job-2", ProjectID: "proj-2", Cron: "* * * * *"},
			}, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, &mockQueue{}, nil)
	err := cs.LoadJobs(context.Background())
	require.NoError(t,
		err)
}

func TestCronScheduler_LoadJobs_NoJobs(t *testing.T) {
	t.Parallel()
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return []domain.Job{}, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, &mockQueue{}, nil)
	err := cs.LoadJobs(context.Background())
	require.NoError(t,
		err)
}

func TestCronScheduler_LoadJobs_CachesDriftSchedules(t *testing.T) {
	t.Parallel()
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return []domain.Job{{ID: "job-1", ProjectID: "proj-1", Cron: "*/5 * * * *"}}, nil
		},
		listCronWorkflowsFn: func(context.Context) ([]domain.Workflow, error) {
			return []domain.Workflow{{ID: "workflow-1", ProjectID: "proj-1", Cron: "0 * * * *"}}, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, &mockQueue{}, &mockWorkflowTrigger{})
	require.NoError(t,
		cs.LoadJobs(context.
			Background()))

	cs.driftMu.RLock()
	defer cs.driftMu.RUnlock()
	require.NotNil(t, cs.
		driftSchedules["*/5 * * * *"])
	require.NotNil(t, cs.
		driftSchedules["0 * * * *"])
}

func TestCronScheduler_GetDriftSchedule_CachesFallbackParse(t *testing.T) {
	t.Parallel()
	cs := NewCronScheduler(context.Background(), &mockCronStore{}, &mockQueue{}, nil)
	require.NotNil(t,
		cs.getDriftSchedule("*/10 * * * *"))
	require.Nil(t, cs.getDriftSchedule("not a cron"))

	cs.driftMu.RLock()
	defer cs.driftMu.RUnlock()
	require.NotNil(t, cs.
		driftSchedules["*/10 * * * *"])
	require.Nil(t, cs.driftSchedules["not a cron"])
}

func TestDeepSecCronScheduler_LoadJobsReplacesStaleEntries(t *testing.T) {
	t.Parallel()

	var jobs []domain.Job
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return jobs, nil
		},
	}
	cs := NewCronScheduler(context.Background(), store, &mockQueue{}, nil)

	jobs = []domain.Job{
		{ID: "job-1", ProjectID: "proj-1", Cron: "* * * * *"},
		{ID: "job-2", ProjectID: "proj-1", Cron: "*/5 * * * *"},
	}
	require.NoError(t,
		cs.LoadJobs(context.
			Background()))
	require.Len(t, cs.cron.Entries(), 2)

	jobs = []domain.Job{
		{ID: "job-2", ProjectID: "proj-1", Cron: "*/10 * * * *"},
	}
	require.NoError(t,
		cs.LoadJobs(context.
			Background()))
	require.Len(t, cs.cron.Entries(), 1)
}

func TestCronScheduler_LoadJobs_StoreError(t *testing.T) {
	t.Parallel()
	storeErr := errors.New("store error")
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return nil, storeErr
		},
	}

	cs := NewCronScheduler(context.Background(), store, &mockQueue{}, nil)
	err := cs.LoadJobs(context.Background())
	require.Error(t, err)
	require.Contains(t, err.
		Error(), "list cron jobs")
}

func TestCronScheduler_LoadJobs_InvalidCron(t *testing.T) {
	t.Parallel()
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return []domain.Job{{ID: "job-1", ProjectID: "proj-1", Cron: "bad"}}, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, &mockQueue{}, nil)
	err := cs.LoadJobs(context.Background())
	require.Error(t, err)
	require.Contains(t, err.
		Error(), "register cron job job-1")
}

func TestCronScheduler_LoadJobs_SkipsInvalidJobTimezone(t *testing.T) {
	t.Parallel()
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return []domain.Job{
				{ID: "job-invalid-tz", ProjectID: "proj-1", Cron: "* * * * *", Timezone: "Mars/Olympus"},
				{ID: "job-valid-tz", ProjectID: "proj-1", Cron: "* * * * *", Timezone: "UTC"},
			}, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, &mockQueue{}, nil)
	require.NoError(t,
		cs.LoadJobs(context.
			Background()))
	require.Len(t, cs.cron.Entries(), 1)
}

func TestCronScheduler_LoadJobs_SkipsInvalidWorkflowTimezone(t *testing.T) {
	t.Parallel()
	store := &mockCronStore{
		listCronWorkflowsFn: func(context.Context) ([]domain.Workflow, error) {
			return []domain.Workflow{
				{ID: "wf-invalid-tz", ProjectID: "proj-1", Cron: "* * * * *", CronTimezone: "Mars/Olympus"},
				{ID: "wf-valid-tz", ProjectID: "proj-1", Cron: "* * * * *", CronTimezone: "UTC"},
			}, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, &mockQueue{}, &mockWorkflowTrigger{})
	require.NoError(t,
		cs.LoadJobs(context.
			Background()))
	require.Len(t, cs.cron.Entries(), 1)
}

func TestCronScheduler_StartStop(t *testing.T) {
	t.Parallel()
	store := &mockCronStore{
		listCronJobsFn: func(context.Context) ([]domain.Job, error) {
			return []domain.Job{{ID: "job-1", ProjectID: "proj-1", Cron: "* * * * *"}}, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, &mockQueue{}, nil)
	require.NoError(t,
		cs.LoadJobs(context.
			Background()))

	cs.Start()
	stopCtx := cs.Stop()
	<-stopCtx.Done()
}

func TestCronScheduler_TriggerJob(t *testing.T) {
	t.Parallel()
	var enqueued domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = *run
			return nil
		},
	}

	cs := NewCronScheduler(context.Background(), &mockCronStore{}, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", ExecutionMode: domain.ExecutionModeWorker, Queue: "priority"}
	cs.triggerJob(context.Background(), job)
	require.Equal(t, job.
		ID, enqueued.JobID,
	)
	require.Equal(t, job.
		ProjectID, enqueued.
		ProjectID,
	)
	require.Equal(t, "cron",
		enqueued.TriggeredBy,
	)
	require.Equal(t, domain.
		ExecutionModeWorker,
		enqueued.
			ExecutionMode,
	)
	require.Equal(t, "priority",
		enqueued.
			QueueName,
	)
}

func TestCronScheduler_TriggerJob_WithTTL(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}
	cs := NewCronScheduler(context.Background(), &mockCronStore{}, mq, nil)

	job := domain.Job{
		ID:         "job-ttl",
		ProjectID:  "proj-1",
		RunTTLSecs: 600,
	}
	cs.triggerJob(context.Background(), job)
	require.NotNil(t, capturedRun)
	require.NotNil(t, capturedRun.
		ExpiresAt,
	)

	expected := time.Now().Add(600 * time.Second)
	diff := capturedRun.ExpiresAt.Sub(expected)
	assert.False(t, diff <
		-5*time.Second ||
		diff >
			5*time.Second,
	)
}

func TestCronScheduler_TriggerJob_NoTTL(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}
	cs := NewCronScheduler(context.Background(), &mockCronStore{}, mq, nil)

	job := domain.Job{
		ID:         "job-no-ttl",
		ProjectID:  "proj-1",
		RunTTLSecs: 0,
	}
	cs.triggerJob(context.Background(), job)
	require.NotNil(t, capturedRun)
	require.Nil(t, capturedRun.ExpiresAt)
}

func TestCronScheduler_TriggerWorkflow_SkipIfRunning(t *testing.T) {
	t.Parallel()
	triggered := false
	cs := NewCronScheduler(context.Background(), &mockCronStore{
		countRunningWfRunsFn: func(_ context.Context, workflowID string) (int, error) {
			require.Equal(t, "wf-1",
				workflowID)

			return 1, nil
		},
	}, &mockQueue{}, &mockWorkflowTrigger{
		triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
			triggered = true
			return &domain.WorkflowRun{ID: "wr-1"}, nil
		},
	})

	cs.triggerWorkflow(context.Background(), domain.Workflow{ID: "wf-1", ProjectID: "proj-1", SkipIfRunning: true})
	require.False(t, triggered)
}

func TestCronScheduler_TriggerWorkflow_Success(t *testing.T) {
	t.Parallel()
	triggered := false
	cs := NewCronScheduler(context.Background(), &mockCronStore{}, &mockQueue{}, &mockWorkflowTrigger{
		triggerWorkflowFn: func(_ context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
			require.False(t, workflowID !=
				"wf-1" ||
				projectID !=
					"proj-1",
			)
			require.Nil(t, payload)
			require.Equal(t, domain.
				TriggerCron,
				triggeredBy,
			)

			triggered = true
			return &domain.WorkflowRun{ID: "wr-1"}, nil
		},
	})

	cs.triggerWorkflow(context.Background(), domain.Workflow{ID: "wf-1", ProjectID: "proj-1"})
	require.True(t, triggered)
}

func TestCronScheduler_TriggerJob_OverlapPolicyAllow(t *testing.T) {
	t.Parallel()
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	// CountActiveRunsForJob returns active runs, but allow policy should ignore that.
	store := &mockCronStore{
		countActiveRunsForJobFn: func(_ context.Context, _ string) (int, error) {
			return 5, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyAllow}
	cs.triggerJob(context.Background(), job)
	require.True(t, enqueued)
}

func TestCronScheduler_TriggerJob_OverlapPolicySkip_ActiveRuns(t *testing.T) {
	t.Parallel()
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	store := &mockCronStore{
		countActiveRunsForJobFn: func(_ context.Context, jobID string) (int, error) {
			require.Equal(t, "job-1",
				jobID)

			return 2, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicySkip}
	cs.triggerJob(context.Background(), job)
	require.False(t, enqueued)
}

func TestCronScheduler_TriggerJob_OverlapPolicySkip_NoActive(t *testing.T) {
	t.Parallel()
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	store := &mockCronStore{
		countActiveRunsForJobFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
	}

	cs := NewCronScheduler(context.Background(), store, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicySkip}
	cs.triggerJob(context.Background(), job)
	require.True(t, enqueued)
}

func TestCronScheduler_TriggerJob_OverlapPolicySkip_CountError(t *testing.T) {
	t.Parallel()
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	store := &mockCronStore{
		countActiveRunsForJobFn: func(_ context.Context, _ string) (int, error) {
			return 0, errors.New("db error")
		},
	}

	cs := NewCronScheduler(context.Background(), store, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicySkip}
	cs.triggerJob(context.Background(), job)
	require.False(t, enqueued)
}

func TestCronScheduler_TriggerJob_OverlapPolicyCancelRunning(t *testing.T) {
	t.Parallel()
	enqueued := false
	cancelCalled := false
	var cancelReason string
	childCancelCalled := false
	wfCallbackCalled := false

	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, jobID string, reason string) ([]store.CanceledRun, error) {
			require.Equal(t, "job-1",
				jobID)

			cancelCalled = true
			cancelReason = reason
			return []store.CanceledRun{
				{ID: "run-1", ExecutionMode: domain.ExecutionModeWorker},
				{ID: "run-2", ExecutionMode: domain.ExecutionModeHTTP},
			}, nil
		},
		cancelChildRunsByParentIDFn: func(_ context.Context, parentIDs []string, _ time.Time, _ string) (int64, error) {
			childCancelCalled = true
			require.Len(t, parentIDs,
				2)

			return 1, nil
		},
	}
	wfCb := &mockWorkflowCallback{
		onJobRunTerminalFn: func(_ context.Context, run *domain.JobRun) error {
			wfCallbackCalled = true
			return nil
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil).
		WithWorkflowCallback(wfCb)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)
	require.True(t, cancelCalled)
	require.Contains(t, cancelReason, "cancel_running")
	require.True(t, enqueued)
	require.True(t, childCancelCalled)
	require.True(t, wfCallbackCalled)
}

func TestCronScheduler_TriggerJob_OverlapPolicyCancelRunning_CancelError(t *testing.T) {
	t.Parallel()
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return nil, errors.New("cancel failed")
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)
	require.True(t, enqueued)
}

func TestCronScheduler_TriggerJob_OverlapPolicyDefault(t *testing.T) {
	t.Parallel()
	enqueued := false
	var idempotencyKey string
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = true
			idempotencyKey = run.IdempotencyKey
			return nil
		},
	}

	cs := NewCronScheduler(context.Background(), &mockCronStore{}, q, nil)
	// Empty CronOverlapPolicy should behave like allow.
	job := domain.Job{ID: "job-1", ProjectID: "proj-1"}
	cs.triggerJob(context.Background(), job)
	require.True(t, enqueued)
	require.True(t, strings.HasPrefix(idempotencyKey,

		"cron:job:job-1:",
	),
	)
}

func TestCronScheduler_TriggerJob_DuplicateFireIsSkipped(t *testing.T) {
	t.Parallel()

	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return domain.ErrIdempotencyConflict
		},
	}

	cs := NewCronScheduler(context.Background(), &mockCronStore{}, q, nil)
	cs.triggerJob(context.Background(), domain.Job{ID: "job-1", ProjectID: "proj-1"})
}

func TestCronScheduler_TriggerJob_ProjectQueuedQuotaSkipsEnqueue(t *testing.T) {
	t.Parallel()

	enqueued := false
	s := &mockCronStore{
		getProjectQuotaFn: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID, MaxQueuedRuns: 1}, nil
		},
		countProjectQueuedRunsFn: func(_ context.Context, _ string) (int, error) {
			return 1, nil
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	cs.triggerJob(context.Background(), domain.Job{ID: "job-quota", ProjectID: "proj-quota"})
	require.False(t, enqueued)
}

func TestCronScheduler_TriggerJob_JobRateLimitSkipsEnqueue(t *testing.T) {
	t.Parallel()

	enqueued := false
	s := &mockCronStore{
		countRunsForJobSinceFn: func(_ context.Context, jobID string, since time.Time) (int, error) {
			require.Equal(t, "job-rate",
				jobID)
			require.LessOrEqual(t, time.Since(since), 2*
				time.Minute,
			)

			return 1, nil
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	cs.triggerJob(context.Background(), domain.Job{
		ID:                  "job-rate",
		ProjectID:           "proj-rate",
		RateLimitMax:        1,
		RateLimitWindowSecs: 60,
	})
	require.False(t, enqueued)
}

func TestCronScheduler_ProcessCanceledRuns_PassesWorkflowStepRunID(t *testing.T) {
	t.Parallel()

	var callbackRun *domain.JobRun
	cs := NewCronScheduler(context.Background(), &mockCronStore{}, &mockQueue{}, nil).
		WithWorkflowCallback(&mockWorkflowCallback{
			onJobRunTerminalFn: func(_ context.Context, run *domain.JobRun) error {
				callbackRun = run
				return nil
			},
		})

	cs.processCanceledRuns(context.Background(), "job-1", []store.CanceledRun{{
		ID:                "run-1",
		JobID:             "job-1",
		ProjectID:         "proj-1",
		WorkflowStepRunID: "step-run-1",
		ExecutionMode:     domain.ExecutionModeWorker,
	}})
	require.NotNil(t, callbackRun)
	require.Equal(t, "step-run-1",
		callbackRun.
			WorkflowStepRunID,
	)
	require.False(t, callbackRun.
		JobID !=
		"job-1" ||
		callbackRun.
			ProjectID !=
			"proj-1",
	)
	require.Equal(t, domain.
		ExecutionModeWorker,
		callbackRun.
			ExecutionMode,
	)
}

// Adversarial and edge-case tests for cron overlap policy.

func TestCronScheduler_TriggerJob_UnknownPolicyBehavesLikeAllow(t *testing.T) {
	t.Parallel()
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	// A garbage policy value that doesn't match any constant should fall
	// through the default case and behave like allow.
	cs := NewCronScheduler(context.Background(), &mockCronStore{}, q, nil)
	job := domain.Job{
		ID:                "job-1",
		ProjectID:         "proj-1",
		CronOverlapPolicy: domain.CronOverlapPolicy("bogus_value"),
	}
	cs.triggerJob(context.Background(), job)
	require.True(t, enqueued)
}

func TestCronScheduler_TriggerJob_CancelRunning_EmptyResultStillEnqueues(t *testing.T) {
	t.Parallel()
	// CancelActiveRunsForJob returns an empty slice (no active runs to cancel).
	// The new run should still be enqueued.
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return []store.CanceledRun{}, nil // empty, not nil
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)
	require.True(t, enqueued)
}

func TestCronScheduler_TriggerJob_CancelRunning_NilResultStillEnqueues(t *testing.T) {
	t.Parallel()
	// CancelActiveRunsForJob returns nil slice (no active runs).
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return nil, nil // nil slice
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)
	require.True(t, enqueued)
}

func TestCronScheduler_TriggerJob_CancelRunning_ChildCancelErrorDoesNotPreventEnqueue(t *testing.T) {
	t.Parallel()
	// Child cancel fails, but the run should still be enqueued.
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return []store.CanceledRun{
				{ID: "run-1", ExecutionMode: domain.ExecutionModeHTTP},
			}, nil
		},
		cancelChildRunsByParentIDFn: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, errors.New("child cancel db error")
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)
	require.True(t, enqueued)
}

func TestCronScheduler_TriggerJob_DailyCostQuotaPreventsEnqueue(t *testing.T) {
	t.Parallel()
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		getProjectQuotaFn: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			require.Equal(t, "proj-1",
				projectID,
			)

			return &store.ProjectQuota{
				MaxDailyCostMicrousd: 500,
				Timezone:             "America/New_York",
			}, nil
		},
		sumProjectDailyCostFn: func(_ context.Context, projectID string, timezone string) (int64, error) {
			require.Equal(t, "proj-1",
				projectID,
			)
			require.Equal(t, "America/New_York",

				timezone)

			return 500, nil
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	cs.triggerJob(context.Background(), domain.Job{
		ID:        "job-1",
		ProjectID: "proj-1",
		Enabled:   true,
	})
	require.False(t, enqueued)
}

func TestCronScheduler_TriggerJob_CancelRunning_WorkflowCallbackErrorDoesNotPreventEnqueue(t *testing.T) {
	t.Parallel()
	// Workflow callback fails, but the run should still be enqueued.
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return []store.CanceledRun{
				{ID: "run-1", ExecutionMode: domain.ExecutionModeHTTP},
			}, nil
		},
	}
	wfCb := &mockWorkflowCallback{
		onJobRunTerminalFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("workflow engine down")
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil).WithWorkflowCallback(wfCb)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)
	require.True(t, enqueued)
}

func TestCronScheduler_TriggerJob_CancelRunning_NilDependenciesAreGraceful(t *testing.T) {
	t.Parallel()
	// No workflowCallback set. Should not panic.
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return []store.CanceledRun{
				{ID: "run-1", ExecutionMode: domain.ExecutionModeWorker},
			}, nil
		},
	}

	// Intentionally no WithWorkflowCallback.
	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)
	require.True(t, enqueued)
}

func TestCronScheduler_TriggerJob_Skip_EnqueueError(t *testing.T) {
	t.Parallel()
	// skip policy, no active runs, but enqueue fails. Should not panic.
	s := &mockCronStore{
		countActiveRunsForJobFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("queue full")
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicySkip}
	// Should not panic.
	cs.triggerJob(context.Background(), job)
}

func TestCronScheduler_TriggerJob_CancelRunning_EnqueueError(t *testing.T) {
	t.Parallel()
	cancelCalled := false
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			cancelCalled = true
			return []store.CanceledRun{
				{ID: "run-1", ExecutionMode: domain.ExecutionModeHTTP},
			}, nil
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("queue full")
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	// Should not panic. Runs are already canceled but new run fails to enqueue.
	cs.triggerJob(context.Background(), job)
	require.False(t, cancelCalled)
}

func TestDeepSecCronScheduler_CancelRunningCancelsOnlyAfterReplacementEnqueue(t *testing.T) {
	t.Parallel()

	var events []string
	var replacementRunID string
	var excludedRunID string
	s := &mockCronStore{
		cancelActiveRunsExceptFn: func(_ context.Context, _ string, excludeRunID string, _ string) ([]store.CanceledRun, error) {
			events = append(events, "cancel")
			excludedRunID = excludeRunID
			return []store.CanceledRun{{ID: "run-1", ExecutionMode: domain.ExecutionModeHTTP}}, nil
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			events = append(events, "enqueue")
			replacementRunID = run.ID
			return nil
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)
	require.Equal(t, "enqueue,cancel",
		strings.Join(events, ","))
	require.NotEmpty(t,
		replacementRunID,
	)
	require.Equal(t, replacementRunID,
		excludedRunID,
	)
}

func TestCronOverlapPolicy_IsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		policy domain.CronOverlapPolicy
		valid  bool
	}{
		{domain.OverlapPolicyAllow, true},
		{domain.OverlapPolicySkip, true},
		{domain.OverlapPolicyCancelRunning, true},
		{"", false},
		{"queue", false},
		{"ALLOW", false},
		{"cancel_Running", false},
		{"skip ", false},
		{" allow", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.
			valid, tt.policy.
			IsValid())
	}
}

func TestCronScheduler_TriggerJob_CancelRunning_CorrectRunFields(t *testing.T) {
	t.Parallel()
	// Verify the enqueued run has the correct fields set.
	var capturedRun *domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return nil, nil
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{
		ID:                "job-1",
		ProjectID:         "proj-1",
		CronOverlapPolicy: domain.OverlapPolicyCancelRunning,
		Tags:              map[string]string{"env": "prod"},
		Version:           3,
		VersionID:         "ver_abc",
	}
	cs.triggerJob(context.Background(), job)
	require.NotNil(t, capturedRun)
	assert.Equal(t, "job-1",
		capturedRun.
			JobID)
	assert.Equal(t, "proj-1",
		capturedRun.
			ProjectID,
	)
	assert.Equal(t, domain.
		TriggerCron,
		capturedRun.
			TriggeredBy,
	)
	assert.Equal(t, "system:cron",
		capturedRun.
			CreatedBy,
	)
	assert.Equal(t, 3,
		capturedRun.JobVersion,
	)
	assert.Equal(t, "ver_abc",
		capturedRun.
			JobVersionID,
	)
	assert.Equal(t, "prod",
		capturedRun.
			Tags["env"])
}

func TestCronScheduler_TriggerJob_CancelRunning_ChildCancelReceivesCorrectParentIDs(t *testing.T) {
	t.Parallel()
	var capturedParentIDs []string
	enqueued := false
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued = true
			return nil
		},
	}
	s := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return []store.CanceledRun{
				{ID: "run-aaa"},
				{ID: "run-bbb"},
				{ID: "run-ccc"},
			}, nil
		},
		cancelChildRunsByParentIDFn: func(_ context.Context, parentIDs []string, _ time.Time, reason string) (int64, error) {
			capturedParentIDs = parentIDs
			assert.Contains(t, reason, "cron overlap policy")

			return 2, nil
		},
	}

	cs := NewCronScheduler(context.Background(), s, q, nil)
	job := domain.Job{ID: "job-1", ProjectID: "proj-1", CronOverlapPolicy: domain.OverlapPolicyCancelRunning}
	cs.triggerJob(context.Background(), job)
	require.True(t, enqueued)
	require.Len(t, capturedParentIDs,
		3)

	expected := []string{"run-aaa", "run-bbb", "run-ccc"}
	for i, id := range capturedParentIDs {
		assert.Equal(t, expected[i], id)
	}
}

func FuzzCronExpression(f *testing.F) {
	f.Add("*/5 * * * *")
	f.Add("0 0 * * *")
	f.Add("0 12 * * MON-FRI")
	f.Add("")
	f.Add("not a cron")
	f.Add("* * * * * *")

	f.Fuzz(func(t *testing.T, expr string) {
		cs := NewCronScheduler(context.Background(), nil, nil, nil)
		_, _ = cs.cron.AddFunc(expr, func() {})
	})
}

func BenchmarkCronParse(b *testing.B) {
	cs := NewCronScheduler(context.Background(), nil, nil, nil)
	for b.Loop() {
		_, _ = cs.cron.AddFunc("*/5 * * * *", func() {})
	}
}
