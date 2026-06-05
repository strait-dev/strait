package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
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
	srv := newTestServer(t, &APIStoreMock{}, nil, pub)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/stream", "run-1",
		`{"chunk":"Hello ","stream_id":"default","done":false}`)
	TypedHandler(srv, http.StatusOK, srv.handleSDKStreamChunk)(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "run_stream:run-1",
		publishedChannel,
	)

	var msg map[string]any
	require.NoError(t, json.Unmarshal(publishedPayload,
		&msg))
	require.Equal(t, "stream_chunk",
		msg["type"])
	require.Equal(t, "Hello ", msg["chunk"])
	require.Equal(t, "default", msg["stream_id"])

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
	srv := newTestServer(t, &APIStoreMock{}, nil, pub)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/stream", "run-1",
		`{"chunk":"token"}`)
	TypedHandler(srv, http.StatusOK, srv.handleSDKStreamChunk)(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

	var msg map[string]any
	_ = json.Unmarshal(publishedPayload, &msg)
	require.Equal(t, "default", msg["stream_id"])

}

func TestHandleSDKStreamChunk_NoPubSub(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/stream", "run-1",
		`{"chunk":"token"}`)
	TypedHandler(srv, http.StatusOK, srv.handleSDKStreamChunk)(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

}

func TestHandleSDKStreamChunk_InvalidBody(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/stream", "run-1", "not json")
	TypedHandler(srv, http.StatusOK, srv.handleSDKStreamChunk)(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)

}
