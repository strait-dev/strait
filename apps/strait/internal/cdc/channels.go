package cdc

func cdcProjectJobRunsChannel(projectID string) string {
	return "cdc:project:" + projectID + ":job_runs"
}

func cdcProjectWorkflowRunsChannel(projectID string) string {
	return "cdc:project:" + projectID + ":workflow_runs"
}

func cdcWorkflowRunStepsChannel(workflowRunID string) string {
	return "cdc:workflow_run:" + workflowRunID + ":steps"
}

func cdcProjectEventTriggersChannel(projectID string) string {
	return "cdc:project:" + projectID + ":event_triggers"
}

func cdcJobRunEventDedupeKey(runID, eventType, targetID string) string {
	return "cdc:job_runs:" + runID + ":" + eventType + ":" + targetID
}
