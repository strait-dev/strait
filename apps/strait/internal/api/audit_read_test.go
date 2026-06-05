package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetAuditEvent_EmitsSingleReadAudit verifies GET /v1/audit-events/{id}
// returns the event and emits a self-audit row with action
// audit.single_read and details{target_id}.
func TestGetAuditEvent_EmitsSingleReadAudit(t *testing.T) {
	t.Parallel()

	const (
		projectID = "proj-a"
		eventID   = "ev-42"
	)

	var (
		mu       sync.Mutex
		emitted  []*domain.AuditEvent
		getCalls int
	)
	ms := &APIStoreMock{
		GetAuditEventFunc: func(_ context.Context, gotProject, gotID string) (*domain.AuditEvent, error) {
			mu.Lock()
			getCalls++
			mu.Unlock()
			assert.Equal(
				t, projectID, gotProject,
			)
			assert.Equal(
				t, eventID, gotID,
			)

			return &domain.AuditEvent{
				ID:        eventID,
				ProjectID: projectID,
				Action:    domain.AuditActionJobCreated,
				CreatedAt: time.Now().UTC(),
			}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			clone := *ev
			emitted = append(emitted, &clone)
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/audit-events/"+eventID, "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)

	mu.Lock()
	defer mu.Unlock()
	require.EqualValues(t, 1, getCalls)

	// Find the self-audit row.
	var found *domain.AuditEvent
	for _, ev := range emitted {
		if ev.Action == domain.AuditActionAuditSingleRead {
			found = ev
			break
		}
	}
	require.NotNil(t, found)
	assert.Equal(
		t, projectID, found.
			ProjectID,
	)
	assert.Equal(
		t, "audit", found.
			ResourceType,
	)
	assert.Equal(
		t, eventID, found.
			ResourceID)

	var details map[string]any
	require.NoError(t, json.Unmarshal(found.Details,
		&details))
	assert.Equal(
		t, eventID, details["target_id"])

}

// TestGetAuditEvent_RejectsCrossTenant verifies the store is called with
// the context project id (not the URL), so project A cannot fetch
// project B's audit row. A not-found from the store is surfaced as 404.
func TestGetAuditEvent_RejectsCrossTenant(t *testing.T) {
	t.Parallel()

	var seenProject string
	ms := &APIStoreMock{
		GetAuditEventFunc: func(_ context.Context, gotProject, _ string) (*domain.AuditEvent, error) {
			seenProject = gotProject
			// Cross-tenant: row belongs to proj-b, not seenProject.
			return nil, store.ErrAuditEventNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/audit-events/ev-owned-by-b", "", "proj-a")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
	assert.Equal(
		t, "proj-a", seenProject,
	)

}

// TestGetSecret_EmitsSecretReadAudit verifies GET /v1/secrets/{id} returns
// the secret metadata and emits a secret.read audit with secret_id + name.
func TestGetSecret_EmitsSecretReadAudit(t *testing.T) {
	t.Parallel()

	const (
		projectID = "proj-a"
		secretID  = "sec-1"
		keyName   = "DATABASE_URL"
	)

	var (
		mu      sync.Mutex
		emitted []*domain.AuditEvent
	)
	ms := &APIStoreMock{
		GetJobSecretFunc: func(_ context.Context, id string) (*domain.JobSecret, error) {
			assert.Equal(
				t, secretID, id)

			return &domain.JobSecret{
				ID:             secretID,
				ProjectID:      projectID,
				SecretKey:      keyName,
				EncryptedValue: "ENC:ciphertext-never-returned",
				Environment:    "production",
				KeyVersion:     1,
			}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			clone := *ev
			emitted = append(emitted, &clone)
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/secrets/"+secretID, "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)

	// Response body must not leak encrypted or plaintext values.
	body := w.Body.String()
	require.False(t, strings.Contains(body, "ciphertext-never-returned"))
	require.False(t, strings.Contains(body, "encrypted_value"))

	mu.Lock()
	defer mu.Unlock()
	var found *domain.AuditEvent
	for _, ev := range emitted {
		if ev.Action == domain.AuditActionSecretRead {
			found = ev
			break
		}
	}
	require.NotNil(t, found)
	assert.Equal(
		t, "secret", found.
			ResourceType,
	)
	assert.Equal(
		t, secretID, found.
			ResourceID,
	)

	var details map[string]any
	require.NoError(t, json.Unmarshal(found.Details,
		&details))
	assert.Equal(
		t, secretID, details["secret_id"])
	assert.Equal(
		t, keyName, details["name"])

}

// TestSecretReadAudit_NeverContainsKeyMaterial ensures the secret.read audit
// details payload never carries forbidden key-material keys, regardless of
// the secret's content.
func TestSecretReadAudit_NeverContainsKeyMaterial(t *testing.T) {
	t.Parallel()

	const (
		projectID = "proj-a"
		secretID  = "sec-evil"
	)

	var (
		mu      sync.Mutex
		emitted []*domain.AuditEvent
	)
	ms := &APIStoreMock{
		GetJobSecretFunc: func(_ context.Context, _ string) (*domain.JobSecret, error) {
			return &domain.JobSecret{
				ID:             secretID,
				ProjectID:      projectID,
				SecretKey:      "AWS_SECRET_KEY",
				EncryptedValue: "ENC:super-secret-value",
				Environment:    "production",
			}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			clone := *ev
			emitted = append(emitted, &clone)
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := authedProjectRequest(http.MethodGet, "/v1/secrets/"+secretID, "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)

	mu.Lock()
	defer mu.Unlock()
	var found *domain.AuditEvent
	for _, ev := range emitted {
		if ev.Action == domain.AuditActionSecretRead {
			found = ev
			break
		}
	}
	require.NotNil(t, found)

	var details map[string]any
	require.NoError(t, json.Unmarshal(found.Details,
		&details))

	// None of these keys may ever appear.
	forbidden := []string{"key", "value", "secret", "key_material", "encrypted_value", "plaintext", "password", "token", "bearer"}
	for _, k := range forbidden {
		if _, ok := details[k]; ok {
			assert.Failf(t, "test failure",

				"secret.read details contains forbidden key %q: %v", k, details)
		}
	}

	// And the raw marshaled payload must not contain the plaintext value or
	// ciphertext — defense in depth against a future emitter accidentally
	// stuffing it under a renamed key.
	raw := string(found.Details)
	for _, needle := range []string{"super-secret-value", "ENC:super-secret-value"} {
		assert.False(
			t, strings.Contains(raw, needle))

	}
}
