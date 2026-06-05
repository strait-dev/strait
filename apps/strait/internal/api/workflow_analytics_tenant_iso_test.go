package api

import (
	"context"
	"net/http"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestTenantIso_WorkflowAnalytics_StepDurations_RejectsCrossProject ensures
// that even though the underlying ClickHouse query already filters by
// project_id, the handler rejects with 404 when the workflow belongs to
// a different project rather than returning empty results (which would
// leak existence).
func TestTenantIso_WorkflowAnalytics_StepDurations_RejectsCrossProject(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-bbb"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-aaa")
	_, err := srv.handleWorkflowStepDurations(ctx, &WorkflowStepDurationsInput{
		WorkflowID: "wf-foreign",
		From:       "2024-01-01T00:00:00Z",
		To:         "2024-01-02T00:00:00Z",
	})
	require.True(
		t, isHumaStatusError(err,

			http.StatusNotFound))
}
