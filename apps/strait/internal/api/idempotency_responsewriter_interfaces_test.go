package api

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCaptureWriterImplementsFlusher verifies the type assertion that
// SSE handlers rely on. httptest.ResponseRecorder is itself a Flusher,
// so a captureWriter wrapping one must also satisfy http.Flusher.
func TestCaptureWriterImplementsFlusher(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	cw := &captureWriter{ResponseWriter: rec}

	f, ok := any(cw).(http.Flusher)
	require.True(t, ok)

	// Must not panic when the underlying writer is a Flusher.
	f.Flush()
}

// TestCaptureWriterFlushIsNoOpWhenUnderlyingIsNotFlusher exercises the
// nil-deref guard for ResponseWriters that do not implement Flusher.
func TestCaptureWriterFlushIsNoOpWhenUnderlyingIsNotFlusher(t *testing.T) {
	t.Parallel()

	cw := &captureWriter{ResponseWriter: &nonFlushingWriter{}}
	cw.Flush() // must not panic
}

// TestCaptureWriterImplementsHijacker pins the type assertion side of
// Hijack support. A captureWriter must always satisfy http.Hijacker,
// even when it returns ErrNotSupported at call time.
func TestCaptureWriterImplementsHijacker(t *testing.T) {
	t.Parallel()

	cw := &captureWriter{ResponseWriter: &nonFlushingWriter{}}
	if _, ok := any(cw).(http.Hijacker); !ok {
		require.Fail(t,

			"captureWriter does not implement http.Hijacker")
	}
}

// TestCaptureWriterHijackForwardsToUnderlying confirms Hijack delegates
// to the wrapped ResponseWriter when supported.
func TestCaptureWriterHijackForwardsToUnderlying(t *testing.T) {
	t.Parallel()

	mock := &hijackingWriter{}
	cw := &captureWriter{ResponseWriter: mock}

	hj, ok := any(cw).(http.Hijacker)
	require.True(t, ok)

	if _, _, err := hj.Hijack(); err != nil {
		require.Failf(t, "test failure",

			"Hijack returned error: %v", err)
	}
	require.True(t, mock.
		hijacked,
	)

}

// TestCaptureWriterHijackReportsUnsupported pins the error sentinel:
// callers using errors.Is(err, http.ErrNotSupported) — the stdlib
// convention also followed by Push — must be able to detect the
// missing-capability case. A bespoke errors.New string would silently
// fail those checks.
func TestCaptureWriterHijackReportsUnsupported(t *testing.T) {
	t.Parallel()

	cw := &captureWriter{ResponseWriter: &nonFlushingWriter{}}
	hj, ok := any(cw).(http.Hijacker)
	require.True(t, ok)

	_, _, err := hj.Hijack()
	require.True(t, errors.Is(err,
		http.ErrNotSupported,
	))

}

// TestCaptureWriterImplementsPusher pins the type assertion for
// HTTP/2 server push.
func TestCaptureWriterImplementsPusher(t *testing.T) {
	t.Parallel()

	cw := &captureWriter{ResponseWriter: &nonFlushingWriter{}}
	if _, ok := any(cw).(http.Pusher); !ok {
		require.Fail(t,

			"captureWriter does not implement http.Pusher")
	}
}

// TestCaptureWriterPushReportsUnsupported confirms the standard fallback
// signal is returned when the wrapped writer is not a Pusher.
func TestCaptureWriterPushReportsUnsupported(t *testing.T) {
	t.Parallel()

	cw := &captureWriter{ResponseWriter: &nonFlushingWriter{}}
	p, _ := any(cw).(http.Pusher)
	if err := p.Push("/foo", nil); !errors.Is(err, http.ErrNotSupported) {
		require.Failf(t, "test failure",

			"Push() = %v, want http.ErrNotSupported", err)
	}
}

// TestCaptureWriterFlushDoesNotCorruptCapturedBody asserts that calling
// Flush mid-stream does not duplicate or drop bytes from the in-memory
// capture buffer used for replay.
func TestCaptureWriterFlushDoesNotCorruptCapturedBody(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	cw := &captureWriter{ResponseWriter: rec}

	_, err := cw.Write([]byte("hello "))
	require.NoError(t, err)
	cw.Flush()
	_, err = cw.Write([]byte("world"))
	require.NoError(t, err)
	require.Equal(t, "hello world",

		cw.body.
			String())
	require.Equal(t, "hello world",

		rec.Body.
			String())

}

// nonFlushingWriter is a minimal ResponseWriter that does not implement
// Flusher, Hijacker, or Pusher.
type nonFlushingWriter struct {
	header http.Header
	status int
	body   []byte
}

func (n *nonFlushingWriter) Header() http.Header {
	if n.header == nil {
		n.header = http.Header{}
	}
	return n.header
}

func (n *nonFlushingWriter) Write(b []byte) (int, error) {
	n.body = append(n.body, b...)
	return len(b), nil
}

func (n *nonFlushingWriter) WriteHeader(code int) { n.status = code }

// hijackingWriter implements both ResponseWriter and Hijacker.
type hijackingWriter struct {
	nonFlushingWriter
	hijacked bool
}

func (h *hijackingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return nil, nil, nil
}
