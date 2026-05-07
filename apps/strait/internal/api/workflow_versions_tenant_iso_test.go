package api

import (
	"context"
	"net/http"
	"testing"

	"strait/internal/domain"
)

// TestTenantIso_WorkflowVersions_List_RejectsCrossProject ensures listing
// versions of a workflow in another project returns 404 instead of leaking
// the version history.
func TestTenantIso_WorkflowVersions_List_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-bbb"}, nil
		},
		ListWorkflowVersionsFunc: func(_ context.Context, _ string, _ int) ([]domain.WorkflowVersion, error) {
			t.Fatal("ListWorkflowVersions must not be called for cross-project access")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleListWorkflowVersions(ctx, &ListWorkflowVersionsInput{WorkflowID: "wf-foreign"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404, got %v", err)
	}
}

// TestTenantIso_WorkflowVersions_Diff_RejectsCrossProject ensures the diff
// endpoint cannot be used to compare versions of a workflow that belongs
// to another project.
func TestTenantIso_WorkflowVersions_Diff_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-bbb"}, nil
		},
		GetWorkflowVersionByVersionIDFunc: func(_ context.Context, _ string, _ string) (*domain.WorkflowVersion, error) {
			t.Fatal("GetWorkflowVersionByVersionID must not be called for cross-project access")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleWorkflowVersionDiff(ctx, &WorkflowVersionDiffInput{
		WorkflowID:    "wf-foreign",
		FromVersionID: "v1",
		ToVersionID:   "v2",
	})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404, got %v", err)
	}
}

// TestTenantIso_WorkflowVersions_Impact_RejectsCrossProject covers the
// impact endpoint which previously could leak run counts pinned to a
// foreign workflow's version.
func TestTenantIso_WorkflowVersions_Impact_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-bbb"}, nil
		},
		GetWorkflowVersionByVersionIDFunc: func(_ context.Context, _ string, _ string) (*domain.WorkflowVersion, error) {
			t.Fatal("GetWorkflowVersionByVersionID must not be called for cross-project access")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleWorkflowVersionImpact(ctx, &WorkflowVersionImpactInput{
		WorkflowID: "wf-foreign",
		VersionID:  "v1",
	})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404, got %v", err)
	}
}
