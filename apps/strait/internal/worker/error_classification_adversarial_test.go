package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"strait/internal/domain"
)

// TestClassifyError_HTTP399 verifies that a 399 status is not classified as client or server.
func TestClassifyError_HTTP399(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 399, Body: "not found"}
	got := classifyError(err)
	if got == domain.ErrorClassClient || got == domain.ErrorClassServer {
		t.Fatalf("status 399: expected neither client nor server, got %q", got)
	}
}

// TestClassifyError_HTTP400 verifies that status 400 is classified as client.
func TestClassifyError_HTTP400(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 400, Body: "bad request"}
	got := classifyError(err)
	if got != domain.ErrorClassClient {
		t.Fatalf("status 400: expected %q, got %q", domain.ErrorClassClient, got)
	}
}

// TestClassifyError_HTTP401 verifies that status 401 is classified as auth.
func TestClassifyError_HTTP401(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 401, Body: "unauthorized"}
	got := classifyError(err)
	if got != domain.ErrorClassAuth {
		t.Fatalf("status 401: expected %q, got %q", domain.ErrorClassAuth, got)
	}
}

// TestClassifyError_HTTP403 verifies that status 403 is classified as auth.
func TestClassifyError_HTTP403(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 403, Body: "forbidden"}
	got := classifyError(err)
	if got != domain.ErrorClassAuth {
		t.Fatalf("status 403: expected %q, got %q", domain.ErrorClassAuth, got)
	}
}

// TestClassifyError_HTTP429 verifies that status 429 is classified as rate_limited.
func TestClassifyError_HTTP429(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 429, Body: "too many requests"}
	got := classifyError(err)
	if got != domain.ErrorClassRateLimited {
		t.Fatalf("status 429: expected %q, got %q", domain.ErrorClassRateLimited, got)
	}
}

// TestClassifyError_HTTP499 verifies that status 499 is classified as client.
func TestClassifyError_HTTP499(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 499, Body: "client closed"}
	got := classifyError(err)
	if got != domain.ErrorClassClient {
		t.Fatalf("status 499: expected %q, got %q", domain.ErrorClassClient, got)
	}
}

// TestClassifyError_HTTP500 verifies that status 500 is classified as server.
func TestClassifyError_HTTP500(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 500, Body: "internal server error"}
	got := classifyError(err)
	if got != domain.ErrorClassServer {
		t.Fatalf("status 500: expected %q, got %q", domain.ErrorClassServer, got)
	}
}

// TestClassifyError_HTTP600 verifies that status 600 is classified as server.
func TestClassifyError_HTTP600(t *testing.T) {
	t.Parallel()
	err := &domain.EndpointError{StatusCode: 600, Body: "non-standard"}
	got := classifyError(err)
	if got != domain.ErrorClassServer {
		t.Fatalf("status 600: expected %q, got %q", domain.ErrorClassServer, got)
	}
}

// TestClassifyError_ContextDeadline verifies that context.DeadlineExceeded is classified as timeout.
func TestClassifyError_ContextDeadline(t *testing.T) {
	t.Parallel()
	got := classifyError(context.DeadlineExceeded)
	if got != domain.ErrorClassTimeout {
		t.Fatalf("DeadlineExceeded: expected %q, got %q", domain.ErrorClassTimeout, got)
	}
}

// TestClassifyError_ContextCanceledAdversarial verifies that context.Canceled is classified as transient.
func TestClassifyError_ContextCanceledAdversarial(t *testing.T) {
	t.Parallel()
	got := classifyError(context.Canceled)
	if got != domain.ErrorClassTransient {
		t.Fatalf("Canceled: expected %q, got %q", domain.ErrorClassTransient, got)
	}
}

// TestClassifyError_ConnectionRefused verifies that connection refused errors are classified as connection.
func TestClassifyError_ConnectionRefused(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("dial tcp 127.0.0.1:8080: connection refused")
	got := classifyError(err)
	if got != domain.ErrorClassConnection {
		t.Fatalf("connection refused: expected %q, got %q", domain.ErrorClassConnection, got)
	}
}

// TestClassifyError_OOM verifies that out of memory errors are classified as oom.
func TestClassifyError_OOM(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("process killed: out of memory")
	got := classifyError(err)
	if got != domain.ErrorClassOOM {
		t.Fatalf("out of memory: expected %q, got %q", domain.ErrorClassOOM, got)
	}
}

// TestClassifyError_BudgetExceeded verifies that budget exceeded errors are classified as budget.
func TestClassifyError_BudgetExceeded(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("operation failed: budget exceeded for project xyz")
	got := classifyError(err)
	if got != domain.ErrorClassBudget {
		t.Fatalf("budget exceeded: expected %q, got %q", domain.ErrorClassBudget, got)
	}
}

// TestClassifyError_DeeplyWrapped verifies classification through 10 levels of error wrapping.
func TestClassifyError_DeeplyWrapped(t *testing.T) {
	t.Parallel()
	var err error = &domain.EndpointError{StatusCode: 429, Body: "rate limited"}
	for i := range 10 {
		err = fmt.Errorf("wrap level %d: %w", i, err)
	}
	got := classifyError(err)
	if got != domain.ErrorClassRateLimited {
		t.Fatalf("deeply wrapped 429: expected %q, got %q", domain.ErrorClassRateLimited, got)
	}
}

// TestClassifyError_NilError verifies that nil errors return unknown.
func TestClassifyError_NilError(t *testing.T) {
	t.Parallel()
	got := classifyError(nil)
	if got != domain.ErrorClassUnknown {
		t.Fatalf("nil error: expected %q, got %q", domain.ErrorClassUnknown, got)
	}
}

// FuzzClassifyError fuzz-tests classifyError with arbitrary error messages.
func FuzzClassifyError(f *testing.F) {
	f.Add("connection refused")
	f.Add("out of memory")
	f.Add("budget exceeded")
	f.Add("some random error")
	f.Add("")
	f.Add("endpoint returned 500: internal server error")
	f.Fuzz(func(t *testing.T, msg string) {
		err := fmt.Errorf("%s", msg)
		got := classifyError(err)
		if !domain.ValidErrorClasses[got] {
			t.Fatalf("classifyError returned invalid class %q for message %q", got, msg)
		}
	})
}

// TestShouldRetryForClass_AllClasses is a table-driven test covering all error classes.
func TestShouldRetryForClass_AllClasses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		class string
		want  bool
	}{
		{domain.ErrorClassClient, false},
		{domain.ErrorClassAuth, false},
		{domain.ErrorClassBudget, false},
		{domain.ErrorClassOOM, false},
		{domain.ErrorClassServer, true},
		{domain.ErrorClassTransient, true},
		{domain.ErrorClassTimeout, true},
		{domain.ErrorClassRateLimited, true},
		{domain.ErrorClassConnection, true},
		{domain.ErrorClassUnknown, true},
	}
	for _, tc := range cases {
		t.Run(tc.class, func(t *testing.T) {
			t.Parallel()
			got := shouldRetryForClass(tc.class)
			if got != tc.want {
				t.Fatalf("shouldRetryForClass(%q) = %v, want %v", tc.class, got, tc.want)
			}
		})
	}
}

// TestShouldUseFallbackForClass_AllClasses is a table-driven test covering all error classes.
func TestShouldUseFallbackForClass_AllClasses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		class string
		want  bool
	}{
		{domain.ErrorClassTransient, true},
		{domain.ErrorClassRateLimited, true},
		{domain.ErrorClassConnection, true},
		{domain.ErrorClassTimeout, true},
		{domain.ErrorClassClient, false},
		{domain.ErrorClassAuth, false},
		{domain.ErrorClassBudget, false},
		{domain.ErrorClassOOM, false},
		{domain.ErrorClassServer, false},
		{domain.ErrorClassUnknown, false},
	}
	for _, tc := range cases {
		t.Run(tc.class, func(t *testing.T) {
			t.Parallel()
			got := shouldUseFallbackForClass(tc.class)
			if got != tc.want {
				t.Fatalf("shouldUseFallbackForClass(%q) = %v, want %v", tc.class, got, tc.want)
			}
		})
	}
}

// TestErrorHash_UTF8Truncation verifies that multi-byte characters at the 200-char boundary
// are not split, producing a valid and consistent hash.
func TestErrorHash_UTF8Truncation(t *testing.T) {
	t.Parallel()
	// Build a string of exactly 200 multi-byte runes followed by more content.
	// Each rune is 3 bytes in UTF-8, so byte length > 200 but rune count = 200+extra.
	runes := strings.Repeat("\u4e16", 199) + "\u4e16" + "extra content after boundary"
	h1 := errorHash(runes)
	if len(h1) != 16 {
		t.Fatalf("errorHash returned %d chars, want 16", len(h1))
	}
	// Hash should be identical when called again.
	h2 := errorHash(runes)
	if h1 != h2 {
		t.Fatalf("errorHash not consistent: %q != %q", h1, h2)
	}
}

// TestErrorHash_EmptyMessage verifies that an empty string produces a valid hash.
func TestErrorHash_EmptyMessage(t *testing.T) {
	t.Parallel()
	h := errorHash("")
	if len(h) != 16 {
		t.Fatalf("errorHash(\"\") returned %d chars, want 16", len(h))
	}
}

// TestErrorHash_LongMessage verifies that a 10KB message produces a valid 16-char hash.
func TestErrorHash_LongMessage(t *testing.T) {
	t.Parallel()
	msg := strings.Repeat("x", 10240)
	h := errorHash(msg)
	if len(h) != 16 {
		t.Fatalf("errorHash(10KB) returned %d chars, want 16", len(h))
	}
}

// TestErrorHash_Consistency verifies that the same message always produces the same hash.
func TestErrorHash_Consistency(t *testing.T) {
	t.Parallel()
	msg := "some deterministic error message"
	h1 := errorHash(msg)
	h2 := errorHash(msg)
	if h1 != h2 {
		t.Fatalf("errorHash not consistent: %q != %q", h1, h2)
	}
	// Different messages should (almost certainly) produce different hashes.
	h3 := errorHash("a completely different error message")
	if h1 == h3 {
		t.Fatalf("different messages produced same hash: %q", h1)
	}
}

// FuzzErrorHashAdversarial fuzz-tests that errorHash always returns a 16-char hex string
// and is consistent for the same input.
func FuzzErrorHashAdversarial(f *testing.F) {
	f.Add("hello world")
	f.Add("")
	f.Add(strings.Repeat("a", 300))
	f.Add("\x00\xff\xfe")
	f.Fuzz(func(t *testing.T, msg string) {
		h1 := errorHash(msg)
		if len(h1) != 16 {
			t.Fatalf("errorHash returned %d chars, want 16", len(h1))
		}
		h2 := errorHash(msg)
		if h1 != h2 {
			t.Fatalf("errorHash not consistent for input %q: %q != %q", msg, h1, h2)
		}
	})
}
