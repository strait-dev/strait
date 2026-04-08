package cli_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"strait/internal/cli"
)

func TestReadEvents_BasicDataLines(t *testing.T) {
	body := "data: {\"chunk\":\"line1\"}\ndata: {\"chunk\":\"line2\"}\n\n"

	var received []string
	err := cli.ReadEvents(context.Background(), strings.NewReader(body), func(data []byte) {
		received = append(received, string(data))
	})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(received), received)
	}
	if received[0] != `{"chunk":"line1"}` {
		t.Errorf("event 0: got %q", received[0])
	}
	if received[1] != `{"chunk":"line2"}` {
		t.Errorf("event 1: got %q", received[1])
	}
}

func TestReadEvents_IgnoresNonDataLines(t *testing.T) {
	body := ": comment\nevent: build\ndata: hello\nid: 1\n\n"

	var received []string
	if err := cli.ReadEvents(context.Background(), strings.NewReader(body), func(data []byte) {
		received = append(received, string(data))
	}); err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(received) != 1 || received[0] != "hello" {
		t.Errorf("expected [hello], got %v", received)
	}
}

func TestReadEvents_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled

	pr, pw := createPipe(t)
	defer pw.Close()

	err := cli.ReadEvents(ctx, pr, func(_ []byte) {})
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestReadEvents_EmptyStream(t *testing.T) {
	var received []string
	if err := cli.ReadEvents(context.Background(), strings.NewReader(""), func(data []byte) {
		received = append(received, string(data))
	}); err != nil {
		t.Fatalf("ReadEvents on empty stream: %v", err)
	}
	if len(received) != 0 {
		t.Errorf("expected no events, got %d", len(received))
	}
}

// createPipe creates a pair of *os.File (read, write) for controlling stream ends.
func createPipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r, w
}
