package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestEventSourceSignatureReplayKey(t *testing.T) {
	t.Parallel()

	sourceID := "src-1"
	algorithm := "hmac-sha256"
	sigHeader := "sha256=abcdef"
	sum := sha256.Sum256([]byte("event-source-signature:" + sourceID + ":" + algorithm + ":" + sigHeader))
	want := "event-source-signature:" + hex.EncodeToString(sum[:])

	require.Equal(t, want, eventSourceSignatureReplayKey(sourceID, algorithm, sigHeader))
}

func BenchmarkEventSourceSignatureReplayKey(b *testing.B) {
	sourceID := "src_0123456789abcdef"
	algorithm := "hmac-sha256"
	sigHeader := "sha256=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		key := eventSourceSignatureReplayKey(sourceID, algorithm, sigHeader)
		if key == "" {
			b.Fatal("eventSourceSignatureReplayKey() returned empty key")
		}
	}
}

// handleCreateEventSource.

func TestHandleCreateEventSource_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateEventSourceFunc: func(_ context.Context, src *domain.EventSource) error {
			src.ID = "src-1"
			src.CreatedAt = time.Now()
			src.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "deploy-events",
		"description": "Deploy lifecycle events"
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/event-sources", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp domain.EventSource
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "deploy-events",
		resp.Name,
	)
	require.True(
		t, resp.Enabled)
}

func TestHandleCreateEventSource_MissingName(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"project_id": "proj-1"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/event-sources", body))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
	require.Contains(
		t, w.Body.String(), "validation",
	)
}

// handleListEventSources.

func TestHandleListEventSources_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()
	ms := &APIStoreMock{
		ListEventSourcesFunc: func(_ context.Context, projectID string) ([]domain.EventSource, error) {
			return []domain.EventSource{
				{ID: "src-1", ProjectID: projectID, Name: "src-a", Enabled: true, CreatedAt: now},
				{ID: "src-2", ProjectID: projectID, Name: "src-b", Enabled: true, CreatedAt: now},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/event-sources", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []domain.EventSource
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t,
		resp, 2)
}

func TestHandleListEventSources_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/event-sources", ""))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

// handleGetEventSource.

func TestHandleGetEventSource_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
			return &domain.EventSource{
				ID: sourceID, ProjectID: projectID, Name: "deploy-events",
				Enabled: true, CreatedAt: time.Now(),
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/event-sources/src-1", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp domain.EventSource
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "src-1", resp.
		ID)
}

func TestHandleGetEventSource_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, _, _ string) (*domain.EventSource, error) {
			return nil, store.ErrEventSourceNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/event-sources/src-999", "", "proj-1"))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

// handleUpdateEventSource.

func TestHandleUpdateEventSource_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		UpdateEventSourceFunc: func(_ context.Context, _, _ string, _ map[string]any) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"name": "renamed-source"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/event-sources/src-1", body, "proj-1"))
	require.Equal(t, http.StatusNoContent,
		w.Code,
	)
}

func TestHandleUpdateEventSource_EmptyPatch(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/event-sources/src-1", `{}`, "proj-1"))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
	require.Contains(
		t, w.Body.String(), "no fields to update")
}

// handleDeleteEventSource.

func TestHandleDeleteEventSource_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		DeleteEventSourceFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/event-sources/src-1", "", "proj-1"))
	require.Equal(t, http.StatusNoContent,
		w.Code,
	)
}

func TestHandleDeleteEventSource_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		DeleteEventSourceFunc: func(_ context.Context, _, _ string) error {
			return store.ErrEventSourceNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/event-sources/src-999", "", "proj-1"))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

// handleSubscribeToEventSource.

func TestHandleSubscribeToEventSource_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
			return &domain.EventSource{ID: sourceID, ProjectID: projectID, Name: "src", Enabled: true}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true}, nil
		},
		CreateEventSubscriptionFunc: func(_ context.Context, sub *domain.EventSubscription) error {
			sub.ID = "sub-1"
			sub.CreatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"target_type": "job",
		"target_id": "job-1",
		"filter_expr": {"eq":[["type","deploy"]]}
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/event-sources/src-1/subscribe", body, "proj-1"))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp domain.EventSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "job", resp.TargetType)
	require.Equal(t, "src-1", resp.
		SourceID)
}

func TestHandleSubscribeToEventSource_MissingTargetType(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"target_id": "job-1"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/event-sources/src-1/subscribe", body))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}

// handleListEventSourceSubscriptions.

func TestHandleListEventSourceSubscriptions_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
			return &domain.EventSource{ID: sourceID, ProjectID: projectID}, nil
		},
		ListEventSubscriptionsBySourceFunc: func(_ context.Context, sourceID string) ([]domain.EventSubscription, error) {
			return []domain.EventSubscription{
				{ID: "sub-1", SourceID: sourceID, TargetType: "job", TargetID: "job-1", Enabled: true},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/event-sources/src-1/subscriptions", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp []domain.EventSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t,
		resp, 1)
}

// handleDeleteEventSubscription.

func TestHandleDeleteEventSubscription_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
			return &domain.EventSource{ID: sourceID, ProjectID: projectID}, nil
		},
		GetEventSubscriptionFunc: func(_ context.Context, subID string) (*domain.EventSubscription, error) {
			return &domain.EventSubscription{ID: subID, SourceID: "src-1"}, nil
		},
		DeleteEventSubscriptionFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/event-sources/src-1/subscriptions/sub-1", "", "proj-1"))
	require.Equal(t, http.StatusNoContent,
		w.Code,
	)
}

func TestHandleDeleteEventSubscription_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
			return &domain.EventSource{ID: sourceID, ProjectID: projectID}, nil
		},
		GetEventSubscriptionFunc: func(_ context.Context, _ string) (*domain.EventSubscription, error) {
			return nil, store.ErrEventSubscriptionNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/event-sources/src-1/subscriptions/sub-999", "", "proj-1"))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

// handleDispatchEvent.

func TestHandleDispatchEvent_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
			return &domain.EventSource{
				ID: "src-1", ProjectID: projectID, Name: name, Enabled: true,
			}, nil
		},
		ListEventSubscriptionsBySourceFunc: func(_ context.Context, sourceID string) ([]domain.EventSubscription, error) {
			return []domain.EventSubscription{
				{
					ID: "sub-1", SourceID: sourceID, TargetType: "job", TargetID: "job-1",
					FilterExpr: json.RawMessage(`{"eq":[["type","deploy"]]}`), Enabled: true,
				},
			}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, Enabled: true, Version: 1, VersionID: "jv-1",
				ProjectID: "proj-1",
			}, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "dispatched-run-1"
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	body := `{"source":"my-source","project_id":"proj-1","payload":{"type":"deploy"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", body))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 1, int(resp["dispatched"].(float64)))
}

func TestHandleDispatchEvent_SourceNotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, _, _ string) (*domain.EventSource, error) {
			return nil, store.ErrEventSourceNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"source":"nonexistent","project_id":"proj-1","payload":{}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", body))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

func TestHandleDispatchEvent_SourceDisabled(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
			return &domain.EventSource{
				ID: "src-1", ProjectID: projectID, Name: name, Enabled: false,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"source":"my-source","project_id":"proj-1","payload":{}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", body))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
	require.Contains(
		t, w.Body.String(), "disabled")
}

func TestHandleDispatchEvent_InvalidBody(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", "not json"))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}
