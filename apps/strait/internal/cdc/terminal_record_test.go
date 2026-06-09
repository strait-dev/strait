package cdc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTerminalRunRecord(t *testing.T) {
	t.Parallel()

	record, err := parseTerminalRunRecord([]byte(`{"id":"run-1","job_id":"job-1","project_id":"p1","status":"completed","attempt":2,"error":null}`))
	require.NoError(t, err)
	require.Equal(t, terminalRunRecord{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "p1",
		Status:    "completed",
		Attempt:   2,
	}, record)

	_, err = parseTerminalRunRecord([]byte(`{"id":123,"attempt":1}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "id must be a string")

	_, err = parseTerminalRunRecord([]byte(`{"id":"run-1","attempt":1.5}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "attempt must be an integer")
}

func BenchmarkParseTerminalRunRecord(b *testing.B) {
	record := []byte(`{"id":"run-1","job_id":"job-1","project_id":"p1","status":"completed","attempt":2,"error":"boom"}`)

	b.ReportAllocs()
	for b.Loop() {
		parsed, err := parseTerminalRunRecord(record)
		if err != nil {
			b.Fatal(err)
		}
		if parsed.ID == "" || parsed.Attempt != 2 {
			b.Fatal("unexpected parsed record")
		}
	}
}
