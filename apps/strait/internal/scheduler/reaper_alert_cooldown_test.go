package scheduler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

type alertMonitorStore struct {
	*mockReaperStore

	dlqDepths    []DLQJobDepth
	dlqErr       error
	dlqCallCount int

	queueDepths    []store.QueueJobDepth
	queueErr       error
	queueCallCount int

	orphans       []store.OrphanedStepRun
	orphansErr    error
	orphansCalled int

	stuckWebhookCount int64
	stuckWebhookErr   error
	stuckWebhookCalls int
}

func (s *alertMonitorStore) ListDLQDepthByJob(_ context.Context) ([]DLQJobDepth, error) {
	s.dlqCallCount++
	return s.dlqDepths, s.dlqErr
}

func (s *alertMonitorStore) ListQueueDepthByJob(_ context.Context) ([]store.QueueJobDepth, error) {
	s.queueCallCount++
	return s.queueDepths, s.queueErr
}

func (s *alertMonitorStore) ListOrphanedStepRuns(_ context.Context) ([]store.OrphanedStepRun, error) {
	s.orphansCalled++
	return s.orphans, s.orphansErr
}

func (s *alertMonitorStore) ResetStuckWebhookDeliveries(_ context.Context) (int64, error) {
	s.stuckWebhookCalls++
	return s.stuckWebhookCount, s.stuckWebhookErr
}

func TestPruneAlertCooldowns_DropsStaleEntries(t *testing.T) {
	t.Parallel()

	r := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, false, nil)

	now := time.Now()
	staleCutoff := now.Add(-25 * time.Hour)
	freshCutoff := now.Add(-5 * time.Minute)

	for i := range 500 {
		r.dlqAlertCooldown[fmt.Sprintf("stale-dlq-%d", i)] = staleCutoff
		r.queueAlertCooldown[fmt.Sprintf("stale-q-%d", i)] = staleCutoff
	}
	for i := range 500 {
		r.dlqAlertCooldown[fmt.Sprintf("fresh-dlq-%d", i)] = freshCutoff
		r.queueAlertCooldown[fmt.Sprintf("fresh-q-%d", i)] = freshCutoff
	}
	require.Len(t, r.dlqAlertCooldown, 1000)
	require.Len(t, r.queueAlertCooldown, 1000)

	r.pruneAlertCooldowns(now)
	require.Len(t, r.dlqAlertCooldown, 500)
	require.Len(t, r.queueAlertCooldown, 500)

	for i := range 500 {
		if _, ok := r.dlqAlertCooldown[fmt.Sprintf("stale-dlq-%d", i)]; ok {
			require.Failf(t, "test failure",

				"stale dlq entry %d should have been pruned", i)
		}
		if _, ok := r.queueAlertCooldown[fmt.Sprintf("stale-q-%d", i)]; ok {
			require.Failf(t, "test failure",

				"stale queue entry %d should have been pruned", i)
		}
		if _, ok := r.dlqAlertCooldown[fmt.Sprintf("fresh-dlq-%d", i)]; !ok {
			require.Failf(t, "test failure",

				"fresh dlq entry %d should have survived pruning", i)
		}
		if _, ok := r.queueAlertCooldown[fmt.Sprintf("fresh-q-%d", i)]; !ok {
			require.Failf(t, "test failure",

				"fresh queue entry %d should have survived pruning", i)
		}
	}
}

func TestMonitorDLQDepth_RecordsNewAlertsAndSkipsFreshCooldown(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := &alertMonitorStore{
		mockReaperStore: &mockReaperStore{},
		dlqDepths: []DLQJobDepth{
			{JobID: "new-job", DLQCount: 7, DLQAlertThreshold: 3},
			{JobID: "fresh-job", DLQCount: 8, DLQAlertThreshold: 4},
			{JobID: "stale-job", DLQCount: 9, DLQAlertThreshold: 5},
		},
	}
	r := NewReaper(store, time.Second, 30*time.Second, 0, 0, false, nil)
	r.dlqAlertCooldown["fresh-job"] = now
	r.dlqAlertCooldown["stale-job"] = now.Add(-25 * time.Hour)

	r.monitorDLQDepth(context.Background())

	require.Equal(t, 1, store.dlqCallCount)
	require.Contains(t, r.dlqAlertCooldown, "new-job")
	require.Equal(t, now, r.dlqAlertCooldown["fresh-job"])
	require.True(t, r.dlqAlertCooldown["stale-job"].After(now))
}

func TestMonitorDLQDepth_StoreErrorDoesNotChangeCooldowns(t *testing.T) {
	t.Parallel()

	store := &alertMonitorStore{
		mockReaperStore: &mockReaperStore{},
		dlqErr:          errors.New("dlq query failed"),
	}
	r := NewReaper(store, time.Second, 30*time.Second, 0, 0, false, nil)
	r.dlqAlertCooldown["existing-job"] = time.Now()

	r.monitorDLQDepth(context.Background())

	require.Equal(t, 1, store.dlqCallCount)
	require.Len(t, r.dlqAlertCooldown, 1)
	require.Contains(t, r.dlqAlertCooldown, "existing-job")
}

func TestMonitorQueueDepth_RecordsNewAlertsAndSkipsFreshCooldown(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := &alertMonitorStore{
		mockReaperStore: &mockReaperStore{},
		queueDepths: []store.QueueJobDepth{
			{JobID: "new-job", QueuedCount: 70, QueueDepthAlertThreshold: 30},
			{JobID: "fresh-job", QueuedCount: 80, QueueDepthAlertThreshold: 40},
			{JobID: "stale-job", QueuedCount: 90, QueueDepthAlertThreshold: 50},
		},
	}
	r := NewReaper(store, time.Second, 30*time.Second, 0, 0, false, nil)
	r.queueAlertCooldown["fresh-job"] = now
	r.queueAlertCooldown["stale-job"] = now.Add(-25 * time.Hour)

	r.monitorQueueDepth(context.Background())

	require.Equal(t, 1, store.queueCallCount)
	require.Contains(t, r.queueAlertCooldown, "new-job")
	require.Equal(t, now, r.queueAlertCooldown["fresh-job"])
	require.True(t, r.queueAlertCooldown["stale-job"].After(now))
}

func TestMonitorQueueDepth_StoreErrorDoesNotChangeCooldowns(t *testing.T) {
	t.Parallel()

	store := &alertMonitorStore{
		mockReaperStore: &mockReaperStore{},
		queueErr:        errors.New("queue query failed"),
	}
	r := NewReaper(store, time.Second, 30*time.Second, 0, 0, false, nil)
	r.queueAlertCooldown["existing-job"] = time.Now()

	r.monitorQueueDepth(context.Background())

	require.Equal(t, 1, store.queueCallCount)
	require.Len(t, r.queueAlertCooldown, 1)
	require.Contains(t, r.queueAlertCooldown, "existing-job")
}

func TestReapOrphanedStepRuns_DispatchesTerminalCallbacks(t *testing.T) {
	t.Parallel()

	store := &alertMonitorStore{
		mockReaperStore: &mockReaperStore{},
		orphans: []store.OrphanedStepRun{
			{
				StepRunID:     "step-complete",
				WorkflowRunID: "workflow-1",
				JobRunID:      "run-1",
				JobStatus:     "completed",
			},
			{
				StepRunID:     "step-failed",
				WorkflowRunID: "workflow-2",
				JobRunID:      "run-2",
				JobStatus:     "failed",
			},
		},
	}
	var completed []string
	var failed []string
	callback := &mockWorkflowCallback{
		onStepCompletedFn: func(_ context.Context, workflowRunID string, stepRunID string) {
			completed = append(completed, workflowRunID+":"+stepRunID)
		},
		onStepFailedFn: func(_ context.Context, workflowRunID string, stepRunID string) {
			failed = append(failed, workflowRunID+":"+stepRunID)
		},
	}
	r := NewReaper(store, time.Second, 30*time.Second, 0, 0, false, callback)

	r.reapOrphanedStepRuns(context.Background())

	require.Equal(t, 1, store.orphansCalled)
	require.Equal(t, []string{"workflow-1:step-complete"}, completed)
	require.Equal(t, []string{"workflow-2:step-failed"}, failed)
}

func TestReapOrphanedStepRuns_StoreErrorSkipsCallbacks(t *testing.T) {
	t.Parallel()

	store := &alertMonitorStore{
		mockReaperStore: &mockReaperStore{},
		orphansErr:      errors.New("orphan query failed"),
	}
	callback := &mockWorkflowCallback{
		onStepCompletedFn: func(context.Context, string, string) {
			require.Fail(t, "completed callback should not run after store error")
		},
		onStepFailedFn: func(context.Context, string, string) {
			require.Fail(t, "failed callback should not run after store error")
		},
	}
	r := NewReaper(store, time.Second, 30*time.Second, 0, 0, false, callback)

	r.reapOrphanedStepRuns(context.Background())

	require.Equal(t, 1, store.orphansCalled)
}

func TestReapStuckWebhookDeliveries_LogsOnlyPositiveResets(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		resetCount    int64
		wantLogRecord bool
	}{
		{name: "zero", resetCount: 0},
		{name: "positive", resetCount: 3, wantLogRecord: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &alertMonitorStore{
				mockReaperStore:   &mockReaperStore{},
				stuckWebhookCount: tc.resetCount,
			}
			var logs bytes.Buffer
			r := NewReaper(store, time.Second, 30*time.Second, 0, 0, false, nil)
			r.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))

			r.reapStuckWebhookDeliveries(context.Background())

			require.Equal(t, 1, store.stuckWebhookCalls)
			require.Equal(t, tc.wantLogRecord, strings.Contains(logs.String(), `level=INFO msg="reset stuck webhook deliveries"`))
		})
	}
}

func TestReapStuckWebhookDeliveries_StoreErrorSkipsResetLog(t *testing.T) {
	t.Parallel()

	store := &alertMonitorStore{
		mockReaperStore: &mockReaperStore{},
		stuckWebhookErr: errors.New("reset failed"),
	}
	var logs bytes.Buffer
	r := NewReaper(store, time.Second, 30*time.Second, 0, 0, false, nil)
	r.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))

	r.reapStuckWebhookDeliveries(context.Background())

	require.Equal(t, 1, store.stuckWebhookCalls)
	require.NotContains(t, logs.String(), `level=INFO msg="reset stuck webhook deliveries"`)
}

func TestPruneAlertCooldowns_DoesNotBlockNewAlerts(t *testing.T) {
	t.Parallel()

	r := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, false, nil)

	now := time.Now()
	r.dlqAlertCooldown["seen-job"] = now.Add(-100 * time.Hour)

	r.pruneAlertCooldowns(now)

	if _, ok := r.dlqAlertCooldown["seen-job"]; ok {
		require.Fail(t,

			"expected stale entry to be removed so a new alert can fire")
	}
}
