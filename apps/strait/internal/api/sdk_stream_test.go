package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestHandleSDKStreamChunk_PublishesChunk(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var publishedChannel string
	var publishedPayload []byte

	pub := &mockPublisher{
		publishFn: func(_ context.Context, channel string, data []byte) error {
			mu.Lock()
			defer mu.Unlock()
			publishedChannel = channel
			publishedPayload = data
			return nil
		},
	}
	srv := newTestServer(t, &mockAPIStore{}, nil, pub)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/stream", "run-1",
		`{"chunk":"Hello ","stream_id":"default","done":false}`)
	srv.handleSDKStreamChunk(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()

	if publishedChannel != "run_stream:run-1" {
		t.Fatalf("expected channel run_stream:run-1, got %s", publishedChannel)
	}

	var msg map[string]any
	if err := json.Unmarshal(publishedPayload, &msg); err != nil {
		t.Fatalf("invalid published JSON: %v", err)
	}
	if msg["type"] != "stream_chunk" {
		t.Fatalf("expected type=stream_chunk, got %v", msg["type"])
	}
	if msg["chunk"] != "Hello " {
		t.Fatalf("expected chunk='Hello ', got %v", msg["chunk"])
	}
	if msg["stream_id"] != "default" {
		t.Fatalf("expected stream_id=default, got %v", msg["stream_id"])
	}
}

func TestHandleSDKStreamChunk_DefaultStreamID(t *testing.T) {
	t.Parallel()
	var publishedPayload []byte
	pub := &mockPublisher{
		publishFn: func(_ context.Context, _ string, data []byte) error {
			publishedPayload = data
			return nil
		},
	}
	srv := newTestServer(t, &mockAPIStore{}, nil, pub)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/stream", "run-1",
		`{"chunk":"token"}`)
	srv.handleSDKStreamChunk(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var msg map[string]any
	_ = json.Unmarshal(publishedPayload, &msg)
	if msg["stream_id"] != "default" {
		t.Fatalf("expected default stream_id, got %v", msg["stream_id"])
	}
}

func TestHandleSDKStreamChunk_NoPubSub(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/stream", "run-1",
		`{"chunk":"token"}`)
	srv.handleSDKStreamChunk(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even without pubsub, got %d", w.Code)
	}
}

func TestHandleSDKStreamChunk_InvalidBody(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/stream", "run-1", "not json")
	srv.handleSDKStreamChunk(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
