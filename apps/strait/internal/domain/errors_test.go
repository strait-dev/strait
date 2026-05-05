package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestTransitionError_Error(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	_, ok := any(&TransitionError{From: StatusQueued, To: StatusCompleted}).(error)
	if !ok {
		t.Fatal("TransitionError should implement error interface")
	}
}

func TestUnknownStatusError_Error(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	err := &EndpointError{StatusCode: 503, Body: "service unavailable with tenant secret"}
	got := err.Error()

	if !strings.Contains(got, "503") {
		t.Errorf("error should contain status code, got %q", got)
	}
	if strings.Contains(got, "tenant secret") || strings.Contains(got, "service unavailable") {
		t.Errorf("error should not contain endpoint response body, got %q", got)
	}
}

func TestEndpointError_EmptyBody(t *testing.T) {
	t.Parallel()
	err := &EndpointError{StatusCode: 500, Body: ""}
	got := err.Error()
	if !strings.Contains(got, "500") {
		t.Errorf("error should contain status code, got %q", got)
	}
}

func TestFieldError_Error(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	if ErrJobDisabled == nil {
		t.Fatal("ErrJobDisabled should not be nil")
	}
	if ErrJobDisabled.Error() != "job is disabled" {
		t.Errorf("ErrJobDisabled = %q, want %q", ErrJobDisabled.Error(), "job is disabled")
	}
}

func TestErrJobDisabled_IsComparable(t *testing.T) {
	t.Parallel()
	wrapped := errors.New("outer: " + ErrJobDisabled.Error())
	_ = wrapped // just verifying sentinel doesn't panic

	if !errors.Is(ErrJobDisabled, ErrJobDisabled) {
		t.Error("ErrJobDisabled should be comparable with errors.Is")
	}
}

func TestValidateTransition_ReturnsTransitionError(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestTransition_ValidReturnsNil(t *testing.T) {
	t.Parallel()
	if err := Transition(StatusQueued, StatusDequeued); err != nil {
		t.Fatalf("Transition returned error for valid transition: %v", err)
	}
}

func TestTransition_InvalidReturnsError(t *testing.T) {
	t.Parallel()
	err := Transition(StatusCompleted, StatusExecuting)
	if err == nil {
		t.Fatal("Transition did not return error for invalid transition")
	}
}

func TestTransition_ErrorContainsStatus(t *testing.T) {
	t.Parallel()
	err := Transition(StatusCompleted, StatusQueued)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "completed") {
		t.Errorf("error should mention status, got %q", err.Error())
	}
}
