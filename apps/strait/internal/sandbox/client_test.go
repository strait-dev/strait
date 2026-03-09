package sandbox

import (
	"errors"
	"testing"

	sandboxv1 "strait/internal/sandbox/v1"
)

func TestNewClient(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil)
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.addr != "localhost:50051" {
		t.Errorf("expected addr localhost:50051, got %s", c.addr)
	}
	if c.logger == nil {
		t.Error("expected default logger when nil is passed")
	}
}

func TestClientNotConnected(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil)

	_, err := c.Execute(t.Context(), &ExecuteRequest{
		RunID:    "test-run",
		Language: "python",
		Code:     "print('hello')",
	})

	if err == nil {
		t.Fatal("expected error when not connected")
	}
	if !errors.Is(err, ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

func TestClientExecuteStream_NotConnected(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil)

	err := c.ExecuteStream(t.Context(), &ExecuteRequest{
		RunID:    "test-run",
		Language: "python",
		Code:     "print('hello')",
	}, func(event *sandboxv1.ExecutionEvent) error {
		return nil
	})

	if !errors.Is(err, ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

func TestClientClose_NotConnected(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil)

	// Close without connecting should not error
	if err := c.Close(); err != nil {
		t.Errorf("expected no error closing unconnected client, got: %v", err)
	}
}

func TestClientClose_Idempotent(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil)

	// Calling close multiple times should be safe
	for i := range 3 {
		if err := c.Close(); err != nil {
			t.Errorf("close call %d: unexpected error: %v", i, err)
		}
	}
}
