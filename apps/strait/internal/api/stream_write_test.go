package api

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteSSEFrames(t *testing.T) {
	t.Parallel()

	var data bytes.Buffer
	require.NoError(t, writeSSEDataFrame(&data, sseDataFramePrefix(""), []byte(`{"status":"completed"}`)))
	require.Equal(t, "data: {\"status\":\"completed\"}\n\n", data.String())

	var named bytes.Buffer
	require.NoError(t, writeSSEDataFrame(&named, sseDataFramePrefix("log"), []byte(`{"level":"info"}`)))
	require.Equal(t, "event: log\ndata: {\"level\":\"info\"}\n\n", named.String())

	var keepalive bytes.Buffer
	require.NoError(t, writeSSEKeepaliveFrame(&keepalive))
	require.Equal(t, ": keepalive\n\n", keepalive.String())
}

func BenchmarkWriteSSEDataFrame(b *testing.B) {
	msg := []byte(`{"status":"completed","run_id":"run-1"}`)
	defaultPrefix := sseDataFramePrefix("")
	namedPrefix := sseDataFramePrefix("log")
	var buf bytes.Buffer

	b.Run("default", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			buf.Reset()
			if err := writeSSEDataFrame(&buf, defaultPrefix, msg); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("named_event", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			buf.Reset()
			if err := writeSSEDataFrame(&buf, namedPrefix, msg); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkWriteSSEKeepaliveFrame(b *testing.B) {
	var buf bytes.Buffer

	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		if err := writeSSEKeepaliveFrame(&buf); err != nil {
			b.Fatal(err)
		}
	}
}
