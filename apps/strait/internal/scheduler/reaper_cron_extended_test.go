package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Section separator.
// Reaper retention tests (per-org retention, event triggers)
// Section separator.

func TestReaper_OrgRetention_PrunesRunsByOrg(t *testing.T) {
	t.Parallel()

	var deleteRunsCalled atomic.Int32
	var deleteWfRunsCalled atomic.Int32

	ms := &mockReaperStore{
		deleteOldWorkflowRunsFn: func(_ context.Context, _ time.Time, _ int) (int64, error) {
			return 0, nil
		},
	}

	resolver := &mockOrgRetentionResolver{
		orgIDs: []string{"org-1"},
		retentionDays: map[string]int{
			"org-1": 7,
		},
	}

	// We need a store that also implements delete-by-org methods.
	// The mockReaperStore already has stub implementations for these.
	orgAwareStore := &mockReaperStoreWithOrgRetention{
		mockReaperStore: ms,
		deleteRunsByOrgFn: func(_ context.Context, orgID string, _ time.Duration) (int64, error) {
			if orgID == "org-1" {
				deleteRunsCalled.Add(1)
				return 5, nil
			}
			return 0, nil
		},
		deleteWfRunsByOrgFn: func(_ context.Context, orgID string, _ time.Duration) (int64, error) {
			if orgID == "org-1" {
				deleteWfRunsCalled.Add(1)
				return 3, nil
			}
			return 0, nil
		},
	}

	r := NewReaper(orgAwareStore, time.Second, 5*time.Minute, 30*24*time.Hour, 90*24*time.Hour, true, nil).
		WithOrgRetention(resolver)
	// reapPerOrgRetention is only called from Run (maintenance loop), not ReapOnce.
	// Call it directly since we are in the same package.
	r.reapPerOrgRetention(context.Background())
	assert.EqualValues(t, 1,
		deleteRunsCalled.
			Load())
	assert.EqualValues(t, 1,
		deleteWfRunsCalled.
			Load())
}

func TestReaper_OrgRetention_ResolverError_Continues(t *testing.T) {
	t.Parallel()

	resolver := &mockOrgRetentionResolver{
		listErr: errors.New("db error"),
	}

	ms := &mockReaperStore{}
	r := NewReaper(ms, time.Second, 5*time.Minute, 0, 0, true, nil).
		WithOrgRetention(resolver)
	// Should not panic.
	r.reapPerOrgRetention(context.Background())
}

func TestReaper_OrgRetention_RetentionLookupErrorSkipsDeletes(t *testing.T) {
	t.Parallel()

	resolver := &mockOrgRetentionResolver{
		orgIDs:        []string{"org-1"},
		retentionErrs: map[string]error{"org-1": errors.New("subscription lookup failed")},
	}
	var deleteRunsCalled atomic.Bool
	var deleteWfRunsCalled atomic.Bool
	store := &mockReaperStoreWithOrgRetention{
		mockReaperStore: &mockReaperStore{},
		deleteRunsByOrgFn: func(context.Context, string, time.Duration) (int64, error) {
			deleteRunsCalled.Store(true)
			return 0, nil
		},
		deleteWfRunsByOrgFn: func(context.Context, string, time.Duration) (int64, error) {
			deleteWfRunsCalled.Store(true)
			return 0, nil
		},
	}

	r := NewReaper(store, time.Second, 5*time.Minute, 0, 0, true, nil).
		WithOrgRetention(resolver)
	r.reapPerOrgRetention(context.Background())
	require.False(t, deleteRunsCalled.
		Load() || deleteWfRunsCalled.
		Load(),
	)
}

func TestReaper_OrgRetention_NilResolver_SkipsOrgRetention(t *testing.T) {
	t.Parallel()

	ms := &mockReaperStore{}
	r := NewReaper(ms, time.Second, 5*time.Minute, 0, 0, true, nil)
	// Should not panic with nil org retention.
	r.reapPerOrgRetention(context.Background())
}

func TestReaper_DeleteTerminalRuns_RetentionDisabled(t *testing.T) {
	t.Parallel()

	retentionCalled := false
	ms := &mockReaperStore{
		deleteRetentionFn: func(_ context.Context, _, _ time.Duration) (int64, error) {
			retentionCalled = true
			return 0, nil
		},
	}

	r := NewReaper(ms, time.Second, 5*time.Minute, 30*24*time.Hour, 90*24*time.Hour, false, nil)
	r.ReapOnce(context.Background())
	require.False(t, retentionCalled)
}

func TestReaper_ReapOldEventTriggers_DeletesFinished(t *testing.T) {
	t.Parallel()

	var deleteCalled atomic.Bool
	ms := &mockReaperStore{
		deleteEventTriggersFinishedBeforeFn: func(_ context.Context, _ time.Time, _ int) (int64, error) {
			deleteCalled.Store(true)
			return 10, nil
		},
	}

	r := NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, nil).
		WithEventTriggerRetention(30 * 24 * time.Hour)
	r.ReapOnce(context.Background())
	require.True(t, deleteCalled.
		Load())
}

func TestReaper_ReapOldEventTriggers_DeleteError_NoPanic(t *testing.T) {
	t.Parallel()

	ms := &mockReaperStore{
		deleteEventTriggersFinishedBeforeFn: func(_ context.Context, _ time.Time, _ int) (int64, error) {
			return 0, errors.New("delete failed")
		},
	}

	r := NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, nil).
		WithEventTriggerRetention(30 * 24 * time.Hour)
	r.ReapOnce(context.Background()) // should not panic
}

func TestReaper_ReapExpiredEventTriggers_MultipleExpired(t *testing.T) {
	t.Parallel()

	var updateCalls atomic.Int32
	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return []domain.EventTrigger{
				{ID: "et-1", SourceType: "job_run", JobRunID: "jr-1", Status: "waiting"},
				{ID: "et-2", SourceType: "job_run", JobRunID: "jr-2", Status: "waiting"},
			}, nil
		},
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, status string, _ json.RawMessage, _ *time.Time, _ string) error {
			if status == "timed_out" {
				updateCalls.Add(1)
			}
			return nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, nil)
	r.ReapOnce(context.Background())
	require.EqualValues(t, 2,
		updateCalls.Load())
}

func TestReaper_ReapExpiredEventTriggers_ListError(t *testing.T) {
	t.Parallel()

	ms := &mockReaperStore{
		listExpiredEventTriggersFn: func(_ context.Context) ([]domain.EventTrigger, error) {
			return nil, errors.New("list failed")
		},
	}

	r := NewReaper(ms, time.Second, 5*time.Minute, 0, 0, false, nil)
	r.ReapOnce(context.Background()) // should not panic
}

// Section separator.
// Cron scheduler extended tests.
// Section separator.

func TestCronScheduler_LoadJobs_WorkflowsAndJobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := &mockCronStore{
		listCronJobsFn: func(_ context.Context) ([]domain.Job, error) {
			return []domain.Job{
				{ID: "j-1", ProjectID: "proj-1", Cron: "*/5 * * * *"},
			}, nil
		},
		listCronWorkflowsFn: func(_ context.Context) ([]domain.Workflow, error) {
			return []domain.Workflow{
				{ID: "wf-1", ProjectID: "proj-1", Cron: "*/10 * * * *"},
			}, nil
		},
	}

	wt := &extMockWorkflowTrigger{}
	cs := NewCronScheduler(ctx, s, &mockQueue{}, wt)
	require.NoError(t,
		cs.LoadJobs(ctx))

	cs.Start()
	stopCtx := cs.Stop()
	<-stopCtx.Done()
}

func TestCronScheduler_TriggerJob_DefaultRunTTL(t *testing.T) {
	t.Parallel()

	var enqueuedRun *domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedRun = run
			return nil
		},
	}

	ctx := context.Background()
	cs := NewCronScheduler(ctx, &mockCronStore{}, q, nil).
		WithDefaultRunTTLSecs(3600)

	job := domain.Job{
		ID:        "j-ttl",
		ProjectID: "proj-1",
		Cron:      "* * * * *",
	}
	cs.triggerJob(ctx, job)
	require.NotNil(t, enqueuedRun)
	require.NotNil(t, enqueuedRun.
		ExpiresAt,
	)

	expectedMin := time.Now().Add(3590 * time.Second)
	assert.False(t, enqueuedRun.
		ExpiresAt.
		Before(expectedMin))
}

func TestCronScheduler_TriggerJob_JobTTLOverridesDefault(t *testing.T) {
	t.Parallel()

	var enqueuedRun *domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedRun = run
			return nil
		},
	}

	ctx := context.Background()
	cs := NewCronScheduler(ctx, &mockCronStore{}, q, nil).
		WithDefaultRunTTLSecs(3600)

	job := domain.Job{
		ID:         "j-ttl-override",
		ProjectID:  "proj-1",
		Cron:       "* * * * *",
		RunTTLSecs: 60, // 1 minute
	}
	cs.triggerJob(ctx, job)
	require.NotNil(t, enqueuedRun)
	require.NotNil(t, enqueuedRun.
		ExpiresAt,
	)

	// Job-level TTL (60s) should take precedence over default (3600s).
	maxExpiry := time.Now().Add(70 * time.Second)
	assert.False(t, enqueuedRun.
		ExpiresAt.
		After(maxExpiry),
	)
}

func TestCronScheduler_TriggerWorkflow_AlreadyRunning_Skips(t *testing.T) {
	t.Parallel()

	var triggerCalled atomic.Bool
	s := &mockCronStore{
		countRunningWfRunsFn: func(_ context.Context, _ string) (int, error) {
			return 1, nil // already running
		},
	}

	wt := &extMockWorkflowTrigger{
		triggerFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride, _ map[string]string) (*domain.WorkflowRun, error) {
			triggerCalled.Store(true)
			return &domain.WorkflowRun{}, nil
		},
	}

	ctx := context.Background()
	cs := NewCronScheduler(ctx, s, &mockQueue{}, wt)

	wf := domain.Workflow{
		ID:            "wf-skip",
		ProjectID:     "proj-1",
		Cron:          "* * * * *",
		SkipIfRunning: true,
	}
	cs.triggerWorkflow(ctx, wf)
	require.False(t, triggerCalled.
		Load())
}

func TestCronScheduler_TriggerWorkflow_CountError_Aborts(t *testing.T) {
	t.Parallel()

	var triggerCalled atomic.Bool
	s := &mockCronStore{
		countRunningWfRunsFn: func(_ context.Context, _ string) (int, error) {
			return 0, errors.New("count error")
		},
	}

	wt := &extMockWorkflowTrigger{
		triggerFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride, _ map[string]string) (*domain.WorkflowRun, error) {
			triggerCalled.Store(true)
			return &domain.WorkflowRun{}, nil
		},
	}

	ctx := context.Background()
	cs := NewCronScheduler(ctx, s, &mockQueue{}, wt)

	wf := domain.Workflow{
		ID:            "wf-count-err",
		ProjectID:     "proj-1",
		Cron:          "* * * * *",
		SkipIfRunning: true,
	}
	cs.triggerWorkflow(ctx, wf)
	require.False(t, triggerCalled.
		Load())
}

func TestCronScheduler_TriggerWorkflow_TriggerError_NoPanic(t *testing.T) {
	t.Parallel()

	s := &mockCronStore{
		countRunningWfRunsFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
	}

	wt := &extMockWorkflowTrigger{
		triggerFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride, _ map[string]string) (*domain.WorkflowRun, error) {
			return nil, errors.New("trigger failed")
		},
	}

	ctx := context.Background()
	cs := NewCronScheduler(ctx, s, &mockQueue{}, wt)

	wf := domain.Workflow{
		ID:            "wf-fail",
		ProjectID:     "proj-1",
		Cron:          "* * * * *",
		SkipIfRunning: true,
	}
	cs.triggerWorkflow(ctx, wf) // should not panic
}

func TestCronScheduler_TriggerJob_EnqueueError_NoPanic(t *testing.T) {
	t.Parallel()

	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("enqueue failed")
		},
	}

	ctx := context.Background()
	cs := NewCronScheduler(ctx, &mockCronStore{}, q, nil)

	job := domain.Job{
		ID:        "j-enqueue-err",
		ProjectID: "proj-1",
		Cron:      "* * * * *",
	}
	cs.triggerJob(ctx, job) // should not panic
}

func TestCronScheduler_TriggerWorkflow_WithTimezone_Ext(t *testing.T) {
	t.Parallel()

	var triggerCalled atomic.Bool
	s := &mockCronStore{
		listCronJobsFn: func(_ context.Context) ([]domain.Job, error) {
			return nil, nil
		},
		listCronWorkflowsFn: func(_ context.Context) ([]domain.Workflow, error) {
			return []domain.Workflow{
				{ID: "wf-tz", ProjectID: "proj-1", Cron: "0 9 * * *", CronTimezone: "America/New_York"},
			}, nil
		},
	}

	wt := &extMockWorkflowTrigger{
		triggerFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride, _ map[string]string) (*domain.WorkflowRun, error) {
			triggerCalled.Store(true)
			return &domain.WorkflowRun{}, nil
		},
	}

	ctx := context.Background()
	cs := NewCronScheduler(ctx, s, &mockQueue{}, wt)
	require.NoError(t,
		cs.LoadJobs(ctx))

	// Just verify it loads without error; timezone processing is handled by the cron library.
	cs.Start()
	stopCtx := cs.Stop()
	<-stopCtx.Done()
}

// Section separator.
// Usage Flusher extended tests.
// Section separator.

func TestUsageFlusher_WithAdvisoryLock_NotAcquired(t *testing.T) {
	t.Parallel()

	var listCalled atomic.Bool
	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			listCalled.Store(true)
			return nil, nil
		},
	}

	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
	}

	uf := NewUsageFlusher(s, time.Minute).WithAdvisoryLocker(locker)
	uf.flush(context.Background())
	require.False(t, listCalled.
		Load())
}

func TestUsageFlusher_WithAdvisoryLock_AcquireError(t *testing.T) {
	t.Parallel()

	var listCalled atomic.Bool
	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			listCalled.Store(true)
			return nil, nil
		},
	}

	locker := &mockAdvisoryLocker{
		acquireFn: func(_ context.Context, _ int64) (bool, error) {
			return false, errors.New("lock error")
		},
	}

	uf := NewUsageFlusher(s, time.Minute).WithAdvisoryLocker(locker)
	uf.flush(context.Background())
	require.False(t, listCalled.
		Load())
}

func TestUsageFlusher_ListOrgsError_Ext(t *testing.T) {
	t.Parallel()

	var upsertCalled atomic.Bool
	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return nil, errors.New("db error")
		},
		replaceUsageRecordFn: func(_ context.Context, _ *billing.UsageRecord) error {
			upsertCalled.Store(true)
			return nil
		},
	}

	uf := NewUsageFlusher(s, time.Minute)
	uf.flush(context.Background())
	require.False(t, upsertCalled.
		Load(),
	)
}

func TestUsageFlusher_UpsertError_ContinuesOtherRecords_Ext(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	var mu sync.Mutex
	var upsertCalls int

	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgDailyUsageFn: func(_ context.Context, _ string, _ time.Time) ([]billing.UsageRecord, error) {
			return []billing.UsageRecord{
				{OrgID: "org-1", ProjectID: "proj-fail", PeriodDate: today, RunsCount: 1},
				{OrgID: "org-1", ProjectID: "proj-ok", PeriodDate: today, RunsCount: 2},
			}, nil
		},
		replaceUsageRecordFn: func(_ context.Context, rec *billing.UsageRecord) error {
			mu.Lock()
			upsertCalls++
			mu.Unlock()
			if rec.ProjectID == "proj-fail" {
				return errors.New("upsert failed")
			}
			return nil
		},
	}

	uf := NewUsageFlusher(s, time.Minute)
	uf.flush(context.Background())

	mu.Lock()
	defer mu.Unlock()
	wantCalls := usageFlusherReconcileLookbackDays * 2
	require.Equal(t, wantCalls,
		upsertCalls,
	)
}

func TestUsageFlusher_DefaultInterval(t *testing.T) {
	t.Parallel()

	uf := NewUsageFlusher(&mockUsageFlusherStore{}, 0)
	require.Equal(t, 60*
		time.Second, uf.
		interval,
	)
}

func TestUsageFlusher_Run_StopsOnContextCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	uf := NewUsageFlusher(&mockUsageFlusherStore{}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		uf.Run(ctx)
		close(done)
	})

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "Run did not stop on context cancel")
	}
}

// Section separator.
// Usage Report Emailer extended tests.
// Section separator.

func TestUsageReportEmailer_Run_StopsOnContextCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	emailer := NewUsageReportEmailer(nil, nil, "", 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		emailer.Run(ctx)
		close(done)
	})

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "Run did not stop on context cancel")
	}
}

func TestUsageReportEmailer_DefaultFromEmail(t *testing.T) {
	t.Parallel()

	emailer := NewUsageReportEmailer(nil, nil, "", time.Hour)
	require.Equal(t, "billing@strait.dev",

		emailer.
			fromEmail,
	)
}

func TestUsageReportEmailer_DefaultInterval(t *testing.T) {
	t.Parallel()

	emailer := NewUsageReportEmailer(nil, nil, "", 0)
	require.Equal(t, time.
		Hour, emailer.
		interval)
}

func TestUsageReportEmailer_SameDayDedup(t *testing.T) {
	t.Parallel()

	emailer := NewUsageReportEmailer(nil, nil, "", time.Hour)
	// Simulate that checkAndSend was already called today.
	emailer.lastRunDate = time.Now().UTC().Format("2006-01-02")

	// The dedup is inside checkAndSend: if lastRunDate == today, it returns early.
	// If store was nil it would panic if it tried to call store methods,
	// so the fact that it doesn't panic proves dedup works.
	emailer.checkAndSend(context.Background())
	// It should return before calling store methods. If store was nil it would panic
	// if it tried to call store methods, so the fact that it doesn't panic proves dedup works.
}

// Section separator.
// Helper mock types.
// Section separator.

type mockOrgRetentionResolver struct {
	orgIDs        []string
	retentionDays map[string]int
	retentionErrs map[string]error
	listErr       error
}

func (m *mockOrgRetentionResolver) ListAllSubscribedOrgIDs(_ context.Context) ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.orgIDs, nil
}

func (m *mockOrgRetentionResolver) GetOrgRetentionDays(_ context.Context, orgID string) (int, error) {
	if err, ok := m.retentionErrs[orgID]; ok {
		return 0, err
	}
	if days, ok := m.retentionDays[orgID]; ok {
		return days, nil
	}
	return 30, nil
}

type mockReaperStoreWithOrgRetention struct {
	*mockReaperStore
	deleteRunsByOrgFn   func(ctx context.Context, orgID string, retention time.Duration) (int64, error)
	deleteWfRunsByOrgFn func(ctx context.Context, orgID string, retention time.Duration) (int64, error)
}

func (m *mockReaperStoreWithOrgRetention) DeleteRunsByOrgOlderThan(ctx context.Context, orgID string, retention time.Duration) (int64, error) {
	if m.deleteRunsByOrgFn != nil {
		return m.deleteRunsByOrgFn(ctx, orgID, retention)
	}
	return 0, nil
}

func (m *mockReaperStoreWithOrgRetention) DeleteWorkflowRunsByOrgOlderThan(ctx context.Context, orgID string, retention time.Duration) (int64, error) {
	if m.deleteWfRunsByOrgFn != nil {
		return m.deleteWfRunsByOrgFn(ctx, orgID, retention)
	}
	return 0, nil
}

type extMockWorkflowTrigger struct {
	triggerFn func(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, stepOverrides []domain.StepOverride, extraTags map[string]string) (*domain.WorkflowRun, error)
}

func (m *extMockWorkflowTrigger) TriggerWorkflow(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, stepOverrides []domain.StepOverride, extraTags map[string]string) (*domain.WorkflowRun, error) {
	if m.triggerFn != nil {
		return m.triggerFn(ctx, workflowID, projectID, payload, triggeredBy, stepOverrides, extraTags)
	}
	return &domain.WorkflowRun{}, nil
}
