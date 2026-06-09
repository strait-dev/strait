package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestEventSource_CreateRejectsSignatureWithoutAlgorithm guards the fail-open
// config: a signature header or secret without an algorithm must be rejected at
// create time rather than silently saved as unverified.
func TestEventSource_CreateRejectsSignatureWithoutAlgorithm(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateEventSourceFunc: func(_ context.Context, _ *domain.EventSource) error { return nil },
	}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})

	body := `{"project_id":"proj-1","name":"sig-src","signature_header":"X-Sig","signature_secret":"shh"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/event-sources", body, "proj-1"))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestEventSource_DispatchFailsClosedWithoutAlgorithm guards the fail-open at
// dispatch: a source configured with a signature header/secret but no algorithm
// must NOT accept an unverified payload.
func TestEventSource_DispatchFailsClosedWithoutAlgorithm(t *testing.T) {
	t.Parallel()
	var subsCalled bool
	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
			return &domain.EventSource{
				ID: "src", ProjectID: projectID, Name: name, Enabled: true,
				SignatureHeader:    "X-Sig",
				SignatureSecretEnc: []byte("encrypted:shh"),
				// SignatureAlgorithm intentionally empty.
			}, nil
		},
		ListEventSubscriptionsBySourceFunc: func(_ context.Context, _ string) ([]domain.EventSubscription, error) {
			subsCalled = true
			return nil, nil
		},
	}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})

	body := `{"source":"sig-src","project_id":"proj-1","payload":{"a":1}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", body))
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.False(t, subsCalled, "must not dispatch an unverified payload")
}
