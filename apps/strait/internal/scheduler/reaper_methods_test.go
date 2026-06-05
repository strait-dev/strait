package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// handleCostGateTimeout tests.

// mockCostGateReaperStore composes mockReaperStore with CostGateDefaultActionStore
// and a stepRunGetter for step run lookups.
type mockCostGateReaperStore struct {
	mockReaperStore
	getCostGateDefaultActionFn func(ctx context.Context, stepRunID string) (string, error)
	getWorkflowStepRunFn       func(ctx context.Context, id string) (*domain.WorkflowStepRun, error)
}

func (m *mockCostGateReaperStore) GetCostGateDefaultAction(ctx context.Context, stepRunID string) (string, error) {
	if m.getCostGateDefaultActionFn != nil {
		return m.getCostGateDefaultActionFn(ctx, stepRunID)
	}
	return "", nil
}

func (m *mockCostGateReaperStore) GetWorkflowStepRun(ctx context.Context, id string) (*domain.WorkflowStepRun, error) {
	if m.getWorkflowStepRunFn != nil {
		return m.getWorkflowStepRunFn(ctx, id)
	}
	return nil, nil
}

func TestReaper_HandleCostGateTimeout_AutoApprove(t *testing.T) {
	t.Parallel()

	var approvedStepRef string
	var approvedWorkflowRunID string
	cb := &mockWorkflowCallback{
		approveStepFn: func(_ context.Context, workflowRunID, stepRef, approver string) error {
			approvedWorkflowRunID = workflowRunID
			approvedStepRef = stepRef
			assert.Equal(t, "system:cost-gate-timeout",

				approver)

			return nil
		},
	}

	ms := &mockCostGateReaperStore{
		getCostGateDefaultActionFn: func(_ context.Context, _ string) (string, error) {
			return "approve", nil
		},
		getWorkflowStepRunFn: func(_ context.Context, id string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID:      id,
				StepRef: "deploy-step",
			}, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, cb)

	approval := &domain.WorkflowStepApproval{
		ID:                "appr-1",
		WorkflowRunID:     "wr-1",
		WorkflowStepRunID: "sr-1",
	}

	handled := r.handleCostGateTimeout(context.Background(), approval)
	require.True(t, handled)
	assert.Equal(t, "wr-1",
		approvedWorkflowRunID,
	)
	assert.Equal(t, "deploy-step",

		approvedStepRef,
	)
}

func TestReaper_HandleCostGateTimeout_AutoReject(t *testing.T) {
	t.Parallel()

	approveCalled := false
	cb := &mockWorkflowCallback{
		approveStepFn: func(_ context.Context, _, _, _ string) error {
			approveCalled = true
			return nil
		},
	}

	ms := &mockCostGateReaperStore{
		getCostGateDefaultActionFn: func(_ context.Context, _ string) (string, error) {
			return "reject", nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, cb)

	approval := &domain.WorkflowStepApproval{
		ID:                "appr-1",
		WorkflowRunID:     "wr-1",
		WorkflowStepRunID: "sr-1",
	}

	handled := r.handleCostGateTimeout(context.Background(), approval)
	require.False(t, handled)
	require.False(t, approveCalled)
}

func TestReaper_HandleCostGateTimeout_NilCallback(t *testing.T) {
	t.Parallel()

	ms := &mockCostGateReaperStore{}
	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)

	approval := &domain.WorkflowStepApproval{
		ID:                "appr-1",
		WorkflowRunID:     "wr-1",
		WorkflowStepRunID: "sr-1",
	}

	handled := r.handleCostGateTimeout(context.Background(), approval)
	require.False(t, handled)
}

func TestReaper_HandleCostGateTimeout_NoCostGateStore(t *testing.T) {
	t.Parallel()

	cb := &mockWorkflowCallback{}
	// Plain mockReaperStore does not implement CostGateDefaultActionStore.
	ms := &mockReaperStore{}
	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, cb)

	approval := &domain.WorkflowStepApproval{
		ID:                "appr-1",
		WorkflowRunID:     "wr-1",
		WorkflowStepRunID: "sr-1",
	}

	handled := r.handleCostGateTimeout(context.Background(), approval)
	require.False(t, handled)
}

func TestReaper_HandleCostGateTimeout_StoreError(t *testing.T) {
	t.Parallel()

	cb := &mockWorkflowCallback{}
	ms := &mockCostGateReaperStore{
		getCostGateDefaultActionFn: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("db error")
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, cb)

	approval := &domain.WorkflowStepApproval{
		ID:                "appr-1",
		WorkflowRunID:     "wr-1",
		WorkflowStepRunID: "sr-1",
	}

	handled := r.handleCostGateTimeout(context.Background(), approval)
	require.False(t, handled)
}

func TestReaper_HandleCostGateTimeout_StepRunNotFound(t *testing.T) {
	t.Parallel()

	approveCalled := false
	cb := &mockWorkflowCallback{
		approveStepFn: func(_ context.Context, _, _, _ string) error {
			approveCalled = true
			return nil
		},
	}

	ms := &mockCostGateReaperStore{
		getCostGateDefaultActionFn: func(_ context.Context, _ string) (string, error) {
			return "approve", nil
		},
		getWorkflowStepRunFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return nil, errors.New("not found")
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, cb)

	approval := &domain.WorkflowStepApproval{
		ID:                "appr-1",
		WorkflowRunID:     "wr-1",
		WorkflowStepRunID: "sr-1",
	}

	handled := r.handleCostGateTimeout(context.Background(), approval)
	require.False(t, handled)
	require.False(t, approveCalled)
}

func TestReaper_HandleCostGateTimeout_ApproveStepError(t *testing.T) {
	t.Parallel()

	cb := &mockWorkflowCallback{
		approveStepFn: func(_ context.Context, _, _, _ string) error {
			return errors.New("approve failed")
		},
	}

	ms := &mockCostGateReaperStore{
		getCostGateDefaultActionFn: func(_ context.Context, _ string) (string, error) {
			return "approve", nil
		},
		getWorkflowStepRunFn: func(_ context.Context, id string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: id, StepRef: "step-1"}, nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, cb)

	approval := &domain.WorkflowStepApproval{
		ID:                "appr-1",
		WorkflowRunID:     "wr-1",
		WorkflowStepRunID: "sr-1",
	}

	handled := r.handleCostGateTimeout(context.Background(), approval)
	require.False(t, handled)
}

// reapApprovalReminders dedup cleanup tests.

func TestReaper_ReapApprovalReminders_DedupCleanupAfterExpiry(t *testing.T) {
	t.Parallel()

	callCount := 0
	// Use a future expiry so the dedup entry stays valid across calls.
	futureExpiry := time.Now().Add(1 * time.Hour)
	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return []domain.WorkflowStepApproval{
				{ID: "appr-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1", ExpiresAt: &futureExpiry},
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1", WorkflowID: "wf-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1", ProjectID: "proj-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			callCount++
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)

	// First call: should send reminder and add to reminderSent.
	r.reapApprovalReminders(context.Background())
	require.Equal(t, 1,
		callCount,
	)

	// Second call: dedup suppresses the delivery (expiry is in the future).
	r.reapApprovalReminders(context.Background())
	require.Equal(t, 1,
		callCount,
	)

	// Now manually set the dedup entry to a past time to simulate expiry.
	r.reminderSent["appr-1"] = time.Now().Add(-1 * time.Minute)

	// Third call: cleanup removes the expired entry, allowing the reminder to fire again.
	r.reapApprovalReminders(context.Background())
	require.Equal(t, 2,
		callCount,
	)
}

func TestReaper_ReapApprovalReminders_NoExpiresAt_DefaultTTL(t *testing.T) {
	t.Parallel()

	deliveryCount := 0
	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return []domain.WorkflowStepApproval{
				{ID: "appr-no-expire", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1"},
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1", WorkflowID: "wf-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{{ID: "ch-1"}}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, _ *domain.NotificationDelivery) error {
			deliveryCount++
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapApprovalReminders(context.Background())
	require.Equal(t, 1,
		deliveryCount,
	)

	// Second call should be deduped even without ExpiresAt (default 1h TTL).
	r.reapApprovalReminders(context.Background())
	require.Equal(t, 1,
		deliveryCount,
	)
}

func TestReaper_ReapApprovalReminders_MultipleChannels(t *testing.T) {
	t.Parallel()

	var deliveries []*domain.NotificationDelivery
	expires := time.Now().Add(10 * time.Minute)
	ms := &mockNotifierReaperStore{
		listApprovalsPastReminderPointFn: func(_ context.Context) ([]domain.WorkflowStepApproval, error) {
			return []domain.WorkflowStepApproval{
				{ID: "appr-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1", ExpiresAt: &expires},
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", ProjectID: "proj-1", WorkflowID: "wf-1"}, nil
		},
		listEnabledNotificationChannelsFn: func(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
			return []domain.NotificationChannel{
				{ID: "ch-1", ProjectID: "proj-1"},
				{ID: "ch-2", ProjectID: "proj-1"},
				{ID: "ch-3", ProjectID: "proj-1"},
			}, nil
		},
		createNotificationDeliveryFn: func(_ context.Context, d *domain.NotificationDelivery) error {
			deliveries = append(deliveries, d)
			return nil
		},
	}

	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapApprovalReminders(context.Background())
	require.Len(t, deliveries,
		3)

	channelIDs := map[string]bool{}
	for _, d := range deliveries {
		channelIDs[d.ChannelID] = true
	}
	for _, id := range []string{"ch-1", "ch-2", "ch-3"} {
		assert.True(t, channelIDs[id])
	}
}

// reapPerOrgRetention additional tests.

func TestReaper_OrgRetention_MultipleOrgs_PartialErrors(t *testing.T) {
	t.Parallel()

	var deletedOrgs []string
	ms := &mockReaperStoreWithOrgRetention{
		mockReaperStore: &mockReaperStore{},
		deleteRunsByOrgFn: func(_ context.Context, orgID string, _ time.Duration) (int64, error) {
			if orgID == "org-fail" {
				return 0, errors.New("db error")
			}
			deletedOrgs = append(deletedOrgs, orgID)
			return 3, nil
		},
		deleteWfRunsByOrgFn: func(_ context.Context, orgID string, _ time.Duration) (int64, error) {
			if orgID == "org-fail" {
				return 0, errors.New("db error")
			}
			return 2, nil
		},
	}

	resolver := &mockOrgRetentionResolver{
		orgIDs: []string{"org-1", "org-fail", "org-2"},
		retentionDays: map[string]int{
			"org-1":    7,
			"org-fail": 14,
			"org-2":    30,
		},
	}

	r := NewReaper(ms, time.Second, 5*time.Minute, 0, 0, true, nil).
		WithOrgRetention(resolver)
	r.reapPerOrgRetention(context.Background())
	require.Len(t, deletedOrgs,
		2,
	)
	assert.False(t, deletedOrgs[0] != "org-1" ||
		deletedOrgs[1] !=
			"org-2",
	)

	// org-1 and org-2 should be processed; org-fail should be skipped.
}

func TestReaper_OrgRetention_ZeroRetentionDays_Skipped(t *testing.T) {
	t.Parallel()

	var deleteRunsCalled atomic.Int32
	ms := &mockReaperStoreWithOrgRetention{
		mockReaperStore: &mockReaperStore{},
		deleteRunsByOrgFn: func(_ context.Context, _ string, _ time.Duration) (int64, error) {
			deleteRunsCalled.Add(1)
			return 0, nil
		},
		deleteWfRunsByOrgFn: func(_ context.Context, _ string, _ time.Duration) (int64, error) {
			return 0, nil
		},
	}

	resolver := &mockOrgRetentionResolver{
		orgIDs: []string{"org-zero"},
		retentionDays: map[string]int{
			"org-zero": 0,
		},
	}

	r := NewReaper(ms, time.Second, 5*time.Minute, 0, 0, true, nil).
		WithOrgRetention(resolver)
	r.reapPerOrgRetention(context.Background())
	require.EqualValues(t, 0,
		deleteRunsCalled.
			Load())
}

func TestReaper_OrgRetention_NegativeRetentionDays_Skipped(t *testing.T) {
	t.Parallel()

	var deleteRunsCalled atomic.Int32
	ms := &mockReaperStoreWithOrgRetention{
		mockReaperStore: &mockReaperStore{},
		deleteRunsByOrgFn: func(_ context.Context, _ string, _ time.Duration) (int64, error) {
			deleteRunsCalled.Add(1)
			return 0, nil
		},
		deleteWfRunsByOrgFn: func(_ context.Context, _ string, _ time.Duration) (int64, error) {
			return 0, nil
		},
	}

	resolver := &mockOrgRetentionResolver{
		orgIDs: []string{"org-neg"},
		retentionDays: map[string]int{
			"org-neg": -5,
		},
	}

	r := NewReaper(ms, time.Second, 5*time.Minute, 0, 0, true, nil).
		WithOrgRetention(resolver)
	r.reapPerOrgRetention(context.Background())
	require.EqualValues(t, 0,
		deleteRunsCalled.
			Load())
}
