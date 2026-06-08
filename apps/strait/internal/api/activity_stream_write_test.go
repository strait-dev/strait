package api

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteActivityStreamFrames(t *testing.T) {
	t.Parallel()

	var event bytes.Buffer
	require.NoError(t, writeActivityStreamEvent(&event, []byte(`{"type":"run"}`)))
	require.Equal(t, "event: activity\ndata: {\"type\":\"run\"}\n\n", event.String())

	var keepalive bytes.Buffer
	require.NoError(t, writeActivityStreamKeepalive(&keepalive))
	require.Equal(t, ": keepalive\n\n", keepalive.String())
}

func BenchmarkWriteActivityStreamEvent(b *testing.B) {
	msg := []byte(`{"table":"job_runs","action":"update","record":{"id":"run-1","status":"completed"}}`)
	var buf bytes.Buffer

	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		if err := writeActivityStreamEvent(&buf, msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriteActivityStreamKeepalive(b *testing.B) {
	var buf bytes.Buffer

	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		if err := writeActivityStreamKeepalive(&buf); err != nil {
			b.Fatal(err)
		}
	}
}
