package workflow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestOnJobRunTerminal_CompensationCompletionMarksWorkflowCompensated(t *testing.T) {
	t.Parallel()

	var terminalStatus string
	var workflowTo domain.WorkflowRunStatus
	store := &mockCallbackStore{
		markCompensationRunTerminalFn: func(_ context.Context, jobRunID string, status string, output json.RawMessage, errMsg string, finishedAt time.Time) (*domain.CompensationRun, error) {
			require.Equal(t, "jr-comp", jobRunID)
			require.Equal(t, domain.CompensationCompleted, status)
			require.Equal(t, `{"refunded":true}`, string(output))
			require.Empty(t, errMsg)

			terminalStatus = status
			return &domain.CompensationRun{ID: "cr-1", WorkflowRunID: "wfr-1", Status: status}, nil
		},
		countIncompleteCompensationRunsFn: func(_ context.Context, workflowRunID string) (int, error) {
			require.Equal(t, "wfr-1", workflowRunID)

			return 0, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
			require.Equal(t, "wfr-1", id)
			require.Equal(t, domain.WfStatusCompensating, from)
			require.Equal(t, domain.WfStatusCompensated, to)
			require.NotNil(t, fields["finished_at"])

			workflowTo = to
			return nil
		},
	}
	cb := NewStepCallback(store, nil, nil)

	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:     "jr-comp",
		Status: domain.StatusCompleted,
		Result: json.RawMessage(`{"refunded":true}`),
		Metadata: map[string]string{
			domain.RunMetadataCompensationRunID:         "cr-1",
			domain.RunMetadataCompensationWorkflowRunID: "wfr-1",
			domain.RunMetadataCompensationStepRef:       "charge-card",
		},
	})
	require.NoError(t, err)
	require.Equal(t, domain.CompensationCompleted, terminalStatus)
	require.Equal(t, domain.WfStatusCompensated, workflowTo)
}

func TestOnJobRunTerminal_CompensationFailureMarksWorkflowFailed(t *testing.T) {
	t.Parallel()

	var workflowTo domain.WorkflowRunStatus
	store := &mockCallbackStore{
		markCompensationRunTerminalFn: func(_ context.Context, jobRunID string, status string, output json.RawMessage, errMsg string, finishedAt time.Time) (*domain.CompensationRun, error) {
			require.Equal(t, domain.CompensationFailed, status)
			require.Equal(t, "refund failed", errMsg)

			return &domain.CompensationRun{ID: "cr-1", WorkflowRunID: "wfr-1", Status: status}, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
			require.Equal(t, "wfr-1", id)
			require.Equal(t, domain.WfStatusCompensating, from)
			require.Equal(t, domain.WfStatusCompensationFailed, to)
			require.Equal(t, "refund failed", fields["error"])
			require.NotNil(t, fields["finished_at"])

			workflowTo = to
			return nil
		},
	}
	cb := NewStepCallback(store, nil, nil)

	err := cb.OnJobRunTerminal(context.Background(), &domain.JobRun{
		ID:     "jr-comp",
		Status: domain.StatusFailed,
		Error:  "refund failed",
		Metadata: map[string]string{
			domain.RunMetadataCompensationRunID:         "cr-1",
			domain.RunMetadataCompensationWorkflowRunID: "wfr-1",
			domain.RunMetadataCompensationStepRef:       "charge-card",
		},
	})
	require.NoError(t, err)
	require.Equal(t, domain.WfStatusCompensationFailed, workflowTo)
}

func TestIsCompensationJobRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  *domain.JobRun
		want bool
	}{
		{
			name: "nil run",
		},
		{
			name: "nil metadata",
			run:  &domain.JobRun{},
		},
		{
			name: "missing compensation id",
			run:  &domain.JobRun{Metadata: map[string]string{}},
		},
		{
			name: "compensation id present",
			run: &domain.JobRun{
				Metadata: map[string]string{
					domain.RunMetadataCompensationRunID: "cr-1",
				},
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, isCompensationJobRun(tc.run))
		})
	}
}
