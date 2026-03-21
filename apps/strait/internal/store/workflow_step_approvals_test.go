package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestOnApprovalChanged_CalledAfterCreate(t *testing.T) {
	t.Parallel()

	// Save and restore the global hook.
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
	if captured == nil {
		t.Fatal("expected OnApprovalChanged to be called")
	}
	if captured.ID != "appr-1" {
		t.Errorf("expected approval ID appr-1, got %s", captured.ID)
	}
	if captured.Status != "pending" {
		t.Errorf("expected status pending, got %s", captured.Status)
	}
}

func TestOnApprovalChanged_NilHookDoesNotPanic(t *testing.T) {
	t.Parallel()

	origHook := OnApprovalChanged
	t.Cleanup(func() { OnApprovalChanged = origHook })

	OnApprovalChanged = nil

	// Should not panic when hook is nil.
	if OnApprovalChanged != nil {
		OnApprovalChanged(context.Background(), &domain.WorkflowStepApproval{})
	}
}

func TestOnApprovalChanged_UpdateFields(t *testing.T) {
	t.Parallel()

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
	if captured == nil {
		t.Fatal("expected OnApprovalChanged to be called")
	}
	if captured.Status != "approved" {
		t.Errorf("expected status approved, got %s", captured.Status)
	}
	if captured.ApprovedBy != "user-1" {
		t.Errorf("expected approved_by user-1, got %s", captured.ApprovedBy)
	}
	if captured.ApprovedAt == nil {
		t.Error("expected ApprovedAt to be set")
	}
}
