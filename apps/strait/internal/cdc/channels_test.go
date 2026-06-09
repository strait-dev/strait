package cdc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var cdcChannelSink string

func TestCDCChannels(t *testing.T) {
	t.Parallel()

	require.Equal(t, "cdc:project:proj-1:job_runs", cdcProjectJobRunsChannel("proj-1"))
	require.Equal(t, "cdc:project:proj-1:workflow_runs", cdcProjectWorkflowRunsChannel("proj-1"))
	require.Equal(t, "cdc:workflow_run:wfr-1:steps", cdcWorkflowRunStepsChannel("wfr-1"))
	require.Equal(t, "cdc:project:proj-1:event_triggers", cdcProjectEventTriggersChannel("proj-1"))
	require.Equal(t, "cdc:job_runs:run-1:run.completed:sub-1", webhookTriggerDedupeKey("run-1", "run.completed", "sub-1"))
	require.Equal(t, "cdc:job_runs:run-1:run.failed:channel-1", notificationTriggerDedupeKey("run-1", "run.failed", "channel-1"))
}

func BenchmarkCDCChannels(b *testing.B) {
	projectID := "proj_0123456789abcdef"
	workflowRunID := "workflow_run_0123456789abcdef"
	runID := "run_0123456789abcdef"
	eventType := "run.completed"
	targetID := "target_0123456789abcdef"

	b.Run("ProjectJobRuns", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			cdcChannelSink = cdcProjectJobRunsChannel(projectID)
		}
	})

	b.Run("ProjectWorkflowRuns", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			cdcChannelSink = cdcProjectWorkflowRunsChannel(projectID)
		}
	})

	b.Run("WorkflowRunSteps", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			cdcChannelSink = cdcWorkflowRunStepsChannel(workflowRunID)
		}
	})

	b.Run("ProjectEventTriggers", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			cdcChannelSink = cdcProjectEventTriggersChannel(projectID)
		}
	})

	b.Run("JobRunEventDedupe", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			cdcChannelSink = cdcJobRunEventDedupeKey(runID, eventType, targetID)
		}
	})
}
