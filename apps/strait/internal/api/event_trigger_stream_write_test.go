package api

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteEventTriggerStatusFrame(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	require.NoError(t, writeEventTriggerStatusFrame(&buf, []byte(`{"status":"received"}`)))
	require.Equal(t, "event: status\ndata: {\"status\":\"received\"}\n\n", buf.String())
}

func TestWriteEventTriggerKeepaliveFrame(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	require.NoError(t, writeEventTriggerKeepaliveFrame(&buf))
	require.Equal(t, ": keepalive\n\n", buf.String())
}

func BenchmarkWriteEventTriggerStatusFrame(b *testing.B) {
	msg := []byte(`{"id":"evt-1","project_id":"proj-1","status":"received"}`)
	var buf bytes.Buffer

	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		if err := writeEventTriggerStatusFrame(&buf, msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriteEventTriggerKeepaliveFrame(b *testing.B) {
	var buf bytes.Buffer

	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		if err := writeEventTriggerKeepaliveFrame(&buf); err != nil {
			b.Fatal(err)
		}
	}
}
