package billing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
)

func TestNewPostHogClient_EmptyAPIKey_ReturnsNil(t *testing.T) {
	t.Parallel()
	if c := NewPostHogClient("", "", nil); c != nil {
		t.Error("expected nil client for empty API key")
	}
}

func TestNewPostHogClient_DefaultHost(t *testing.T) {
	t.Parallel()
	c := NewPostHogClient("key", "", nil)
	if c == nil {
		t.Fatal("expected non-nil client")
		return
	}
	if c.host != "https://us.i.posthog.com" {
		t.Errorf("host = %q, want https://us.i.posthog.com", c.host)
	}
}

func TestNewPostHogClient_CustomHost(t *testing.T) {
	t.Parallel()
	c := NewPostHogClient("key", "https://eu.posthog.com", nil)
	if c == nil {
		t.Fatal("expected non-nil client")
		return
	}
	if c.host != "https://eu.posthog.com" {
		t.Errorf("host = %q, want https://eu.posthog.com", c.host)
	}
}

func TestNewPostHogClient_NilLogger(t *testing.T) {
	t.Parallel()
	c := NewPostHogClient("key", "http://localhost", nil)
	if c == nil {
		t.Fatal("expected non-nil client")
		return
	}
	if c.logger == nil {
		t.Error("logger should default to slog.Default(), not nil")
	}
}

func TestPostHogCapture_NilReceiver(t *testing.T) {
	t.Parallel()
	(*PostHogClient)(nil).Capture(context.Background(), "user-1", "test_event", nil)
}

func TestPostHogCapture_Success(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var captured posthogCapturePayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		_ = json.Unmarshal(body, &captured)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewPostHogClient("test-key", srv.URL, nil)
	c.Capture(context.Background(), "user-42", "plan_upgraded", map[string]any{
		"plan": "pro",
	})

	mu.Lock()
	defer mu.Unlock()
	if captured.APIKey != "test-key" {
		t.Errorf("api_key = %q, want test-key", captured.APIKey)
	}
	if captured.DistinctID != "user-42" {
		t.Errorf("distinct_id = %q, want user-42", captured.DistinctID)
	}
	if captured.Event != "plan_upgraded" {
		t.Errorf("event = %q, want plan_upgraded", captured.Event)
	}
	if captured.Properties["plan"] != "pro" {
		t.Errorf("properties[plan] = %v, want pro", captured.Properties["plan"])
	}
}

func TestPostHogCapture_ServerError_NoPropagate(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewPostHogClient("key", srv.URL, nil)
	// Should not panic or propagate error.
	c.Capture(context.Background(), "user-1", "test", nil)
}

func TestPostHogCaptureAsync_NilReceiver(t *testing.T) {
	t.Parallel()
	(*PostHogClient)(nil).CaptureAsync("user-1", "test_event", nil)
}

func TestPostHogCaptureAsync_SendsInBackground(t *testing.T) {
	t.Parallel()
	hit := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewPostHogClient("key", srv.URL, nil)
	c.CaptureAsync("user-1", "async_event", nil)

	var received conc.WaitGroup
	done := make(chan struct{})
	received.Go(func() {
		<-hit
		close(done)
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("CaptureAsync did not send request within timeout")
	}
	received.Wait()
}

func TestPostHogCaptureRevenueEvent_NilReceiver(t *testing.T) {
	t.Parallel()
	(*PostHogClient)(nil).CaptureRevenueEvent("org-1", "revenue", nil)
}

func TestPostHogCaptureRevenueEvent_SetsGroups(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var captured posthogCapturePayload
	done := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		_ = json.Unmarshal(body, &captured)
		w.WriteHeader(http.StatusOK)
		select {
		case done <- struct{}{}:
		default:
		}
	}))
	defer srv.Close()

	c := NewPostHogClient("key", srv.URL, nil)
	c.CaptureRevenueEvent("org-123", "subscription_created", map[string]any{"plan": "pro"})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("async capture never reached server")
	}

	mu.Lock()
	defer mu.Unlock()
	if captured.DistinctID != "org:org-123" {
		t.Errorf("distinct_id = %q, want org:org-123", captured.DistinctID)
	}
	groups, ok := captured.Properties["$groups"]
	if !ok {
		t.Fatal("expected $groups in properties")
	}
	groupMap, ok := groups.(map[string]any)
	if !ok {
		t.Fatalf("$groups type = %T, want map", groups)
	}
	if groupMap["organization"] != "org-123" {
		t.Errorf("$groups.organization = %v, want org-123", groupMap["organization"])
	}
}
