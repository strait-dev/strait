package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"

	sandboxv1 "strait/internal/sandbox/v1"
)

func TestNewClient_Defaults(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil)
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.addr != "localhost:50051" {
		t.Errorf("addr: got %q, want %q", c.addr, "localhost:50051")
	}
	if c.logger == nil {
		t.Error("expected default logger when nil is passed")
	}
	if c.cfg.keepaliveTime != defaultKeepaliveTime {
		t.Errorf("keepalive time: got %v, want %v", c.cfg.keepaliveTime, defaultKeepaliveTime)
	}
	if c.cfg.keepaliveTimeout != defaultKeepaliveTimeout {
		t.Errorf("keepalive timeout: got %v, want %v", c.cfg.keepaliveTimeout, defaultKeepaliveTimeout)
	}
}

func TestNewClient_WithOptions(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil,
		WithKeepaliveInterval(30*time.Second),
		WithKeepaliveTimeout(5*time.Second),
	)
	if c.cfg.keepaliveTime != 30*time.Second {
		t.Errorf("keepalive time: got %v, want 30s", c.cfg.keepaliveTime)
	}
	if c.cfg.keepaliveTimeout != 5*time.Second {
		t.Errorf("keepalive timeout: got %v, want 5s", c.cfg.keepaliveTimeout)
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

func TestIsConnected_BeforeConnect(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil)
	if c.IsConnected() {
		t.Error("expected IsConnected() = false before Connect()")
	}
}

func TestIsConnected_AfterConnect(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	// grpc.NewClient creates a connection in Idle state, which counts as
	// connected (usable for RPCs — gRPC connects lazily on first RPC).
	if !c.IsConnected() {
		t.Error("expected IsConnected() = true after Connect()")
	}
}

func TestIsConnected_AfterClose(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if c.IsConnected() {
		t.Error("expected IsConnected() = false after Close()")
	}
}

func TestWaitForReady_NotConnected(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil)

	err := c.WaitForReady(t.Context())
	if !errors.Is(err, ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

func TestWaitForReady_ContextCanceled(t *testing.T) {
	t.Parallel()
	// Connect to a non-existent address so it will never become Ready
	c := NewClient("localhost:19999", nil)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := c.WaitForReady(ctx)
	if err == nil {
		t.Fatal("expected error from WaitForReady with short timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}
}

func TestClientClose_NotConnected(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil)
	if err := c.Close(); err != nil {
		t.Errorf("expected no error closing unconnected client, got: %v", err)
	}
}

func TestClientClose_Idempotent(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil)
	for i := range 3 {
		if err := c.Close(); err != nil {
			t.Errorf("close call %d: unexpected error: %v", i, err)
		}
	}
}

func TestReconnect_Success(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("initial connect: %v", err)
	}
	defer c.Close()

	if err := c.Reconnect(context.Background()); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if !c.IsConnected() {
		t.Error("expected connected after reconnect")
	}
}

func TestReconnect_AfterClose(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if c.IsConnected() {
		t.Error("expected not connected after close")
	}

	if err := c.Reconnect(context.Background()); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if !c.IsConnected() {
		t.Error("expected connected after reconnect")
	}
	c.Close()
}

func TestStartReconnectLoop_ExitsOnCancel(t *testing.T) {
	t.Parallel()
	c := NewClient("localhost:50051", nil,
		WithKeepaliveInterval(10*time.Millisecond),
	)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		c.StartReconnectLoop(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good — loop exited
	case <-time.After(2 * time.Second):
		t.Fatal("reconnect loop did not exit after context cancel")
	}
}

func TestReconnectBackoff_Constants(t *testing.T) {
	t.Parallel()
	if reconnectInitial != 1*time.Second {
		t.Errorf("initial backoff: got %v, want 1s", reconnectInitial)
	}
	if reconnectMax != 16*time.Second {
		t.Errorf("max backoff: got %v, want 16s", reconnectMax)
	}
	if reconnectFactor != 2 {
		t.Errorf("backoff factor: got %d, want 2", reconnectFactor)
	}
}
