package cdc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMarshalTerminalRunPayloadMatchesJSONSemantics(t *testing.T) {
	t.Parallel()

	timestamp := time.Date(2026, 6, 7, 18, 30, 45, 123_456_789, time.UTC)
	payload := marshalTerminalRunPayload(
		"run.completed",
		"run-\"\\\n1",
		"job-1",
		"project-1",
		"completed",
		3,
		"error \"quoted\"",
		timestamp,
	)

	var got struct {
		EventType string    `json:"event_type"`
		RunID     string    `json:"run_id"`
		JobID     string    `json:"job_id"`
		ProjectID string    `json:"project_id"`
		Status    string    `json:"status"`
		Attempt   int       `json:"attempt"`
		Error     string    `json:"error"`
		Timestamp time.Time `json:"timestamp"`
	}
	require.NoError(t, json.Unmarshal(payload, &got))
	require.Equal(t, "run.completed", got.EventType)
	require.Equal(t, "run-\"\\\n1", got.RunID)
	require.Equal(t, "job-1", got.JobID)
	require.Equal(t, "project-1", got.ProjectID)
	require.Equal(t, "completed", got.Status)
	require.Equal(t, 3, got.Attempt)
	require.Equal(t, "error \"quoted\"", got.Error)
	require.Equal(t, timestamp, got.Timestamp)
}
