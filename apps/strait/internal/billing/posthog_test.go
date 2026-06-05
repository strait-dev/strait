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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPostHogClient_EmptyAPIKey_ReturnsNil(t *testing.T) {
	t.Parallel()
	assert.Nil(t, NewPostHogClient("",
		"", nil))

}

func TestNewPostHogClient_DefaultHost(t *testing.T) {
	t.Parallel()
	c := NewPostHogClient("key", "", nil)
	require.NotNil(t,
		c)
	assert.Equal(t, "https://us.i.posthog.com",

		c.host)

}

func TestNewPostHogClient_CustomHost(t *testing.T) {
	t.Parallel()
	c := NewPostHogClient("key", "https://eu.posthog.com", nil)
	require.NotNil(t,
		c)
	assert.Equal(t, "https://eu.posthog.com",

		c.host)

}

func TestNewPostHogClient_NilLogger(t *testing.T) {
	t.Parallel()
	c := NewPostHogClient("key", "http://localhost", nil)
	require.NotNil(t,
		c)
	assert.NotNil(t,
		c.logger)

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
	assert.Equal(t, "test-key",
		captured.
			APIKey,
	)
	assert.Equal(t, "user-42",
		captured.
			DistinctID,
	)
	assert.Equal(t, "plan_upgraded",
		captured.
			Event)
	assert.Equal(t, "pro",
		captured.Properties["plan"])

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
		assert.Fail(t, "CaptureAsync did not send request within timeout")
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
		require.Fail(t, "async capture never reached server")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "org:org-123",
		captured.
			DistinctID)

	groups, ok := captured.Properties["$groups"]
	require.True(t, ok)

	groupMap, ok := groups.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "org-123",
		groupMap["organization"])

}
