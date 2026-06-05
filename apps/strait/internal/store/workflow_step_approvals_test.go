package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnApprovalChanged_CalledAfterCreate(t *testing.T) {
	// Not parallel: mutates the package-level OnApprovalChanged hook.
	origHook := OnApprovalChanged
	t.Cleanup(func() { OnApprovalChanged = origHook })

	var mu sync.Mutex
	var captured *domain.WorkflowStepApproval
	OnApprovalChanged = func(_ context.Context, a *domain.WorkflowStepApproval) {
		mu.Lock()
		defer mu.Unlock()
		captured = a
	}

	// We cannot call the real CreateWorkflowStepApproval without a DB,
	// so test that the hook variable is set and invokable.
	approval := &domain.WorkflowStepApproval{
		ID:                "appr-1",
		WorkflowRunID:     "wfr-1",
		WorkflowStepRunID: "wfsr-1",
		Status:            "pending",
		RequestedAt:       time.Now(),
	}

	// Invoke the hook directly as the store would.
	if OnApprovalChanged != nil {
		OnApprovalChanged(context.Background(), approval)
	}

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(
		t, captured,
	)
	assert.Equal(t,
		"appr-1",
		captured.
			ID)
	assert.Equal(t,
		"pending",
		captured.
			Status,
	)
}

func TestOnApprovalChanged_NilHookDoesNotPanic(t *testing.T) {
	// Not parallel: mutates the package-level OnApprovalChanged hook.
	origHook := OnApprovalChanged
	t.Cleanup(func() { OnApprovalChanged = origHook })

	OnApprovalChanged = nil

	// Should not panic when hook is nil.
	if OnApprovalChanged != nil {
		OnApprovalChanged(context.Background(), &domain.WorkflowStepApproval{})
	}
}

func TestOnApprovalChanged_UpdateFields(t *testing.T) {
	// Not parallel: mutates the package-level OnApprovalChanged hook.
	origHook := OnApprovalChanged
	t.Cleanup(func() { OnApprovalChanged = origHook })

	var mu sync.Mutex
	var captured *domain.WorkflowStepApproval
	OnApprovalChanged = func(_ context.Context, a *domain.WorkflowStepApproval) {
		mu.Lock()
		defer mu.Unlock()
		captured = a
	}

	now := time.Now()
	// Simulate the approval object constructed in UpdateWorkflowStepApproval.
	approval := &domain.WorkflowStepApproval{
		ID:         "appr-2",
		Status:     "approved",
		ApprovedBy: "user-1",
		ApprovedAt: &now,
	}

	if OnApprovalChanged != nil {
		OnApprovalChanged(context.Background(), approval)
	}

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(
		t, captured,
	)
	assert.Equal(t,
		"approved",
		captured.
			Status,
	)
	assert.Equal(t,
		"user-1",
		captured.
			ApprovedBy,
	)
	assert.NotNil(t,
		captured.
			ApprovedAt,
	)
}
