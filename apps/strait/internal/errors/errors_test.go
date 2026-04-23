package errors

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/sourcegraph/conc"
)

var errSentinel = fmt.Errorf("sentinel")

type myErr struct{ Code int }

func (e *myErr) Error() string { return "my error" }

func TestWrap_PreservesErrorsIs(t *testing.T) {
	t.Parallel()
	wrapped := Wrap(errSentinel, "context")
	if !errors.Is(wrapped, errSentinel) {
		t.Fatal("errors.Is should find sentinel in wrapped error")
	}
}

func TestWrap_PreservesErrorsAs(t *testing.T) {
	t.Parallel()
	original := &myErr{Code: 42}
	wrapped := Wrap(original, "context")
	var target *myErr
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should find typed error")
	}
	if target.Code != 42 {
		t.Fatalf("Code = %d, want 42", target.Code)
	}
}

func TestWrap_AddsMessage(t *testing.T) {
	t.Parallel()
	wrapped := Wrap(errSentinel, "operation failed")
	msg := wrapped.Error()
	if !strings.Contains(msg, "operation failed") {
		t.Fatalf("expected message to contain 'operation failed', got: %s", msg)
	}
}

func TestWrap_NilError(t *testing.T) {
	t.Parallel()
	if Wrap(nil, "msg") != nil {
		t.Fatal("Wrap(nil) should return nil")
	}
}

func TestWrapf_FormatsMessage(t *testing.T) {
	t.Parallel()
	wrapped := Wrapf(errSentinel, "failed %s %d", "op", 42)
	msg := wrapped.Error()
	if !strings.Contains(msg, "failed op 42") {
		t.Fatalf("expected formatted message, got: %s", msg)
	}
}

func TestIn_SetsComponent(t *testing.T) {
	t.Parallel()
	err := In("worker").Wrap(errSentinel)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestIn_WithAttributes(t *testing.T) {
	t.Parallel()
	err := In("worker").With("run_id", "r-1").Wrap(errSentinel)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, errSentinel) {
		t.Fatal("errors.Is should find sentinel")
	}
}

func TestIn_ChainedAttributes(t *testing.T) {
	t.Parallel()
	err := In("scheduler").With("job_id", "j-1").With("attempt", 3).Wrap(errSentinel)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestNew_ErrorString(t *testing.T) {
	t.Parallel()
	err := New("something broke")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "something broke") {
		t.Fatalf("expected message, got: %s", err.Error())
	}
}

func TestWrap_DeepNesting(t *testing.T) {
	t.Parallel()
	base := errSentinel
	err := Wrap(Wrap(Wrap(base, "layer1"), "layer2"), "layer3")
	if !errors.Is(err, errSentinel) {
		t.Fatal("deeply nested Wrap should preserve errors.Is")
	}
}

func TestWrapf_NilError(t *testing.T) {
	t.Parallel()
	if Wrapf(nil, "format %s %d", "arg", 1) != nil {
		t.Fatal("Wrapf(nil) should return nil")
	}
}

func TestIn_ConcurrentUsage(t *testing.T) {
	t.Parallel()
	var wg conc.WaitGroup
	for range 50 {
		wg.Go(func() {
			_ = In("worker").With("id", "x").Wrap(errSentinel)
		})
	}
	wg.Wait()
}
