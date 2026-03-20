package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamRunEvents_SurvivesBeyondClientTimeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		// Send first event immediately
		_, _ = fmt.Fprint(w, "data: {\"type\":\"event\",\"message\":\"first\",\"timestamp\":\"2026-03-19T10:00:00Z\"}\n\n")
		flusher.Flush()

		// Wait longer than the client's regular HTTP timeout (1s), then send second event
		time.Sleep(1500 * time.Millisecond)

		_, _ = fmt.Fprint(w, "data: {\"type\":\"event\",\"message\":\"second\",\"timestamp\":\"2026-03-19T10:00:02Z\"}\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	// Create client with a 1s timeout for regular HTTP requests
	c, err := New(srv.URL, "test-key", 1*time.Second)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var messages []RunStreamMessage
	err = c.StreamRunEvents(context.Background(), "run-1", func(msg RunStreamMessage) error {
		messages = append(messages, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamRunEvents: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Message != "first" || messages[1].Message != "second" {
		t.Fatalf("unexpected messages: %+v", messages)
	}
}

func TestStreamRunEvents(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertPath(t, r, "/v1/runs/run-1/stream")
		assertAuth(t, r, "test-key")

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		_, _ = fmt.Fprint(w, "data: {\"type\":\"event\",\"event_type\":\"log\",\"run_id\":\"run-1\",\"level\":\"info\",\"message\":\"hello\",\"timestamp\":\"2026-03-19T10:00:00Z\"}\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	var messages []RunStreamMessage
	err := c.StreamRunEvents(context.Background(), "run-1", func(msg RunStreamMessage) error {
		messages = append(messages, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamRunEvents: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].EventType != "log" || messages[0].Message != "hello" {
		t.Fatalf("unexpected message: %+v", messages[0])
	}
}

func TestStreamRunEvents_KeepaliveComments(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		_, _ = fmt.Fprint(w, ": keepalive\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"status_change\",\"run_id\":\"run-1\",\"from\":\"queued\",\"to\":\"executing\",\"timestamp\":\"2026-03-19T10:00:00Z\"}\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	var count int
	err := c.StreamRunEvents(context.Background(), "run-1", func(msg RunStreamMessage) error {
		count++
		if msg.Type != "status_change" {
			t.Fatalf("unexpected message type: %+v", msg)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("StreamRunEvents: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 message after keepalive, got %d", count)
	}
}

func TestStreamRunEvents_TerminalRun(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		_, _ = fmt.Fprint(w, `{"error":"run already in terminal state"}`)
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	err := c.StreamRunEvents(context.Background(), "run-done", func(RunStreamMessage) error {
		return nil
	})
	if err == nil || err.Error() != "run stream failed (410): run already in terminal state" {
		t.Fatalf("expected terminal run error, got %v", err)
	}
}

func TestStreamRunEvents_ContextCancellation(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- c.StreamRunEvents(ctx, "run-1", func(RunStreamMessage) error {
			return nil
		})
	}()

	cancel()

	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestStreamRunEvents_ServerCloses(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		_, _ = fmt.Fprint(w, "data: {\"type\":\"event\",\"message\":\"one\",\"timestamp\":\"2026-03-19T10:00:00Z\"}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"event\",\"message\":\"two\",\"timestamp\":\"2026-03-19T10:00:01Z\"}\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	var count int
	err := c.StreamRunEvents(context.Background(), "run-1", func(msg RunStreamMessage) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 events, got %d", count)
	}
}

func TestStreamRunEvents_MalformedJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "data: {not-json\n\n")
		w.(http.Flusher).Flush()
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	err := c.StreamRunEvents(context.Background(), "run-1", func(RunStreamMessage) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "decode run stream event") {
		t.Fatalf("expected decode error, got: %v", err)
	}
}

func TestStreamRunEvents_ErrorEvent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "event: error\ndata: {\"error\":\"server broke\",\"timestamp\":\"2026-03-19T10:00:00Z\"}\n\n")
		w.(http.Flusher).Flush()
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	err := c.StreamRunEvents(context.Background(), "run-1", func(RunStreamMessage) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "run stream error: server broke") {
		t.Fatalf("expected error event, got: %v", err)
	}
}

func TestStreamRunEvents_MultiLineData(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "data: {\"type\":\"event\",\"message\":\"hello\",\n")
		_, _ = fmt.Fprint(w, "data: \"timestamp\":\"2026-03-19T10:00:00Z\"}\n\n")
		w.(http.Flusher).Flush()
	}))
	defer srv.Close()

	c := mustClient(t, srv.URL)
	var messages []RunStreamMessage
	err := c.StreamRunEvents(context.Background(), "run-1", func(msg RunStreamMessage) error {
		messages = append(messages, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Message != "hello" {
		t.Fatalf("expected message 'hello', got %q", messages[0].Message)
	}
}
