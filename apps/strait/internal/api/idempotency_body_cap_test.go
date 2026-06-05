package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/quick"
	"time"

	"github.com/stretchr/testify/require"
)

// TestCaptureWriterAllowsUnderCap regression-tests the happy path:
// a sub-cap response is memoized via CompleteIdempotencyKey as before.
func TestCaptureWriterAllowsUnderCap(t *testing.T) {
	t.Parallel()

	var (
		mu             sync.Mutex
		completeCalled bool
		completedBytes int
		deleteCalled   bool
	)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, http.Header, []byte, error) {
			return "acquired", 0, nil, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ int, _ http.Header, body []byte) error {
			mu.Lock()
			defer mu.Unlock()
			completeCalled = true
			completedBytes = len(body)
			return nil
		},
		DeleteIdempotencyKeyFunc: func(context.Context, string, string) (int64, error) {
			mu.Lock()
			defer mu.Unlock()
			deleteCalled = true
			return 1, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := make([]byte, maxIdempotencyResponseBytes-1)
	for i := range body {
		body[i] = 'a'
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "under-cap")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code)
	require.Equal(t, len(body), w.Body.Len())

	mu.Lock()
	defer mu.Unlock()
	require.True(
		t, completeCalled,
	)
	require.Equal(t, len(body), completedBytes)
	require.False(t, deleteCalled)

}

// TestCaptureWriterDropsCacheOnOverflow pins the overflow contract:
// when the response exceeds the cap, the client still receives the full
// bytes, but CompleteIdempotencyKey is skipped (we never persist a
// truncated body) and DeleteIdempotencyKey clears the pending row so
// retries can proceed.
func TestCaptureWriterDropsCacheOnOverflow(t *testing.T) {
	t.Parallel()

	var (
		mu             sync.Mutex
		completeCalled bool
		deleteCalled   bool
	)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, http.Header, []byte, error) {
			return "acquired", 0, nil, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(context.Context, string, string, int, http.Header, []byte) error {
			mu.Lock()
			defer mu.Unlock()
			completeCalled = true
			return nil
		},
		DeleteIdempotencyKeyFunc: func(context.Context, string, string) (int64, error) {
			mu.Lock()
			defer mu.Unlock()
			deleteCalled = true
			return 1, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := make([]byte, maxIdempotencyResponseBytes+1024)
	for i := range body {
		body[i] = 'b'
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	})
	wrapped := srv.idempotencyMiddleware(handler)

	r := httptest.NewRequest(http.MethodPost, "/v1/jobs", nil)
	r.Header.Set("Idempotency-Key", "overflow-key")
	r = r.WithContext(idempotencyTestCtx(r.Context(), "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code)
	require.Equal(t, len(body), w.Body.Len())

	mu.Lock()
	defer mu.Unlock()
	require.False(t, completeCalled)
	require.True(
		t, deleteCalled,
	)

}

// TestCaptureWriterBuffersAtMostCap ensures the in-memory buffer
// does not grow past maxIdempotencyResponseBytes regardless of the
// response size. Without the cap, a malicious or buggy handler could
// pin the entire response in RAM.
func TestCaptureWriterBuffersAtMostCap(t *testing.T) {
	t.Parallel()

	cw := &captureWriter{ResponseWriter: httptest.NewRecorder()}
	chunk := make([]byte, 1<<20) // 1 MiB
	totalWritten := 0
	// Cap is 16 MiB; write exactly cap+1 chunks so we trigger overflow
	// but never balloon the test process to 32 MiB. Re-using one chunk
	// keeps allocator pressure flat at 1 MiB.
	iterations := (maxIdempotencyResponseBytes / len(chunk)) + 1
	for range iterations {
		n, err := cw.Write(chunk)
		require.NoError(t, err)
		require.Equal(t, len(chunk), n)

		totalWritten += n
	}
	require.LessOrEqual(t, cw.
		body.Len(), maxIdempotencyResponseBytes,
	)
	require.True(
		t, cw.overflow,
	)

}

// FuzzCaptureWriterBoundedBufferSize exercises the buffer-cap
// invariant with random write sizes.
func FuzzCaptureWriterBoundedBufferSize(f *testing.F) {
	f.Add(uint32(0))
	f.Add(uint32(maxIdempotencyResponseBytes - 1))
	f.Add(uint32(maxIdempotencyResponseBytes))
	f.Add(uint32(maxIdempotencyResponseBytes + 1))
	f.Add(uint32(2 * maxIdempotencyResponseBytes))

	f.Fuzz(func(t *testing.T, size uint32) {
		// Cap fuzz inputs at 32 MiB (2x the body cap) so we still
		// exercise the overflow branch but never allocate 64+ MiB per
		// iteration in -fuzz runs.
		if size > 32<<20 {
			size = 32 << 20
		}
		cw := &captureWriter{ResponseWriter: httptest.NewRecorder()}
		chunk := make([]byte, size)
		n, err := cw.Write(chunk)
		require.NoError(t, err)
		require.Equal(t, int(size), n)
		require.LessOrEqual(t, cw.
			body.Len(), maxIdempotencyResponseBytes,
		)
		require.False(t, size > uint32(maxIdempotencyResponseBytes) && !cw.
			overflow)

	})
}

// quickCheckPropertyForCap runs a small property check via testing/quick
// as a redundancy on the fuzz target.
func TestCaptureWriterPropertyBoundedBuffer(t *testing.T) {
	t.Parallel()

	prop := func(rawSize uint32) bool {
		// Bound at 2x cap so a 200-iteration property check tops out
		// around 6 GB of allocations rather than 12 GB. Still exercises
		// both under-cap and over-cap branches.
		size := int(rawSize % (2 * uint32(maxIdempotencyResponseBytes)))
		cw := &captureWriter{ResponseWriter: httptest.NewRecorder()}
		_, _ = cw.Write(make([]byte, size))
		return cw.body.Len() <= maxIdempotencyResponseBytes
	}
	require.NoError(t, quick.
		Check(prop, &quick.Config{MaxCount: 200}))

}
