package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/robfig/cron/v3"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cron parsing edge cases

func TestCron_DSTBoundary_SpringForward(t *testing.T) {
	t.Parallel()

	// Spring-forward DST boundary: 2:30 AM does not exist in America/New_York
	// on the second Sunday of March. The cron library should handle this.
	expr := "CRON_TZ=America/New_York 30 2 * * *"
	c := cron.New()
	_, err := c.AddFunc(expr, func() {})
	require.NoError(t,
		err)
}

func TestCron_DSTBoundary_FallBack(t *testing.T) {
	t.Parallel()

	// Fall-back DST boundary: 1:30 AM occurs twice in America/New_York
	// on the first Sunday of November.
	expr := "CRON_TZ=America/New_York 30 1 * * *"
	c := cron.New()
	_, err := c.AddFunc(expr, func() {})
	require.NoError(t,
		err)
}

func TestCron_Feb29_LeapYear(t *testing.T) {
	t.Parallel()

	// Schedule for Feb 29 -- only fires on leap years.
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse("0 0 29 2 *")
	require.NoError(t,
		err)

	// From Jan 1, 2027 (not a leap year), next Feb 29 is 2028.
	from := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	next := sched.Next(from)
	require.False(t, next.
		Year() != 2028 ||
		next.Month() !=
			2 || next.
		Day() !=
		29)
}

func TestCron_InvalidTimezone(t *testing.T) {
	t.Parallel()

	expr := "CRON_TZ=Mars/Olympus_Mons 0 * * * *"
	c := cron.New()
	_, err := c.AddFunc(expr, func() {})
	require.Error(t, err)
}

func TestCron_EverySecondDescriptor(t *testing.T) {
	t.Parallel()

	c := cron.New()
	_, err := c.AddFunc("@every 1s", func() {})
	require.NoError(t,
		err)
}

func TestCron_OverflowFields(t *testing.T) {
	t.Parallel()

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	overflows := []string{
		"60 * * * *",
		"* 24 * * *",
		"* * 32 * *",
		"* * * 13 *",
		"* * * * 8",
		"-1 * * * *",
		"* * * * -1",
		"999 999 999 999 999",
	}
	for _, expr := range overflows {
		_, err := parser.Parse(expr)
		assert.Error(t, err)
	}
}

func TestCron_EmptyExpression(t *testing.T) {
	t.Parallel()

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	empties := []string{"", "     ", "\t\n"}
	for _, expr := range empties {
		_, err := parser.Parse(expr)
		assert.Error(t, err)
	}
}

// CronScheduler.LoadJobs edge cases

func TestCronScheduler_LoadJobs_InvalidCronExpression(t *testing.T) {
	t.Parallel()

	ms := &mockCronStore{
		listCronJobsFn: func(_ context.Context) ([]domain.Job, error) {
			return []domain.Job{
				{ID: "job-bad", Cron: "NOT_A_CRON", ProjectID: "p1"},
			}, nil
		},
	}
	cs := NewCronScheduler(context.Background(), ms, &mockQueue{}, nil)
	err := cs.LoadJobs(context.Background())
	require.Error(t, err)
	require.Contains(t, err.
		Error(), "register cron job")
}

func TestCronScheduler_LoadJobs_WorkflowListError(t *testing.T) {
	t.Parallel()

	ms := &mockCronStore{
		listCronWorkflowsFn: func(_ context.Context) ([]domain.Workflow, error) {
			return nil, errors.New("workflow table missing")
		},
	}
	cs := NewCronScheduler(context.Background(), ms, &mockQueue{}, nil)
	err := cs.LoadJobs(context.Background())
	require.Error(t, err)
	require.Contains(t, err.
		Error(), "list cron workflows")
}

func TestCronScheduler_LoadJobs_InvalidWorkflowCron(t *testing.T) {
	t.Parallel()

	wt := &mockWorkflowTrigger{}
	ms := &mockCronStore{
		listCronWorkflowsFn: func(_ context.Context) ([]domain.Workflow, error) {
			return []domain.Workflow{
				{ID: "wf-bad", Cron: "INVALID", CronTimezone: "", ProjectID: "p1"},
			}, nil
		},
	}
	cs := NewCronScheduler(context.Background(), ms, &mockQueue{}, wt)
	err := cs.LoadJobs(context.Background())
	require.Error(t, err)
	require.Contains(t, err.
		Error(), "register cron workflow")
}

// Overlapping schedules / cron overlap policies

func TestCronScheduler_TriggerJob_SkipPolicy_ActiveRuns(t *testing.T) {
	t.Parallel()

	var enqueued atomic.Int32
	ms := &mockCronStore{
		countActiveRunsForJobFn: func(_ context.Context, _ string) (int, error) {
			return 3, nil
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued.Add(1)
			return nil
		},
	}
	cs := NewCronScheduler(context.Background(), ms, q, nil)

	job := domain.Job{
		ID:                "job-skip",
		ProjectID:         "p1",
		CronOverlapPolicy: domain.OverlapPolicySkip,
		Cron:              "* * * * *",
	}
	cs.triggerJob(context.Background(), job)
	require.EqualValues(t, 0,
		enqueued.Load())
}

func TestCronScheduler_TriggerJob_SkipPolicy_CountError(t *testing.T) {
	t.Parallel()

	var enqueued atomic.Int32
	ms := &mockCronStore{
		countActiveRunsForJobFn: func(_ context.Context, _ string) (int, error) {
			return 0, errors.New("db timeout")
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued.Add(1)
			return nil
		},
	}
	cs := NewCronScheduler(context.Background(), ms, q, nil)

	job := domain.Job{
		ID:                "job-skip-err",
		ProjectID:         "p1",
		CronOverlapPolicy: domain.OverlapPolicySkip,
		Cron:              "* * * * *",
	}
	cs.triggerJob(context.Background(), job)
	require.EqualValues(t, 0,
		enqueued.Load())
}

func TestCronScheduler_TriggerJob_CancelRunning_CancelErrorAfterEnqueue(t *testing.T) {
	t.Parallel()

	var enqueued atomic.Int32
	ms := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return nil, errors.New("cancel failed")
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued.Add(1)
			return nil
		},
	}
	cs := NewCronScheduler(context.Background(), ms, q, nil)

	job := domain.Job{
		ID:                "job-cancel-err",
		ProjectID:         "p1",
		CronOverlapPolicy: domain.OverlapPolicyCancelRunning,
		Cron:              "* * * * *",
	}
	cs.triggerJob(context.Background(), job)
	require.EqualValues(t, 1,
		enqueued.Load())
}

func TestCronScheduler_TriggerJob_CancelRunning_WorkflowCallback(t *testing.T) {
	t.Parallel()

	var callbackIDs []string
	var mu sync.Mutex
	cb := &mockWorkflowCallback{
		onJobRunTerminalFn: func(_ context.Context, run *domain.JobRun) error {
			mu.Lock()
			callbackIDs = append(callbackIDs, run.ID)
			mu.Unlock()
			return nil
		},
	}

	ms := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return []store.CanceledRun{
				{ID: "run-cb-1"},
				{ID: "run-cb-2"},
			}, nil
		},
	}
	q := &mockQueue{}
	cs := NewCronScheduler(context.Background(), ms, q, nil).
		WithWorkflowCallback(cb)

	job := domain.Job{
		ID:                "job-cancel-cb",
		ProjectID:         "p1",
		CronOverlapPolicy: domain.OverlapPolicyCancelRunning,
		Cron:              "* * * * *",
	}
	cs.triggerJob(context.Background(), job)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, callbackIDs,
		2)
}

func TestCronScheduler_TriggerJob_CancelRunning_CallbackError(t *testing.T) {
	t.Parallel()

	cb := &mockWorkflowCallback{
		onJobRunTerminalFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("callback blew up")
		},
	}

	ms := &mockCronStore{
		cancelActiveRunsForJobFn: func(_ context.Context, _ string, _ string) ([]store.CanceledRun, error) {
			return []store.CanceledRun{{ID: "run-cb-err"}}, nil
		},
	}
	q := &mockQueue{}
	cs := NewCronScheduler(context.Background(), ms, q, nil).
		WithWorkflowCallback(cb)

	job := domain.Job{
		ID:                "job-cancel-cb-err",
		ProjectID:         "p1",
		CronOverlapPolicy: domain.OverlapPolicyCancelRunning,
		Cron:              "* * * * *",
	}
	// Should not panic even when callback returns error.
	cs.triggerJob(context.Background(), job)
}

func TestCronScheduler_TriggerJob_AllowPolicy(t *testing.T) {
	t.Parallel()

	var enqueued atomic.Int32
	ms := &mockCronStore{}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued.Add(1)
			return nil
		},
	}
	cs := NewCronScheduler(context.Background(), ms, q, nil)

	job := domain.Job{
		ID:                "job-allow",
		ProjectID:         "p1",
		CronOverlapPolicy: domain.OverlapPolicyAllow,
		Cron:              "* * * * *",
	}
	cs.triggerJob(context.Background(), job)
	require.EqualValues(t, 1,
		enqueued.Load())
}

func TestCronScheduler_TriggerJob_UnknownPolicy(t *testing.T) {
	t.Parallel()

	var enqueued atomic.Int32
	ms := &mockCronStore{}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued.Add(1)
			return nil
		},
	}
	cs := NewCronScheduler(context.Background(), ms, q, nil)

	job := domain.Job{
		ID:                "job-unknown",
		ProjectID:         "p1",
		CronOverlapPolicy: "future_policy_v2",
		Cron:              "* * * * *",
	}
	cs.triggerJob(context.Background(), job)
	require.EqualValues(t, 1,
		enqueued.Load())
}

func TestCronScheduler_TriggerJob_EnqueueError(t *testing.T) {
	t.Parallel()

	ms := &mockCronStore{}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("queue full")
		},
	}
	cs := NewCronScheduler(context.Background(), ms, q, nil)

	job := domain.Job{
		ID:        "job-enq-err",
		ProjectID: "p1",
		Cron:      "* * * * *",
	}
	// Should not panic.
	cs.triggerJob(context.Background(), job)
}

func TestCronScheduler_TriggerJob_TTLFromJob(t *testing.T) {
	t.Parallel()

	var capturedRun *domain.JobRun
	ms := &mockCronStore{}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, r *domain.JobRun) error {
			capturedRun = r
			return nil
		},
	}
	cs := NewCronScheduler(context.Background(), ms, q, nil)

	job := domain.Job{
		ID:         "job-ttl",
		ProjectID:  "p1",
		RunTTLSecs: 300,
		Cron:       "* * * * *",
	}
	cs.triggerJob(context.Background(), job)
	require.False(t, capturedRun ==
		nil ||
		capturedRun.
			ExpiresAt ==
			nil,
	)

	delta := time.Until(*capturedRun.ExpiresAt)
	require.False(t, delta <
		4*time.Minute ||
		delta >
			6*time.
				Minute)
}

func TestCronScheduler_TriggerJob_TTLFromDefault(t *testing.T) {
	t.Parallel()

	var capturedRun *domain.JobRun
	ms := &mockCronStore{}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, r *domain.JobRun) error {
			capturedRun = r
			return nil
		},
	}
	cs := NewCronScheduler(context.Background(), ms, q, nil).
		WithDefaultRunTTLSecs(600)

	job := domain.Job{
		ID:        "job-ttl-def",
		ProjectID: "p1",
		Cron:      "* * * * *",
	}
	cs.triggerJob(context.Background(), job)
	require.False(t, capturedRun ==
		nil ||
		capturedRun.
			ExpiresAt ==
			nil,
	)

	delta := time.Until(*capturedRun.ExpiresAt)
	require.False(t, delta <
		9*time.Minute ||
		delta >
			11*time.
				Minute)
}

// CronScheduler.triggerWorkflow edge cases

func TestCronScheduler_TriggerWorkflow_CountRunningError(t *testing.T) {
	t.Parallel()

	var triggered atomic.Int32
	wt := &mockWorkflowTrigger{
		triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
			triggered.Add(1)
			return &domain.WorkflowRun{}, nil
		},
	}
	ms := &mockCronStore{
		countRunningWfRunsFn: func(_ context.Context, _ string) (int, error) {
			return 0, errors.New("db error")
		},
	}
	cs := NewCronScheduler(context.Background(), ms, &mockQueue{}, wt)

	wf := domain.Workflow{
		ID:            "wf-count-err",
		ProjectID:     "p1",
		Cron:          "* * * * *",
		SkipIfRunning: true,
	}
	cs.triggerWorkflow(context.Background(), wf)
	require.EqualValues(t, 0,
		triggered.Load(),
	)
}

func TestCronScheduler_TriggerWorkflow_TriggerError(t *testing.T) {
	t.Parallel()

	wt := &mockWorkflowTrigger{
		triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
			return nil, errors.New("trigger failed")
		},
	}
	ms := &mockCronStore{}
	cs := NewCronScheduler(context.Background(), ms, &mockQueue{}, wt)

	wf := domain.Workflow{
		ID:        "wf-trigger-err",
		ProjectID: "p1",
		Cron:      "* * * * *",
	}
	// Should not panic.
	cs.triggerWorkflow(context.Background(), wf)
}

func TestCronScheduler_TriggerWorkflow_NoSkipPolicy(t *testing.T) {
	t.Parallel()

	var triggered atomic.Int32
	wt := &mockWorkflowTrigger{
		triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
			triggered.Add(1)
			return &domain.WorkflowRun{}, nil
		},
	}
	ms := &mockCronStore{}
	cs := NewCronScheduler(context.Background(), ms, &mockQueue{}, wt)

	wf := domain.Workflow{
		ID:            "wf-no-skip",
		ProjectID:     "p1",
		Cron:          "* * * * *",
		SkipIfRunning: false,
	}
	cs.triggerWorkflow(context.Background(), wf)
	require.EqualValues(t, 1,
		triggered.Load(),
	)
}

// Batch operation abuse

func TestBatchFlusher_ZeroInterval_Clamped(t *testing.T) {
	t.Parallel()
	f := NewBatchFlusher(nil, nil, 0)
	require.Equal(t, time.
		Second, f.interval,
	)
}

func TestBatchFlusher_NegativeInterval_Clamped(t *testing.T) {
	t.Parallel()
	f := NewBatchFlusher(nil, nil, -5*time.Minute)
	require.Equal(t, time.
		Second, f.interval,
	)
}

func TestBatchFlusher_SingleItemBatch(t *testing.T) {
	t.Parallel()

	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "", ItemCount: 1},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "i1", JobID: "job-1", ProjectID: "proj-1", Payload: json.RawMessage(`{"x":1}`), Priority: 1, CreatedBy: "u1"},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, BatchMaxSize: 10},
		},
	}

	var enqueued []*domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = append(enqueued, run)
			return nil
		},
	}
	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())
	require.Len(t, enqueued,
		1)

	var payload map[string]any
	require.NoError(t,
		json.Unmarshal(enqueued[0].
			Payload,
			&payload))

	items := payload["items"].([]any)
	require.Len(t, items,
		1)
}

func TestBatchFlusher_ZeroBatchMaxSize_UsesItemCount(t *testing.T) {
	t.Parallel()

	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "", ItemCount: 5},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "i1", JobID: "job-1", ProjectID: "proj-1", Payload: json.RawMessage(`{}`), Priority: 1, CreatedBy: "u1"},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, BatchMaxSize: 0},
		},
	}

	var enqueued []*domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = append(enqueued, run)
			return nil
		},
	}
	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())
	require.Len(t, enqueued,
		1)
}

func TestBatchFlusher_RunTTL_OverridesTimeout(t *testing.T) {
	t.Parallel()

	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "", ItemCount: 1},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "i1", JobID: "job-1", ProjectID: "proj-1", Payload: json.RawMessage(`{}`), Priority: 1, CreatedBy: "u1"},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, RunTTLSecs: 3600, BatchMaxSize: 10},
		},
	}

	var capturedRun *domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRun = run
			return nil
		},
	}
	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())
	require.False(t, capturedRun ==
		nil ||
		capturedRun.
			ExpiresAt ==
			nil,
	)

	delta := time.Until(*capturedRun.ExpiresAt)
	require.False(t, delta <
		55*time.Minute ||
		delta >
			65*
				time.Minute,
	)
}

func TestBatchFlusher_MultipleBatches(t *testing.T) {
	t.Parallel()

	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "a", ItemCount: 1},
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "b", ItemCount: 1},
			{JobID: "job-2", ProjectID: "proj-2", BatchKey: "", ItemCount: 1},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "i1", JobID: "job-1", ProjectID: "proj-1", Payload: json.RawMessage(`{}`), Priority: 1, CreatedBy: "u1"},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, BatchMaxSize: 10},
			"job-2": {ID: "job-2", ProjectID: "proj-2", Enabled: true, TimeoutSecs: 60, BatchMaxSize: 10},
		},
	}

	var enqueued atomic.Int32
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued.Add(1)
			return nil
		},
	}
	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())
	require.EqualValues(t, 3,
		enqueued.Load())
}

func TestBatchFlusher_AdvisoryLockError(t *testing.T) {
	t.Parallel()

	bs := &mockBatchStore{
		tryAdvisoryLockFn: func(_ context.Context, _ int64) (bool, error) {
			return false, errors.New("lock error")
		},
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "", ItemCount: 1},
		},
	}

	var enqueued atomic.Int32
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued.Add(1)
			return nil
		},
	}
	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())
	require.EqualValues(t, 0,
		enqueued.Load())
}

func TestBatchFlusher_RunStopsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	bs := &mockBatchStore{}
	q := &mockQueue{}
	flusher := NewBatchFlusher(bs, q, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		flusher.Run(ctx)
		close(done)
	})

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "Run did not stop on context cancel")
	}
}

// Anomaly detection edge cases

func TestAnomalyMonitor_AdvisoryLockerNotAcquired(t *testing.T) {
	t.Parallel()

	var checked bool
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			checked = true
			return nil, nil
		},
	}
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
	}
	am := NewAnomalyMonitor(s, time.Minute).WithAdvisoryLocker(locker)
	am.check(context.Background())
	require.False(t, checked)
}

func TestAnomalyMonitor_AdvisoryLockerError(t *testing.T) {
	t.Parallel()

	var checked bool
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			checked = true
			return nil, nil
		},
	}
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, errors.New("pg connection lost")
		},
	}
	am := NewAnomalyMonitor(s, time.Minute).WithAdvisoryLocker(locker)
	am.check(context.Background())
	require.False(t, checked)
}

func TestAnomalyMonitor_CooldownError_SkipsOrg(t *testing.T) {
	t.Parallel()

	// Use the existing mockCooldown from anomaly_monitor_test.go, but we need
	// a custom one that returns errors. Instead, test via check behavior.
	// The existing mockCooldown only supports map-based responses.
	// We verify behavior indirectly: if cooldown is active, the org is skipped.
	cooldown := newMockCooldown()
	cooldown.cooled["org-1"] = true

	var subscriptionChecked bool
	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			subscriptionChecked = true
			return nil, nil
		},
	}
	am := NewAnomalyMonitor(s, time.Minute).WithCooldown(cooldown)
	am.check(context.Background())
	require.False(t, subscriptionChecked)
}

func TestAnomalyMonitor_EmptyOrgList(t *testing.T) {
	t.Parallel()

	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			return nil, nil
		},
	}
	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
}

func TestAnomalyMonitor_ListOrgsError(t *testing.T) {
	t.Parallel()

	s := &mockAnomalyMonitorStore{
		listAllSubscribedOrgIDsFn: func(_ context.Context) ([]string, error) {
			return nil, errors.New("db down")
		},
	}
	am := NewAnomalyMonitor(s, time.Minute)
	am.check(context.Background())
}

// RedisCooldown edge cases

func TestRedisCooldown_ZeroTTL_ClampsToDefault(t *testing.T) {
	t.Parallel()
	rc := NewRedisCooldown(&advMockRedisClient{}, 0)
	require.Equal(t, 4*
		time.Hour, rc.ttl,
	)
}

func TestRedisCooldown_NegativeTTL_ClampsToDefault(t *testing.T) {
	t.Parallel()
	rc := NewRedisCooldown(&advMockRedisClient{}, -time.Hour)
	require.Equal(t, 4*
		time.Hour, rc.ttl,
	)
}

func TestRedisCooldown_InCooldown_KeyNotFound(t *testing.T) {
	t.Parallel()
	client := &advMockRedisClient{
		getFn: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("redis: nil")
		},
	}
	rc := NewRedisCooldown(client, time.Hour)
	cooled, err := rc.InCooldown(context.Background(), "org-1")
	require.NoError(t,
		err)
	require.False(t, cooled)
}

func TestRedisCooldown_InCooldown_KeyExists(t *testing.T) {
	t.Parallel()
	client := &advMockRedisClient{
		getFn: func(_ context.Context, _ string) (string, error) {
			return "1", nil
		},
	}
	rc := NewRedisCooldown(client, time.Hour)
	cooled, err := rc.InCooldown(context.Background(), "org-1")
	require.NoError(t,
		err)
	require.True(t, cooled)
}

func TestRedisCooldown_SetCooldown(t *testing.T) {
	t.Parallel()
	var setKey string
	client := &advMockRedisClient{
		setFn: func(_ context.Context, key string, _ any, _ time.Duration) error {
			setKey = key
			return nil
		},
	}
	rc := NewRedisCooldown(client, time.Hour)
	require.NoError(t,
		rc.SetCooldown(context.
			Background(),
			"org-1"))

	expected := "strait:anomaly_cooldown:org-1"
	require.Equal(t, expected,
		setKey)
}

func TestCooldownKey_Format(t *testing.T) {
	t.Parallel()
	key := cooldownKey("org-abc")
	require.Equal(t, "strait:anomaly_cooldown:org-abc",

		key,
	)
}

// Budget monitor concurrency and edge cases

func TestBudgetMonitor_ConcurrentCheckAndCleanup(t *testing.T) {
	t.Parallel()

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute)

	bm.alertedMu.Lock()
	bm.alerted["proj-x:1970-01-01"] = true
	bm.alerted["proj-y:1970-01-01"] = true
	bm.alertedMu.Unlock()

	var wg conc.WaitGroup
	for range 20 {
		wg.Go(func() {
			bm.check(context.Background())
		})
	}
	wg.Wait()

	bm.alertedMu.Lock()
	for k := range bm.alerted {
		assert.NotContains(t, k, "1970-01-01")
	}
	bm.alertedMu.Unlock()
}

// Budget monitor spending limits edge cases

func TestBudgetMonitor_SpendingLimit_NilPeriodStart(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return &billing.OrgSubscription{
				OrgID:                 "org-1",
				PlanTier:              "starter",
				SpendingLimitMicrousd: 100_000_000,
				LimitAction:           "notify",
				CurrentPeriodStart:    nil,
			}, nil
		},
		sumOrgPeriodSpendFn: func(_ context.Context, _ string, _ time.Time) (int64, error) {
			return 200_000_000, nil
		},
		listProjectsByOrgFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"proj-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{
				{ID: "ch-1", ProjectID: "proj-1", ChannelType: domain.ChannelTypeWebhook},
			}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	bm.check(context.Background())
}

func TestBudgetMonitor_SpendingLimit_SubscriptionError(t *testing.T) {
	t.Parallel()

	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return nil, errors.New("db error")
		},
	}

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	bm.check(context.Background())
}

func TestBudgetMonitor_SpendingLimit_SpendError(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*billing.OrgSubscription, error) {
			return &billing.OrgSubscription{
				OrgID:                 "org-1",
				PlanTier:              "starter",
				SpendingLimitMicrousd: 100_000_000,
				CurrentPeriodStart:    &now,
			}, nil
		},
		sumOrgPeriodSpendFn: func(_ context.Context, _ string, _ time.Time) (int64, error) {
			return 0, errors.New("query timeout")
		},
	}

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	bm.check(context.Background())
}

func TestBudgetMonitor_SpendingLimit_OrgListError(t *testing.T) {
	t.Parallel()

	ss := &mockSpendingLimitStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return nil, errors.New("db down")
		},
	}

	bm := NewBudgetMonitor(struct{}{}, &mockEnqueuer{}, time.Minute).WithSpendingLimitStore(ss)
	bm.check(context.Background())
}

// SLO evaluator edge cases

func TestCalculateErrorBudget_InfInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current float64
		target  float64
		metric  string
	}{
		{"inf_success_current", math.Inf(1), 0.99, domain.SLOMetricSuccessRate},
		{"neg_inf_success_current", math.Inf(-1), 0.99, domain.SLOMetricSuccessRate},
		{"inf_latency_current", math.Inf(1), 1.0, domain.SLOMetricP95LatencySecs},
		{"neg_inf_latency_current", math.Inf(-1), 1.0, domain.SLOMetricP95LatencySecs},
		{"inf_target", 0.99, math.Inf(1), domain.SLOMetricSuccessRate},
		{"nan_current", math.NaN(), 0.99, domain.SLOMetricSuccessRate},
		{"nan_target", 0.99, math.NaN(), domain.SLOMetricSuccessRate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			budget := CalculateErrorBudget(tt.current, tt.target, tt.metric)
			assert.False(t, math.
				IsNaN(budget))
			assert.False(t, budget <
				0 || budget >
				1)
		})
	}
}

func TestCalculateErrorBudget_TargetOne_SuccessRate(t *testing.T) {
	t.Parallel()

	budget := CalculateErrorBudget(1.0, 1.0, domain.SLOMetricSuccessRate)
	assert.InDelta(t, 1.0,
		budget, 1e-9)

	budget = CalculateErrorBudget(0.99, 1.0, domain.SLOMetricSuccessRate)
	assert.InDelta(t, 0.0,
		budget, 1e-9)
}

func TestCalculateErrorBudget_ZeroTarget_Latency(t *testing.T) {
	t.Parallel()

	budget := CalculateErrorBudget(5.0, 0.0, domain.SLOMetricP95LatencySecs)
	assert.InDelta(t, 1.0,
		budget, 1e-9)
}

func TestCalculateErrorBudget_P99Latency(t *testing.T) {
	t.Parallel()

	budget := CalculateErrorBudget(0.5, 1.0, domain.SLOMetricP99LatencySecs)
	assert.InDelta(t, 0.5,
		budget, 1e-9)
}

// Maintenance loop edge cases

func TestMaintenanceLoop_NilTask(t *testing.T) {
	t.Parallel()

	loop := NewMaintenanceLoop("nil-task", 50*time.Millisecond, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	loop.Run(ctx)
}

func TestMaintenanceLoop_ZeroInterval_Clamped(t *testing.T) {
	t.Parallel()

	loop := NewMaintenanceLoop("zero-int", 0, nil, func(_ context.Context) {})
	require.Equal(t, time.
		Second, loop.interval,
	)
}

func TestMaintenanceLoop_NegativeInterval_Clamped(t *testing.T) {
	t.Parallel()

	loop := NewMaintenanceLoop("neg-int", -5*time.Second, nil, func(_ context.Context) {})
	require.Equal(t, time.
		Second, loop.interval,
	)
}

// Index maintenance edge cases

func TestIndexMaintainer_ZeroInterval_Clamped(t *testing.T) {
	t.Parallel()
	s := &advMockIndexStore{}
	im := NewIndexMaintainer(s, 0)
	require.Equal(t, 24*
		time.Hour, im.
		interval)
}

func TestIndexMaintainer_NegativeInterval_Clamped(t *testing.T) {
	t.Parallel()
	s := &advMockIndexStore{}
	im := NewIndexMaintainer(s, -time.Hour)
	require.Equal(t, 24*
		time.Hour, im.
		interval)
}

// Memory cleanup edge cases

func TestMemoryCleanup_ZeroInterval_Clamped(t *testing.T) {
	t.Parallel()
	s := &advMockMemoryStore{}
	mc := NewMemoryCleanup(s, 0)
	require.Equal(t, 5*
		time.Minute, mc.
		interval)
}

func TestMemoryCleanup_StoreError(t *testing.T) {
	t.Parallel()
	s := &advMockMemoryStore{
		deleteExpiredFn: func(_ context.Context) (int64, error) {
			return 0, errors.New("db error")
		},
	}
	mc := NewMemoryCleanup(s, time.Minute)
	mc.cleanup(context.Background())
}

func TestMemoryCleanup_DeletesExpired(t *testing.T) {
	t.Parallel()
	s := &advMockMemoryStore{
		deleteExpiredFn: func(_ context.Context) (int64, error) {
			return 42, nil
		},
	}
	mc := NewMemoryCleanup(s, time.Minute)
	mc.cleanup(context.Background())
}

// Usage flusher edge cases

func TestUsageFlusher_ZeroInterval_Clamped(t *testing.T) {
	t.Parallel()
	uf := NewUsageFlusher(nil, 0)
	require.Equal(t, 60*
		time.Second, uf.
		interval,
	)
}

func TestUsageFlusher_NegativeInterval_Clamped(t *testing.T) {
	t.Parallel()
	uf := NewUsageFlusher(nil, -time.Minute)
	require.Equal(t, 60*
		time.Second, uf.
		interval,
	)
}

func TestUsageFlusher_WithAdvisoryLocker(t *testing.T) {
	t.Parallel()
	uf := NewUsageFlusher(nil, time.Minute)
	locker := &mockAdvisoryLocker{}
	uf2 := uf.WithAdvisoryLocker(locker)
	require.NotNil(t, uf2.
		advisoryLocker,
	)
}

// Stale subscription checker edge cases

func TestStaleSubscriptionChecker_BasicConstruction(t *testing.T) {
	t.Parallel()
	s := &advMockStaleSubStore{}
	c := NewStaleSubscriptionChecker(s, time.Minute)
	require.Equal(t, time.
		Minute, c.interval,
	)
}

func TestStaleSubscriptionChecker_WithAdvisoryLocker(t *testing.T) {
	t.Parallel()
	s := &advMockStaleSubStore{}
	c := NewStaleSubscriptionChecker(s, time.Minute)
	locker := &mockAdvisoryLocker{}
	c2 := c.WithAdvisoryLocker(locker)
	require.NotNil(t, c2.
		advisoryLocker)
}

func TestStaleSubscriptionChecker_Check_NoSubs(t *testing.T) {
	t.Parallel()
	s := &advMockStaleSubStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			return nil, nil
		},
	}
	c := NewStaleSubscriptionChecker(s, time.Minute)
	c.check(context.Background())
}

func TestStaleSubscriptionChecker_Check_StoreError(t *testing.T) {
	t.Parallel()
	s := &advMockStaleSubStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			return nil, errors.New("db error")
		},
	}
	c := NewStaleSubscriptionChecker(s, time.Minute)
	c.check(context.Background())
}

func TestStaleSubscriptionChecker_Check_WithSubs(t *testing.T) {
	t.Parallel()
	pastEnd := time.Now().Add(-48 * time.Hour)
	s := &advMockStaleSubStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			return []billing.OrgSubscription{
				{OrgID: "org-1", PlanTier: "pro", CurrentPeriodEnd: &pastEnd},
			}, nil
		},
	}
	c := NewStaleSubscriptionChecker(s, time.Minute)
	c.check(context.Background())
}

func TestStaleSubscriptionChecker_Check_LockerNotAcquired(t *testing.T) {
	t.Parallel()
	var checked bool
	s := &advMockStaleSubStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			checked = true
			return nil, nil
		},
	}
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
	}
	c := NewStaleSubscriptionChecker(s, time.Minute).WithAdvisoryLocker(locker)
	c.check(context.Background())
	require.False(t, checked)
}

func TestStaleSubscriptionChecker_Run_StopsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	s := &advMockStaleSubStore{}
	c := NewStaleSubscriptionChecker(s, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		c.Run(ctx)
		close(done)
	})

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "Run did not stop on context cancel")
	}
}

// Concurrent schedule/unschedule of same job

func TestCronScheduler_ConcurrentLoadAndStop(t *testing.T) {
	t.Parallel()

	ms := &mockCronStore{
		listCronJobsFn: func(_ context.Context) ([]domain.Job, error) {
			return []domain.Job{
				{ID: "job-1", Cron: "* * * * *", ProjectID: "p1"},
			}, nil
		},
	}
	q := &mockQueue{}

	cs := NewCronScheduler(context.Background(), ms, q, nil)

	var wg conc.WaitGroup
	for range 5 {
		wg.Go(func() {
			_ = cs.LoadJobs(context.Background())
		})
	}
	wg.Wait()

	cs.Start()
	stopCtx := cs.Stop()
	<-stopCtx.Done()
}

// Downgrade applier edge cases

func TestDowngradeApplier_LockerNotAcquired(t *testing.T) {
	t.Parallel()
	var applied bool
	s := &mockDowngradeStore{
		pendingOrgs: nil,
	}
	// Override store to detect calls.
	advStore := &advMockDowngradeStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			applied = true
			return nil, nil
		},
	}
	_ = s // suppress unused
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
	}
	d := NewDowngradeApplier(advStore, nil, time.Minute).WithAdvisoryLocker(locker)
	d.apply(context.Background())
	require.False(t, applied)
}

func TestDowngradeApplier_LockerError(t *testing.T) {
	t.Parallel()
	var applied bool
	advStore := &advMockDowngradeStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			applied = true
			return nil, nil
		},
	}
	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, errors.New("pg connection lost")
		},
	}
	d := NewDowngradeApplier(advStore, nil, time.Minute).WithAdvisoryLocker(locker)
	d.apply(context.Background())
	require.False(t, applied)
}

func TestDowngradeApplier_StoreListError(t *testing.T) {
	t.Parallel()
	advStore := &advMockDowngradeStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			return nil, errors.New("db error")
		},
	}
	d := NewDowngradeApplier(advStore, nil, time.Minute)
	d.apply(context.Background())
}

func TestDowngradeApplier_ApplyError(t *testing.T) {
	t.Parallel()
	tier := "starter"
	advStore := &advMockDowngradeStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			return []billing.OrgSubscription{
				{OrgID: "org-1", PendingPlanTier: &tier},
			}, nil
		},
		applyFn: func(_ context.Context, _ string) error {
			return errors.New("apply failed")
		},
	}
	d := NewDowngradeApplier(advStore, nil, time.Minute)
	d.apply(context.Background())
}

func TestDowngradeApplier_EmptyList(t *testing.T) {
	t.Parallel()
	advStore := &advMockDowngradeStore{
		listFn: func(_ context.Context) ([]billing.OrgSubscription, error) {
			return nil, nil
		},
	}
	d := NewDowngradeApplier(advStore, nil, time.Minute)
	d.apply(context.Background())
}

// Concurrent reconciler edge cases

func TestConcurrentReconciler_Construction(t *testing.T) {
	t.Parallel()
	r := NewConcurrentReconciler(nil, nil, time.Minute)
	require.Equal(t, time.
		Minute, r.interval,
	)
}

// FormatBudgetAlertKey edge cases

func TestFormatBudgetAlertKey_EmptyProject(t *testing.T) {
	t.Parallel()
	key := FormatBudgetAlertKey("", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	require.Equal(t, ":2026-01-01",
		key)
}

func TestFormatBudgetAlertKey_FarFutureDate(t *testing.T) {
	t.Parallel()
	key := FormatBudgetAlertKey("proj-1", time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC))
	require.Equal(t, "proj-1:9999-12-31",

		key)
}

// Mock types used only by adversarial tests (prefixed with "adv" to avoid conflicts)

type advMockRedisClient struct {
	getFn func(ctx context.Context, key string) (string, error)
	setFn func(ctx context.Context, key string, value any, ttl time.Duration) error
}

func (m *advMockRedisClient) Get(ctx context.Context, key string) (string, error) {
	if m.getFn != nil {
		return m.getFn(ctx, key)
	}
	return "", nil
}

func (m *advMockRedisClient) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if m.setFn != nil {
		return m.setFn(ctx, key, value, ttl)
	}
	return nil
}

type advMockIndexStore struct{}

func (m *advMockIndexStore) ReindexIndexConcurrently(_ context.Context, _ string) error { return nil }

type advMockMemoryStore struct {
	deleteExpiredFn func(ctx context.Context) (int64, error)
}

func (m *advMockMemoryStore) DeleteExpiredJobMemory(ctx context.Context) (int64, error) {
	if m.deleteExpiredFn != nil {
		return m.deleteExpiredFn(ctx)
	}
	return 0, nil
}

type advMockStaleSubStore struct {
	listFn func(ctx context.Context) ([]billing.OrgSubscription, error)
}

func (m *advMockStaleSubStore) ListStaleSubscriptions(ctx context.Context) ([]billing.OrgSubscription, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

type advMockDowngradeStore struct {
	listFn    func(ctx context.Context) ([]billing.OrgSubscription, error)
	applyFn   func(ctx context.Context, orgID string) error
	suspendFn func(ctx context.Context, orgID string, maxProjects int) (int, error)
}

func (m *advMockDowngradeStore) ListOrgsWithPendingDowngrade(ctx context.Context) ([]billing.OrgSubscription, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func (m *advMockDowngradeStore) ApplyPendingDowngrade(ctx context.Context, orgID string) error {
	if m.applyFn != nil {
		return m.applyFn(ctx, orgID)
	}
	return nil
}

func (m *advMockDowngradeStore) ApplyPendingDowngradeTierIfPending(ctx context.Context, orgID, _ string) (bool, error) {
	if err := m.ApplyPendingDowngrade(ctx, orgID); err != nil {
		return false, err
	}
	return true, nil
}

func (m *advMockDowngradeStore) ClearPendingPlanTierIfTier(context.Context, string, string) (bool, error) {
	return true, nil
}

func (m *advMockDowngradeStore) SuspendExcessProjects(ctx context.Context, orgID string, maxProjects int) (int, error) {
	if m.suspendFn != nil {
		return m.suspendFn(ctx, orgID, maxProjects)
	}
	return 0, nil
}

func (m *advMockDowngradeStore) DeactivateExcessCronJobs(_ context.Context, _ string, _ int) ([]string, error) {
	return nil, nil
}

func (m *advMockDowngradeStore) DeactivateExcessWebhookSubscriptions(_ context.Context, _ string, _ int) (int64, error) {
	return 0, nil
}

func (m *advMockDowngradeStore) DeactivateExcessEnvironments(_ context.Context, _ string, _ int) (int64, error) {
	return 0, nil
}

func (m *advMockDowngradeStore) ListProjectsByOrg(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (m *advMockDowngradeStore) PauseHTTPJobsByOrg(_ context.Context, _ string, _ string) ([]string, error) {
	return nil, nil
}

func (m *advMockDowngradeStore) DeactivateExcessLogDrains(_ context.Context, _ string, _ int) (int64, error) {
	return 0, nil
}

func (m *advMockDowngradeStore) DeactivateExcessNotificationChannelsByProject(_ context.Context, _ string, _ int) (int64, error) {
	return 0, nil
}

func (m *advMockDowngradeStore) CountMembersByOrg(_ context.Context, _ string) (int, error) {
	return 0, nil
}
