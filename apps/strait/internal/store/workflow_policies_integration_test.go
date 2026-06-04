//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"
)

func TestUpsertWorkflowPolicy(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	policy := &domain.WorkflowPolicy{
		ProjectID:                "project-workflow-policy",
		MaxFanOut:                10,
		MaxDepth:                 5,
		ForbiddenStepTypes:       []string{"dangerous"},
		RequireApprovalForDeploy: true,
	}
	if err := q.UpsertWorkflowPolicy(ctx, policy); err != nil {
		t.Fatalf("UpsertWorkflowPolicy() error = %v", err)
	}
	if policy.ID == "" {
		t.Fatal("UpsertWorkflowPolicy() did not set ID")
	}
	if policy.CreatedAt.IsZero() {
		t.Fatal("UpsertWorkflowPolicy() did not set CreatedAt")
	}

	got, err := q.GetWorkflowPolicyByProject(ctx, "project-workflow-policy")
	if err != nil {
		t.Fatalf("GetWorkflowPolicyByProject() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetWorkflowPolicyByProject() returned nil")
	}
	if got.MaxFanOut != 10 {
		t.Fatalf("MaxFanOut = %d, want 10", got.MaxFanOut)
	}
	if got.MaxDepth != 5 {
		t.Fatalf("MaxDepth = %d, want 5", got.MaxDepth)
	}
	if !got.RequireApprovalForDeploy {
		t.Fatal("RequireApprovalForDeploy = false, want true")
	}
	if len(got.ForbiddenStepTypes) != 1 || got.ForbiddenStepTypes[0] != "dangerous" {
		t.Fatalf("ForbiddenStepTypes = %v, want [dangerous]", got.ForbiddenStepTypes)
	}
}

func TestUpsertWorkflowPolicy_Update(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	policy := &domain.WorkflowPolicy{
		ProjectID: "project-workflow-policy-update",
		MaxFanOut: 5,
		MaxDepth:  3,
	}
	if err := q.UpsertWorkflowPolicy(ctx, policy); err != nil {
		t.Fatalf("UpsertWorkflowPolicy() error = %v", err)
	}
	initialID := policy.ID
	initialUpdatedAt := policy.UpdatedAt
	var xminBeforeNoop string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM workflow_policies
		WHERE project_id = $1`,
		"project-workflow-policy-update",
	).Scan(&xminBeforeNoop); err != nil {
		t.Fatalf("query workflow_policies xmin before no-op: %v", err)
	}

	same := &domain.WorkflowPolicy{
		ProjectID: "project-workflow-policy-update",
		MaxFanOut: 5,
		MaxDepth:  3,
	}
	if err := q.UpsertWorkflowPolicy(ctx, same); err != nil {
		t.Fatalf("UpsertWorkflowPolicy(no-op) error = %v", err)
	}
	var xminAfterNoop string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM workflow_policies
		WHERE project_id = $1`,
		"project-workflow-policy-update",
	).Scan(&xminAfterNoop); err != nil {
		t.Fatalf("query workflow_policies xmin after no-op: %v", err)
	}
	if xminAfterNoop != xminBeforeNoop {
		t.Fatalf("workflow_policies no-op changed xmin from %s to %s", xminBeforeNoop, xminAfterNoop)
	}
	if same.ID != initialID {
		t.Fatalf("workflow_policies no-op id = %q, want %q", same.ID, initialID)
	}
	if !same.UpdatedAt.Equal(initialUpdatedAt) {
		t.Fatalf("workflow_policies no-op updated_at = %v, want %v", same.UpdatedAt, initialUpdatedAt)
	}

	// Update.
	updated := &domain.WorkflowPolicy{
		ProjectID: "project-workflow-policy-update",
		MaxFanOut: 20,
		MaxDepth:  10,
	}
	if err := q.UpsertWorkflowPolicy(ctx, updated); err != nil {
		t.Fatalf("UpsertWorkflowPolicy(update) error = %v", err)
	}

	got, err := q.GetWorkflowPolicyByProject(ctx, "project-workflow-policy-update")
	if err != nil {
		t.Fatalf("GetWorkflowPolicyByProject() error = %v", err)
	}
	if got.MaxFanOut != 20 {
		t.Fatalf("MaxFanOut = %d, want 20", got.MaxFanOut)
	}
	if got.MaxDepth != 10 {
		t.Fatalf("MaxDepth = %d, want 10", got.MaxDepth)
	}
}

func TestGetWorkflowPolicyByProject_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.GetWorkflowPolicyByProject(ctx, "nonexistent-project")
	if err != nil {
		t.Fatalf("GetWorkflowPolicyByProject() error = %v", err)
	}
	if got != nil {
		t.Fatalf("GetWorkflowPolicyByProject() = %+v, want nil", got)
	}
}
