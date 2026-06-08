package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var activityStreamChannelsSink [3]string

func TestActivityStreamChannels(t *testing.T) {
	t.Parallel()

	require.Equal(t, "cdc:project:proj-1:job_runs", activityStreamJobRunsChannel("proj-1"))
	require.Equal(t, "cdc:project:proj-1:workflow_runs", activityStreamWorkflowRunsChannel("proj-1"))
	require.Equal(t, "cdc:project:proj-1:event_triggers", activityStreamEventTriggersChannel("proj-1"))
	require.Equal(t, [3]string{
		"cdc:project:proj-1:job_runs",
		"cdc:project:proj-1:workflow_runs",
		"cdc:project:proj-1:event_triggers",
	}, activityStreamChannels("proj-1"))
}

func BenchmarkActivityStreamChannels(b *testing.B) {
	projectID := "proj_0123456789abcdef"

	b.ReportAllocs()
	for b.Loop() {
		activityStreamChannelsSink = activityStreamChannels(projectID)
	}
}
