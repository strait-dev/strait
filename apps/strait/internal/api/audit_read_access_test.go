package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
)

func TestAuditReadAccess_ListAuditEvents(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var captured []domain.AuditEvent

	ms := &APIStoreMock{
		ListAuditEventsFunc: func(_ context.Context, _ string, _, _, _ string, _ int, _ *time.Time, _, _ *time.Time, _ bool) ([]domain.AuditEvent, error) {
			return []domain.AuditEvent{
				{ID: "ev-1", Action: domain.AuditActionJobCreated, CreatedAt: time.Now()},
			}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			captured = append(captured, *ev)
			mu.Unlock()
			return nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	_, err := srv.handleListAuditEvents(adminCtx("proj-1"), &ListAuditEventsInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, ev := range captured {
		if ev.Action == domain.AuditActionAuditListRead {
			found = true
			if ev.ProjectID != "proj-1" {
				t.Errorf("project_id = %q, want %q", ev.ProjectID, "proj-1")
			}
		}
	}
	if !found {
		t.Error("expected audit.list_read event to be emitted")
	}
}

func TestAuditReadAccess_GetAuditEvent(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var captured []domain.AuditEvent

	ms := &APIStoreMock{
		GetAuditEventFunc: func(_ context.Context, projectID, id string) (*domain.AuditEvent, error) {
			return &domain.AuditEvent{ID: id, ProjectID: projectID, Action: domain.AuditActionJobCreated}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			captured = append(captured, *ev)
			mu.Unlock()
			return nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	_, err := srv.handleGetAuditEvent(adminCtx("proj-1"), &GetAuditEventInput{ID: "ev-target"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, ev := range captured {
		if ev.Action == domain.AuditActionAuditSingleRead {
			found = true
			assertDetailContains(t, ev, "target_id", "ev-target")
		}
	}
	if !found {
		t.Error("expected audit.single_read event to be emitted")
	}
}

func TestAuditReadAccess_VerifyChain(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var captured []domain.AuditEvent

	ms := &APIStoreMock{
		VerifyAuditChainFunc: func(_ context.Context, projectID string) (*domain.AuditChainVerification, error) {
			return &domain.AuditChainVerification{ProjectID: projectID, Valid: true, EventsChecked: 42}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			captured = append(captured, *ev)
			mu.Unlock()
			return nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	_, err := srv.handleVerifyAuditChain(adminCtx("proj-1"), &VerifyAuditChainInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, ev := range captured {
		if ev.Action == domain.AuditActionAuditChainVerified {
			found = true
			assertDetailContains(t, ev, "valid", true)
			assertDetailContains(t, ev, "events_checked", float64(42))
		}
	}
	if !found {
		t.Error("expected audit.chain_verified event to be emitted")
	}
}

func TestAuditReadAccess_EnvironmentScopedKeyRejected(t *testing.T) {
	t.Parallel()

	ctx := adminCtx("proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-staging")

	srv := newTestServer(t, &APIStoreMock{
		ListAuditEventsFunc: func(context.Context, string, string, string, string, int, *time.Time, *time.Time, *time.Time, bool) ([]domain.AuditEvent, error) {
			t.Fatal("ListAuditEvents must not be called for environment-scoped audit access")
			return nil, nil
		},
		GetAuditEventFunc: func(context.Context, string, string) (*domain.AuditEvent, error) {
			t.Fatal("GetAuditEvent must not be called for environment-scoped audit access")
			return nil, nil
		},
		VerifyAuditChainFunc: func(context.Context, string) (*domain.AuditChainVerification, error) {
			t.Fatal("VerifyAuditChain must not be called for environment-scoped audit access")
			return nil, nil
		},
	}, nil, nil)

	cases := []struct {
		name string
		call func() error
	}{
		{
			name: "list",
			call: func() error {
				_, err := srv.handleListAuditEvents(ctx, &ListAuditEventsInput{})
				return err
			},
		},
		{
			name: "get",
			call: func() error {
				_, err := srv.handleGetAuditEvent(ctx, &GetAuditEventInput{ID: "ev-1"})
				return err
			},
		},
		{
			name: "verify",
			call: func() error {
				_, err := srv.handleVerifyAuditChain(ctx, &VerifyAuditChainInput{})
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			var statusErr huma.StatusError
			if !errors.As(err, &statusErr) {
				t.Fatalf("expected huma.StatusError, got %T: %v", err, err)
			}
			if statusErr.GetStatus() != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", statusErr.GetStatus())
			}
		})
	}
}

func TestAuditReadAccess_ListSecrets(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var captured []domain.AuditEvent

	ms := &APIStoreMock{
		ListJobSecretsFunc: func(_ context.Context, _, _, _ string, _ int, _ *time.Time) ([]domain.JobSecret, error) {
			return []domain.JobSecret{
				{ID: "sec-1", ProjectID: "proj-1", SecretKey: "DB_PASS", CreatedAt: time.Now()},
			}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			captured = append(captured, *ev)
			mu.Unlock()
			return nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	_, err := srv.handleListSecrets(adminCtx("proj-1"), &ListSecretsInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, ev := range captured {
		if ev.Action == domain.AuditActionSecretListRead {
			found = true
			assertDetailNotContains(t, ev, "value")
			assertDetailNotContains(t, ev, "secret_value")
		}
	}
	if !found {
		t.Error("expected secret.list_read event to be emitted")
	}
}

func TestAuditReadAccess_GetSecret(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var captured []domain.AuditEvent

	ms := &APIStoreMock{
		GetJobSecretFunc: func(_ context.Context, id string, _ string) (*domain.JobSecret, error) {
			return &domain.JobSecret{
				ID:        id,
				ProjectID: "proj-1",
				SecretKey: "DB_PASS",
			}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			captured = append(captured, *ev)
			mu.Unlock()
			return nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	_, err := srv.handleGetSecret(adminCtx("proj-1"), &GetSecretInput{SecretID: "sec-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, ev := range captured {
		if ev.Action == domain.AuditActionSecretRead {
			found = true
			assertDetailContains(t, ev, "secret_id", "sec-1")
			assertDetailNotContains(t, ev, "value")
			assertDetailNotContains(t, ev, "secret_value")
		}
	}
	if !found {
		t.Error("expected secret.read event to be emitted")
	}
}

func TestAuditReadAccess_ListAPIKeys(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var captured []domain.AuditEvent

	ms := &APIStoreMock{
		ListAPIKeysByProjectFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.APIKey, error) {
			return []domain.APIKey{
				{ID: "key-1", CreatedAt: time.Now()},
			}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			captured = append(captured, *ev)
			mu.Unlock()
			return nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	_, err := srv.handleListAPIKeys(adminCtx("proj-1"), &ListAPIKeysInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, ev := range captured {
		if ev.Action == domain.AuditActionAPIKeyListRead {
			found = true
		}
	}
	if !found {
		t.Error("expected api_key.list_read event to be emitted")
	}
}

func assertDetailContains(t *testing.T, ev domain.AuditEvent, key string, want any) {
	t.Helper()
	details := parseDetails(t, ev)
	got, ok := details[key]
	if !ok {
		t.Errorf("event %s details missing key %q; details=%v", ev.Action, key, details)
		return
	}
	if got != want {
		t.Errorf("event %s details[%q] = %v (%T), want %v (%T)", ev.Action, key, got, got, want, want)
	}
}

func assertDetailNotContains(t *testing.T, ev domain.AuditEvent, key string) {
	t.Helper()
	details := parseDetails(t, ev)
	if _, ok := details[key]; ok {
		t.Errorf("event %s details should NOT contain key %q but it does; details=%v", ev.Action, key, details)
	}
}

func parseDetails(t *testing.T, ev domain.AuditEvent) map[string]any {
	t.Helper()
	if len(ev.Details) == 0 {
		t.Fatalf("event %s has nil/empty details", ev.Action)
	}
	var m map[string]any
	if err := json.Unmarshal(ev.Details, &m); err != nil {
		t.Fatalf("event %s details not valid JSON: %v", ev.Action, err)
	}
	return m
}
