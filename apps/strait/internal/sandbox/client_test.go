package sandbox

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("localhost:50051", nil)
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.addr != "localhost:50051" {
		t.Errorf("expected addr localhost:50051, got %s", c.addr)
	}
}

func TestClientNotConnected(t *testing.T) {
	c := NewClient("localhost:50051", nil)

	_, err := c.Execute(t.Context(), &ExecuteRequest{
		RunID:    "test-run",
		Language: "python",
		Code:     "print('hello')",
	})

	if err == nil {
		t.Fatal("expected error when not connected")
	}
	if err.Error() != "sandbox client not connected" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClientClose(t *testing.T) {
	c := NewClient("localhost:50051", nil)

	// Close without connecting should not error
	if err := c.Close(); err != nil {
		t.Errorf("expected no error closing unconnected client, got: %v", err)
	}
}
