package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

// adminCtx returns a context that satisfies requireAdmin (scopes == nil,
// i.e. internal-secret auth path) and carries an authenticated project id
// plus the minimum forensic/actor fields required by emitAuditEvent.
func adminCtx(projectID string) context.Context {
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, projectID)
	ctx = context.WithValue(ctx, ctxActorIDKey, "internal:admin")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "internal")
	return ctx
}

// nonAdminCtx mimics a request authenticated via API key: scopes slice is
// non-nil, so requireAdmin must return 403.
func nonAdminCtx(projectID string) context.Context {
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, projectID)
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:k-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	return ctx
}

func TestListDeadletter_RequiresAdmin(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	_, err := srv.handleListDeadletter(nonAdminCtx("proj-a"), &ListDeadletterInput{})
	if err == nil {
		t.Fatal("expected 403 for non-admin caller, got nil")
	}
	if !strings.Contains(err.Error(), "admin") {
		t.Errorf("expected admin-required error, got %v", err)
	}
}

func TestListDeadletter_ReturnsEntriesForOwnProject(t *testing.T) {
	t.Parallel()

	var listProject atomic.Value
	listProject.Store("")
	ms := &APIStoreMock{
		ListAuditEventsDeadletterByProjectFunc: func(_ context.Context, projectID string, _ int, _ string) ([]domain.AuditEvent, []string, []string, error) {
			listProject.Store(projectID)
			return []domain.AuditEvent{
				{ID: "dlq-1", ProjectID: projectID, Action: domain.AuditActionJobTriggered, CreatedAt: time.Now().UTC(), Details: json.RawMessage(`{"run_id":"r-1"}`)},
				{ID: "dlq-2", ProjectID: projectID, Action: domain.AuditActionJobTriggered, CreatedAt: time.Now().UTC(), Details: json.RawMessage(`{"run_id":"r-2"}`)},
			}, []string{"dlq-1", "dlq-2"}, []string{"2026-04-01T00:00:00Z", "2026-04-01T00:00:01Z"}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}
	srv := newTestServer(t, ms, nil, nil)

	out, err := srv.handleListDeadletter(adminCtx("proj-a"), &ListDeadletterInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := listProject.Load().(string); got != "proj-a" {
		t.Errorf("store called with project_id = %q, want %q", got, "proj-a")
	}
	if len(out.Body.Entries) != 2 {
		t.Errorf("Entries len = %d, want 2", len(out.Body.Entries))
	}
}

func TestListDeadletter_RejectsCrossTenantProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	// Admin of proj-a tries to list proj-b's DLQ via the query param.
	// Cross-tenant must surface as 404 (not 403) so a probing admin
	// cannot enumerate other projects by watching error codes.
	_, err := srv.handleListDeadletter(adminCtx("proj-a"), &ListDeadletterInput{ProjectID: "proj-b"})
	if err == nil {
		t.Fatal("expected 404 for cross-tenant request, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestReplayDeadletter_MovesEventToChain(t *testing.T) {
	t.Parallel()

	seed := &domain.AuditEvent{
		ID:        "dlq-1",
		ProjectID: "proj-a",
		Action:    domain.AuditActionJobTriggered,
		Details:   json.RawMessage(`{"run_id":"r-1"}`),
		CreatedAt: time.Now().UTC(),
	}

	var (
		mu               sync.Mutex
		createdEvents    []domain.AuditEvent
		deletedDLQID     string
		selfAuditFound   bool
		selfAuditEventID string
		selfAuditDLQID   string
	)

	ms := &APIStoreMock{
		GetAuditEventDeadletterFunc: func(_ context.Context, id, projectID string) (*domain.AuditEvent, error) {
			if id == seed.ID && projectID == seed.ProjectID {
				clone := *seed
				return &clone, nil
			}
			return nil, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			createdEvents = append(createdEvents, *ev)
			if ev.Action == domain.AuditActionDeadletterReplayed {
				selfAuditFound = true
				var d map[string]any
				_ = json.Unmarshal(ev.Details, &d)
				if s, ok := d["new_event_id"].(string); ok {
					selfAuditEventID = s
				}
				if s, ok := d["deadletter_id"].(string); ok {
					selfAuditDLQID = s
				}
			}
			return nil
		},
		DeleteAuditEventDeadletterFunc: func(_ context.Context, id string) error {
			mu.Lock()
			defer mu.Unlock()
			deletedDLQID = id
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	out, err := srv.handleReplayDeadletter(adminCtx("proj-a"), &ReplayDeadletterInput{ID: "dlq-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if deletedDLQID != "dlq-1" {
		t.Errorf("DeleteAuditEventDeadletter called with %q, want %q", deletedDLQID, "dlq-1")
	}
	if len(createdEvents) < 2 {
		t.Fatalf("expected at least 2 CreateAuditEvent calls (replay + self-audit), got %d", len(createdEvents))
	}
	if !selfAuditFound {
		t.Error("self-audit audit.deadletter_replayed event never emitted")
	}
	if selfAuditDLQID != "dlq-1" {
		t.Errorf("self-audit deadletter_id = %q, want %q", selfAuditDLQID, "dlq-1")
	}
	if selfAuditEventID != out.Body.NewEventID {
		t.Errorf("self-audit new_event_id = %q, want %q", selfAuditEventID, out.Body.NewEventID)
	}
	if out.Body.NewEventID == seed.ID {
		t.Error("replayed event must have a fresh id, not the DLQ id")
	}
}

func TestReplayDeadletter_NotFound_404(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetAuditEventDeadletterFunc: func(_ context.Context, _, _ string) (*domain.AuditEvent, error) {
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleReplayDeadletter(adminCtx("proj-a"), &ReplayDeadletterInput{ID: "missing"})
	if err == nil {
		t.Fatal("expected 404 for missing id, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestDropDeadletter_EmitsAuditAndRemoves(t *testing.T) {
	t.Parallel()

	seed := &domain.AuditEvent{ID: "dlq-1", ProjectID: "proj-a", Action: domain.AuditActionJobTriggered, CreatedAt: time.Now().UTC()}
	var (
		mu           sync.Mutex
		deletedID    string
		selfAuditHit bool
		seenReason   string
	)

	ms := &APIStoreMock{
		GetAuditEventDeadletterFunc: func(_ context.Context, id, projectID string) (*domain.AuditEvent, error) {
			if id == seed.ID && projectID == seed.ProjectID {
				clone := *seed
				return &clone, nil
			}
			return nil, nil
		},
		DeleteAuditEventDeadletterFunc: func(_ context.Context, id string) error {
			mu.Lock()
			defer mu.Unlock()
			deletedID = id
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			if ev.Action == domain.AuditActionDeadletterDropped {
				selfAuditHit = true
				var d map[string]any
				_ = json.Unmarshal(ev.Details, &d)
				if s, ok := d["reason"].(string); ok {
					seenReason = s
				}
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleDropDeadletter(adminCtx("proj-a"), &DropDeadletterInput{ID: "dlq-1", Reason: "corrupt_payload"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if deletedID != "dlq-1" {
		t.Errorf("DeleteAuditEventDeadletter called with %q, want %q", deletedID, "dlq-1")
	}
	if !selfAuditHit {
		t.Error("expected audit.deadletter_dropped self-audit, none emitted")
	}
	if seenReason != "corrupt_payload" {
		t.Errorf("self-audit reason = %q, want %q", seenReason, "corrupt_payload")
	}
}

func TestDropDeadletter_CrossTenant_404(t *testing.T) {
	t.Parallel()

	// Store returns nil because GetAuditEventDeadletter is project-scoped:
	// row belongs to proj-b but the admin is on proj-a.
	ms := &APIStoreMock{
		GetAuditEventDeadletterFunc: func(_ context.Context, _, projectID string) (*domain.AuditEvent, error) {
			if projectID == "proj-a" {
				return nil, nil // row exists under proj-b but not visible
			}
			return &domain.AuditEvent{ID: "dlq-1", ProjectID: "proj-b"}, nil
		},
		DeleteAuditEventDeadletterFunc: func(_ context.Context, _ string) error {
			t.Error("Delete must not be called on a cross-tenant drop")
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleDropDeadletter(adminCtx("proj-a"), &DropDeadletterInput{ID: "dlq-1", Reason: "x"})
	if err == nil {
		t.Fatal("expected 404 for cross-tenant drop, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 404 not-found, got %v", err)
	}
}

func TestReplayDeadletter_ChainInsertFailure_LeavesInDLQ(t *testing.T) {
	t.Parallel()

	seed := &domain.AuditEvent{ID: "dlq-1", ProjectID: "proj-a", Action: domain.AuditActionJobTriggered, CreatedAt: time.Now().UTC()}
	var (
		mu           sync.Mutex
		deleteCalled bool
		selfAuditHit bool
	)

	ms := &APIStoreMock{
		GetAuditEventDeadletterFunc: func(_ context.Context, id, projectID string) (*domain.AuditEvent, error) {
			if id == seed.ID && projectID == seed.ProjectID {
				clone := *seed
				return &clone, nil
			}
			return nil, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			if ev.Action == domain.AuditActionDeadletterReplayed {
				selfAuditHit = true
				return nil
			}
			// Simulate a broken chain insert for the replayed event.
			return errors.New("chain down")
		},
		DeleteAuditEventDeadletterFunc: func(_ context.Context, _ string) error {
			mu.Lock()
			defer mu.Unlock()
			deleteCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleReplayDeadletter(adminCtx("proj-a"), &ReplayDeadletterInput{ID: "dlq-1"})
	if err == nil {
		t.Fatal("expected error on chain insert failure, got nil")
	}

	mu.Lock()
	defer mu.Unlock()
	if deleteCalled {
		t.Error("DeleteAuditEventDeadletter must not be called when chain insert fails")
	}
	if selfAuditHit {
		t.Error("audit.deadletter_replayed must not be emitted when replay fails")
	}
}

// TestRedactDeadletterFilter_StripsSecretShapes asserts the filter now
// redacts by shape rather than by key-name allow-list. The previous
// implementation treated a bare Stripe secret key as "safe" because
// neither the key name nor the containing string contained the words
// "secret" or "token"; scanAndRedact catches it by regex.
func TestRedactDeadletterFilter_StripsSecretShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		projectID string
		limit     string
		cursor    string
		mustHide  string
	}{
		{"stripe-secret-in-cursor", "proj-a", "50", "sk_live_abcdefghijklmnop", "sk_live_abcdefghijklmnop"},
		{"bearer-in-projectid", "Bearer abcdefghijklmnop1234", "50", "", "abcdefghijklmnop1234"},
		{"whsec-in-limit", "proj-a", "whsec_abcdefghijklmnop", "", "whsec_abcdefghijklmnop"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := redactDeadletterFilter(tc.projectID, tc.limit, tc.cursor)
			if strings.Contains(out, tc.mustHide) {
				t.Errorf("redactDeadletterFilter(%q,%q,%q) = %q; must not contain %q", tc.projectID, tc.limit, tc.cursor, out, tc.mustHide)
			}
			if !strings.Contains(out, "[redacted:") {
				t.Errorf("redactDeadletterFilter(%q,%q,%q) = %q; missing redacted marker", tc.projectID, tc.limit, tc.cursor, out)
			}
		})
	}
}

// TestRedactDeadletterFilter_PassesThroughPlainValues asserts normal
// query params survive unchanged — the scanner must not false-positive
// on UUID-shaped cursors, numeric limits, or short project ids.
func TestRedactDeadletterFilter_PassesThroughPlainValues(t *testing.T) {
	t.Parallel()

	out := redactDeadletterFilter("proj-a", "50", "2026-04-01T00:00:00Z")
	want := "project_id=proj-a&limit=50&cursor=2026-04-01T00:00:00Z"
	if out != want {
		t.Errorf("filter = %q, want %q", out, want)
	}
}
