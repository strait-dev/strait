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
			if gotProject != projectID {
				t.Errorf("GetAuditEvent project = %q, want %q", gotProject, projectID)
			}
			if gotID != eventID {
				t.Errorf("GetAuditEvent id = %q, want %q", gotID, eventID)
			}
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

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if getCalls != 1 {
		t.Fatalf("GetAuditEvent called %d times, want 1", getCalls)
	}

	// Find the self-audit row.
	var found *domain.AuditEvent
	for _, ev := range emitted {
		if ev.Action == domain.AuditActionAuditSingleRead {
			found = ev
			break
		}
	}
	if found == nil {
		t.Fatalf("no audit.single_read event emitted; got %d events", len(emitted))
		return
	}

	if found.ProjectID != projectID {
		t.Errorf("emitted project_id = %q, want %q", found.ProjectID, projectID)
	}
	if found.ResourceType != "audit" {
		t.Errorf("emitted resource_type = %q, want 'audit'", found.ResourceType)
	}
	if found.ResourceID != eventID {
		t.Errorf("emitted resource_id = %q, want %q", found.ResourceID, eventID)
	}

	var details map[string]any
	if err := json.Unmarshal(found.Details, &details); err != nil {
		t.Fatalf("unmarshal details: %v", err)
	}
	if details["target_id"] != eventID {
		t.Errorf("details.target_id = %v, want %q", details["target_id"], eventID)
	}
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

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", w.Code, w.Body.String())
	}
	if seenProject != "proj-a" {
		t.Errorf("store called with project = %q, want %q (must use ctx, not URL)", seenProject, "proj-a")
	}
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
			if id != secretID {
				t.Errorf("GetJobSecret id = %q, want %q", id, secretID)
			}
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

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	// Response body must not leak encrypted or plaintext values.
	body := w.Body.String()
	if strings.Contains(body, "ciphertext-never-returned") {
		t.Fatalf("response body leaked encrypted value: %s", body)
	}
	if strings.Contains(body, "encrypted_value") {
		t.Fatalf("response body contained encrypted_value key: %s", body)
	}

	mu.Lock()
	defer mu.Unlock()
	var found *domain.AuditEvent
	for _, ev := range emitted {
		if ev.Action == domain.AuditActionSecretRead {
			found = ev
			break
		}
	}
	if found == nil {
		t.Fatalf("no secret.read event emitted; got %d events", len(emitted))
		return
	}

	if found.ResourceType != "secret" {
		t.Errorf("resource_type = %q, want 'secret'", found.ResourceType)
	}
	if found.ResourceID != secretID {
		t.Errorf("resource_id = %q, want %q", found.ResourceID, secretID)
	}

	var details map[string]any
	if err := json.Unmarshal(found.Details, &details); err != nil {
		t.Fatalf("unmarshal details: %v", err)
	}
	if details["secret_id"] != secretID {
		t.Errorf("details.secret_id = %v, want %q", details["secret_id"], secretID)
	}
	if details["name"] != keyName {
		t.Errorf("details.name = %v, want %q", details["name"], keyName)
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	var found *domain.AuditEvent
	for _, ev := range emitted {
		if ev.Action == domain.AuditActionSecretRead {
			found = ev
			break
		}
	}
	if found == nil {
		t.Fatal("secret.read audit not emitted")
		return
	}

	var details map[string]any
	if err := json.Unmarshal(found.Details, &details); err != nil {
		t.Fatalf("unmarshal details: %v", err)
	}

	// None of these keys may ever appear.
	forbidden := []string{"key", "value", "secret", "key_material", "encrypted_value", "plaintext", "password", "token", "bearer"}
	for _, k := range forbidden {
		if _, ok := details[k]; ok {
			t.Errorf("secret.read details contains forbidden key %q: %v", k, details)
		}
	}

	// And the raw marshaled payload must not contain the plaintext value or
	// ciphertext — defense in depth against a future emitter accidentally
	// stuffing it under a renamed key.
	raw := string(found.Details)
	for _, needle := range []string{"super-secret-value", "ENC:super-secret-value"} {
		if strings.Contains(raw, needle) {
			t.Errorf("secret.read details leaked secret material %q: %s", needle, raw)
		}
	}
}
