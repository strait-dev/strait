package api

import (
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestWorkflowRunHookPayloadAndChannels(t *testing.T) {
	t.Parallel()

	timestamp := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	run := &domain.WorkflowRun{
		ID:         "wr-1",
		WorkflowID: "wf-1",
		ProjectID:  "proj-1",
	}

	payload, err := marshalWorkflowRunHookPayload(run, domain.WfStatusRunning, domain.WfStatusCompleted, "done", timestamp)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(payload, &got))
	require.Equal(t, "workflow_status_change", got["type"])
	require.Equal(t, "wr-1", got["workflow_run_id"])
	require.Equal(t, "wf-1", got["workflow_id"])
	require.Equal(t, "proj-1", got["project_id"])
	require.Equal(t, string(domain.WfStatusRunning), got["from"])
	require.Equal(t, string(domain.WfStatusCompleted), got["to"])
	require.Equal(t, "done", got["reason"])
	require.Equal(t, timestamp.Format(time.RFC3339), got["timestamp"])
	require.Equal(t, "workflow-run:wr-1", workflowRunChannel(run.ID))
	require.Equal(t, "workflow:wf-1:runs", workflowRunsChannel(run.WorkflowID))
}

func TestWorkflowRunHookPayloadEscapesStrings(t *testing.T) {
	t.Parallel()

	timestamp := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	run := &domain.WorkflowRun{
		ID:         `wr-"1"`,
		WorkflowID: "wf-1",
		ProjectID:  "proj-1",
	}

	payload, err := marshalWorkflowRunHookPayload(run, domain.WfStatusRunning, domain.WfStatusFailed, "failed\nreason", timestamp)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(payload, &got))
	require.Equal(t, `wr-"1"`, got["workflow_run_id"])
	require.Equal(t, "failed\nreason", got["reason"])
}

func BenchmarkWorkflowRunHookPayloadAndChannels(b *testing.B) {
	timestamp := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	run := &domain.WorkflowRun{
		ID:         "wr-1",
		WorkflowID: "wf-1",
		ProjectID:  "proj-1",
	}

	b.Run("payload", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			payload, err := marshalWorkflowRunHookPayload(run, domain.WfStatusRunning, domain.WfStatusCompleted, "done", timestamp)
			if err != nil {
				b.Fatal(err)
			}
			if len(payload) == 0 {
				b.Fatal("marshalWorkflowRunHookPayload() returned empty payload")
			}
		}
	})

	b.Run("run_channel", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			channel := workflowRunChannel(run.ID)
			if channel == "" {
				b.Fatal("workflowRunChannel() returned empty channel")
			}
		}
	})

	b.Run("workflow_channel", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			channel := workflowRunsChannel(run.WorkflowID)
			if channel == "" {
				b.Fatal("workflowRunsChannel() returned empty channel")
			}
		}
	})
}
