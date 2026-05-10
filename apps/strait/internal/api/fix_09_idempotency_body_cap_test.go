package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/quick"
	"time"
)

// TestFix_09_CaptureWriterAllowsUnderCap regression-tests the happy path:
// a sub-cap response is memoized via CompleteIdempotencyKey as before.
func TestFix_09_CaptureWriterAllowsUnderCap(t *testing.T) {
	t.Parallel()

	var (
		mu             sync.Mutex
		completeCalled bool
		completedBytes int
		deleteCalled   bool
	)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, []byte, error) {
			return "acquired", 0, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ int, body []byte) error {
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
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("response code = %d, want 201", w.Code)
	}
	if w.Body.Len() != len(body) {
		t.Fatalf("response body length = %d, want %d", w.Body.Len(), len(body))
	}
	mu.Lock()
	defer mu.Unlock()
	if !completeCalled {
		t.Fatal("expected CompleteIdempotencyKey under cap")
	}
	if completedBytes != len(body) {
		t.Fatalf("CompleteIdempotencyKey body length = %d, want %d", completedBytes, len(body))
	}
	if deleteCalled {
		t.Fatal("DeleteIdempotencyKey must not run on success path under cap")
	}
}

// TestFix_09_CaptureWriterDropsCacheOnOverflow pins the overflow contract:
// when the response exceeds the cap, the client still receives the full
// bytes, but CompleteIdempotencyKey is skipped (we never persist a
// truncated body) and DeleteIdempotencyKey clears the pending row so
// retries can proceed.
func TestFix_09_CaptureWriterDropsCacheOnOverflow(t *testing.T) {
	t.Parallel()

	var (
		mu             sync.Mutex
		completeCalled bool
		deleteCalled   bool
	)

	ms := &APIStoreMock{
		TryAcquireIdempotencyKeyFunc: func(_ context.Context, _, _ string, _ time.Duration) (string, int, []byte, error) {
			return "acquired", 0, nil, nil
		},
		CompleteIdempotencyKeyFunc: func(context.Context, string, string, int, []byte) error {
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
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("response code = %d, want 201 (caller still succeeds on overflow)", w.Code)
	}
	if w.Body.Len() != len(body) {
		t.Fatalf("response body bytes = %d, want %d (caller must receive full bytes)", w.Body.Len(), len(body))
	}
	mu.Lock()
	defer mu.Unlock()
	if completeCalled {
		t.Fatal("CompleteIdempotencyKey must not run when response overflows the cap")
	}
	if !deleteCalled {
		t.Fatal("DeleteIdempotencyKey must clear the pending row on overflow so retries can proceed")
	}
}

// TestFix_09_CaptureWriterBuffersAtMostCap ensures the in-memory buffer
// does not grow past maxIdempotencyResponseBytes regardless of the
// response size. Without the cap, a malicious or buggy handler could
// pin the entire response in RAM.
func TestFix_09_CaptureWriterBuffersAtMostCap(t *testing.T) {
	t.Parallel()

	cw := &captureWriter{ResponseWriter: httptest.NewRecorder()}
	chunk := make([]byte, 1<<20) // 1 MiB
	totalWritten := 0
	for i := range 32 { // 32 MiB total
		n, err := cw.Write(chunk)
		if err != nil {
			t.Fatalf("Write iteration %d returned err = %v", i, err)
		}
		if n != len(chunk) {
			t.Fatalf("Write iteration %d wrote %d bytes, want %d (caller bytes must always pass through)", i, n, len(chunk))
		}
		totalWritten += n
	}
	if cw.body.Len() > maxIdempotencyResponseBytes {
		t.Fatalf("captureWriter buffered %d bytes, want <= cap %d", cw.body.Len(), maxIdempotencyResponseBytes)
	}
	if !cw.overflow {
		t.Fatalf("captureWriter overflow flag not set after writing %d bytes (cap = %d)", totalWritten, maxIdempotencyResponseBytes)
	}
}

// FuzzFix_09_CaptureWriterBoundedBufferSize exercises the buffer-cap
// invariant with random write sizes.
func FuzzFix_09_CaptureWriterBoundedBufferSize(f *testing.F) {
	f.Add(uint32(0))
	f.Add(uint32(maxIdempotencyResponseBytes - 1))
	f.Add(uint32(maxIdempotencyResponseBytes))
	f.Add(uint32(maxIdempotencyResponseBytes + 1))
	f.Add(uint32(2 * maxIdempotencyResponseBytes))

	f.Fuzz(func(t *testing.T, size uint32) {
		// Cap fuzz inputs at 64 MiB to keep runtime sane.
		if size > 64<<20 {
			size = 64 << 20
		}
		cw := &captureWriter{ResponseWriter: httptest.NewRecorder()}
		chunk := make([]byte, size)
		n, err := cw.Write(chunk)
		if err != nil {
			t.Fatalf("Write returned err = %v (caller bytes must always pass through)", err)
		}
		if n != int(size) {
			t.Fatalf("Write returned n=%d, want %d", n, size)
		}
		if cw.body.Len() > maxIdempotencyResponseBytes {
			t.Fatalf("buffered %d bytes, cap %d", cw.body.Len(), maxIdempotencyResponseBytes)
		}
		if size > uint32(maxIdempotencyResponseBytes) && !cw.overflow {
			t.Fatal("overflow flag should be set when size > cap")
		}
	})
}

// quickCheckPropertyForCap runs a small property check via testing/quick
// as a redundancy on the fuzz target.
func TestFix_09_CaptureWriterPropertyBoundedBuffer(t *testing.T) {
	t.Parallel()

	prop := func(rawSize uint32) bool {
		size := int(rawSize % (4 * uint32(maxIdempotencyResponseBytes)))
		cw := &captureWriter{ResponseWriter: httptest.NewRecorder()}
		_, _ = cw.Write(make([]byte, size))
		return cw.body.Len() <= maxIdempotencyResponseBytes
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}
