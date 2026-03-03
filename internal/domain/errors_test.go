package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestTransitionError_Error(t *testing.T) {
	err := &TransitionError{From: StatusQueued, To: StatusCompleted}
	got := err.Error()

	if !strings.Contains(got, "queued") {
		t.Errorf("error should contain 'queued', got %q", got)
	}
	if !strings.Contains(got, "completed") {
		t.Errorf("error should contain 'completed', got %q", got)
	}
	if !strings.Contains(got, "invalid transition") {
		t.Errorf("error should contain 'invalid transition', got %q", got)
	}
}

func TestTransitionError_ImplementsError(t *testing.T) {
	var err error = &TransitionError{From: StatusQueued, To: StatusCompleted}
	if err == nil {
		t.Fatal("TransitionError should implement error interface")
	}
}

func TestUnknownStatusError_Error(t *testing.T) {
	err := &UnknownStatusError{Status: RunStatus("bogus")}
	got := err.Error()

	if !strings.Contains(got, "bogus") {
		t.Errorf("error should contain status, got %q", got)
	}
	if !strings.Contains(got, "unknown status") {
		t.Errorf("error should contain 'unknown status', got %q", got)
	}
}

func TestEndpointError_Error(t *testing.T) {
	err := &EndpointError{StatusCode: 503, Body: "service unavailable"}
	got := err.Error()

	if !strings.Contains(got, "503") {
		t.Errorf("error should contain status code, got %q", got)
	}
	if !strings.Contains(got, "service unavailable") {
		t.Errorf("error should contain body, got %q", got)
	}
}

func TestEndpointError_EmptyBody(t *testing.T) {
	err := &EndpointError{StatusCode: 500, Body: ""}
	got := err.Error()
	if !strings.Contains(got, "500") {
		t.Errorf("error should contain status code, got %q", got)
	}
}

func TestFieldError_Error(t *testing.T) {
	err := &FieldError{Field: "nonexistent_field"}
	got := err.Error()

	if !strings.Contains(got, "nonexistent_field") {
		t.Errorf("error should contain field name, got %q", got)
	}
	if !strings.Contains(got, "unsupported update field") {
		t.Errorf("error should contain 'unsupported update field', got %q", got)
	}
}

func TestConfigError_Error(t *testing.T) {
	err := &ConfigError{Field: "DATABASE_URL", Message: "is required"}
	got := err.Error()

	if !strings.Contains(got, "DATABASE_URL") {
		t.Errorf("error should contain field, got %q", got)
	}
	if !strings.Contains(got, "is required") {
		t.Errorf("error should contain message, got %q", got)
	}
}

func TestErrJobDisabled(t *testing.T) {
	if ErrJobDisabled == nil {
		t.Fatal("ErrJobDisabled should not be nil")
	}
	if ErrJobDisabled.Error() != "job is disabled" {
		t.Errorf("ErrJobDisabled = %q, want %q", ErrJobDisabled.Error(), "job is disabled")
	}
}

func TestErrJobDisabled_IsComparable(t *testing.T) {
	wrapped := errors.New("outer: " + ErrJobDisabled.Error())
	_ = wrapped // just verifying sentinel doesn't panic

	if !errors.Is(ErrJobDisabled, ErrJobDisabled) {
		t.Error("ErrJobDisabled should be comparable with errors.Is")
	}
}

func TestValidateTransition_ReturnsTransitionError(t *testing.T) {
	err := ValidateTransition(StatusQueued, StatusCompleted)
	if err == nil {
		t.Fatal("expected error for invalid transition")
	}

	var te *TransitionError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TransitionError, got %T", err)
	}
	if te.From != StatusQueued {
		t.Errorf("From = %q, want %q", te.From, StatusQueued)
	}
	if te.To != StatusCompleted {
		t.Errorf("To = %q, want %q", te.To, StatusCompleted)
	}
}

func TestValidateTransition_ReturnsUnknownStatusError(t *testing.T) {
	err := ValidateTransition(RunStatus("invalid"), StatusQueued)
	if err == nil {
		t.Fatal("expected error for unknown status")
	}

	var ue *UnknownStatusError
	if !errors.As(err, &ue) {
		t.Fatalf("expected *UnknownStatusError, got %T", err)
	}
	if ue.Status != RunStatus("invalid") {
		t.Errorf("Status = %q, want %q", ue.Status, "invalid")
	}
}

func TestMustTransition_ValidDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MustTransition panicked for valid transition: %v", r)
		}
	}()
	MustTransition(StatusQueued, StatusDequeued)
}

func TestMustTransition_InvalidPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustTransition did not panic for invalid transition")
		}
	}()
	MustTransition(StatusCompleted, StatusExecuting)
}

func TestMustTransition_PanicContainsError(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("panic value is %T, want error", r)
		}
		if !strings.Contains(err.Error(), "completed") {
			t.Errorf("panic error should mention status, got %q", err.Error())
		}
	}()
	MustTransition(StatusCompleted, StatusQueued)
}
