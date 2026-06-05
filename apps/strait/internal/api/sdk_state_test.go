package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestHandleSDKSetState_Success(t *testing.T) {
	t.Parallel()
	var captured *domain.RunState
	ms := &APIStoreMock{
		UpsertRunStateFunc: func(_ context.Context, s *domain.RunState) error {
			captured = s
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/state", "run-1",
		`{"key":"step_result","value":{"score":42}}`)
	TypedHandler(srv, http.StatusCreated, srv.handleSDKSetState)(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, captured)
	require.Equal(t, "run-1", captured.
		RunID)
	require.Equal(t, "step_result",
		captured.StateKey,
	)

}

func TestHandleSDKSetState_KeyTooLong(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	longKey := make([]byte, 257)
	for i := range longKey {
		longKey[i] = 'a'
	}
	body, _ := json.Marshal(map[string]any{"key": string(longKey), "value": "x"})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/state", "run-1", string(body))
	TypedHandler(srv, http.StatusCreated, srv.handleSDKSetState)(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)

}

func TestHandleSDKSetState_ValueTooLarge(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	largeValue := make([]byte, 65537)
	for i := range largeValue {
		largeValue[i] = 'x'
	}
	body, _ := json.Marshal(map[string]any{"key": "k", "value": json.RawMessage(`"` + string(largeValue) + `"`)})

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/state", "run-1", string(body))
	TypedHandler(srv, http.StatusCreated, srv.handleSDKSetState)(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)

}

func TestHandleSDKSetState_MissingKey(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/state", "run-1",
		`{"value":"hello"}`)
	TypedHandler(srv, http.StatusCreated, srv.handleSDKSetState)(w, r)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)

}

func TestHandleSDKGetState_Found(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunStateFunc: func(_ context.Context, runID, key string) (*domain.RunState, error) {
			return &domain.RunState{RunID: runID, StateKey: key, Value: json.RawMessage(`"hello"`)}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodGet, "/sdk/v1/runs/run-1/state/mykey", "run-1", "")
	rctx := chi.RouteContext(r.Context())
	rctx.URLParams.Add("key", "mykey")
	TypedHandler(srv, http.StatusOK, srv.handleSDKGetState)(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

}

func TestHandleSDKGetState_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetRunStateFunc: func(context.Context, string, string) (*domain.RunState, error) {
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodGet, "/sdk/v1/runs/run-1/state/missing", "run-1", "")
	rctx := chi.RouteContext(r.Context())
	rctx.URLParams.Add("key", "missing")
	TypedHandler(srv, http.StatusOK, srv.handleSDKGetState)(w, r)
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)

}

func TestHandleSDKListState(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListRunStateFunc: func(context.Context, string) ([]domain.RunState, error) {
			return []domain.RunState{
				{RunID: "run-1", StateKey: "a", Value: json.RawMessage(`1`)},
				{RunID: "run-1", StateKey: "b", Value: json.RawMessage(`2`)},
			}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodGet, "/sdk/v1/runs/run-1/state", "run-1", "")
	TypedHandler(srv, http.StatusOK, srv.handleSDKListState)(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

	var items []domain.RunState
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
	require.Len(t,
		items, 2)

}

func TestHandleSDKDeleteState(t *testing.T) {
	t.Parallel()
	var deletedKey string
	ms := &APIStoreMock{
		DeleteRunStateFunc: func(_ context.Context, _, key string) error {
			deletedKey = key
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodDelete, "/sdk/v1/runs/run-1/state/mykey", "run-1", "")
	rctx := chi.RouteContext(r.Context())
	rctx.URLParams.Add("key", "mykey")
	TypedHandler(srv, http.StatusNoContent, srv.handleSDKDeleteState)(w, r)
	require.Equal(t, http.StatusNoContent,
		w.Code,
	)
	require.Equal(t, "mykey", deletedKey)

}
