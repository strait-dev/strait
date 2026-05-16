package api

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"
)

// newTestServerWithRetentionConfig wires a Server with a non-zero
// AuditRetentionDefaultDays so tests can assert the inherited-default
// path without having to reach into a package-level constant.
func newTestServerWithRetentionConfig(t *testing.T, s APIStore, defaultDays int) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:            "test-secret-value",
		MaxBulkTriggerItems:       500,
		JWTSigningKey:             testJWTSigningKey,
		AuditRetentionDefaultDays: defaultDays,
	}
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   s,
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestGetAuditRetention_DefaultWhenUnset(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetAuditRetentionDaysFunc: func(_ context.Context, _ string) (int, bool, error) {
			// No override row present.
			return 0, false, nil
		},
	}
	srv := newTestServerWithRetentionConfig(t, ms, 365)

	out, err := srv.handleGetAuditRetention(adminCtx("proj-a"), &GetAuditRetentionInput{ID: "proj-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Body.Days != 365 {
		t.Errorf("days = %d, want 365", out.Body.Days)
	}
	if !out.Body.InheritedFromDefault {
		t.Errorf("expected inherited_from_default=true, got false")
	}
	if out.Body.ProjectID != "proj-a" {
		t.Errorf("project_id = %q, want proj-a", out.Body.ProjectID)
	}
}

func TestGetAuditRetention_ProjectOverride(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetAuditRetentionDaysFunc: func(_ context.Context, _ string) (int, bool, error) {
			return 30, true, nil
		},
	}
	srv := newTestServerWithRetentionConfig(t, ms, 365)

	out, err := srv.handleGetAuditRetention(adminCtx("proj-a"), &GetAuditRetentionInput{ID: "proj-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Body.Days != 30 {
		t.Errorf("days = %d, want 30", out.Body.Days)
	}
	if out.Body.InheritedFromDefault {
		t.Errorf("expected inherited_from_default=false, got true")
	}
}

func TestGetAuditRetention_RequiresAdmin(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	_, err := srv.handleGetAuditRetention(nonAdminCtx("proj-a"), &GetAuditRetentionInput{ID: "proj-a"})
	if err == nil {
		t.Fatal("expected 403 for non-admin caller, got nil")
	}
	if !strings.Contains(err.Error(), "admin") {
		t.Errorf("expected admin-required error, got %v", err)
	}
}

func TestGetAuditRetention_RejectsCrossTenant(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	_, err := srv.handleGetAuditRetention(adminCtx("proj-a"), &GetAuditRetentionInput{ID: "proj-b"})
	if err == nil {
		t.Fatal("expected 404 for cross-tenant request, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestPutAuditRetention_PersistsAndEmitsAudit(t *testing.T) {
	t.Parallel()

	var (
		mu            sync.Mutex
		storedDays    int
		storedProject string
		selfAuditHit  bool
		seenOldDays   float64
		seenNewDays   float64
	)

	ms := &APIStoreMock{
		GetAuditRetentionDaysFunc: func(_ context.Context, projectID string) (int, bool, error) {
			// Prior explicit override of 90 days on proj-a.
			if projectID == "proj-a" {
				return 90, true, nil
			}
			return 0, false, nil
		},
		SetAuditRetentionDaysFunc: func(_ context.Context, projectID string, days int) error {
			mu.Lock()
			defer mu.Unlock()
			storedProject = projectID
			storedDays = days
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			if ev.Action == domain.AuditActionRetentionUpdated {
				selfAuditHit = true
				var d map[string]any
				_ = json.Unmarshal(ev.Details, &d)
				if v, ok := d["old_days"].(float64); ok {
					seenOldDays = v
				}
				if v, ok := d["new_days"].(float64); ok {
					seenNewDays = v
				}
			}
			return nil
		},
	}
	srv := newTestServerWithRetentionConfig(t, ms, 365)

	in := &UpdateAuditRetentionInput{ID: "proj-a"}
	in.Body.Days = 30
	out, err := srv.handleSetAuditRetention(adminCtx("proj-a"), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.Body.ProjectID != "proj-a" || out.Body.Days != 30 {
		t.Fatalf("unexpected response: %+v", out)
	}

	mu.Lock()
	defer mu.Unlock()
	if storedProject != "proj-a" {
		t.Errorf("Set called with project %q, want proj-a", storedProject)
	}
	if storedDays != 30 {
		t.Errorf("persisted days = %d, want 30", storedDays)
	}
	if !selfAuditHit {
		t.Fatal("audit.retention_updated self-audit was not emitted")
	}
	if seenOldDays != 90 {
		t.Errorf("self-audit old_days = %v, want 90", seenOldDays)
	}
	if seenNewDays != 30 {
		t.Errorf("self-audit new_days = %v, want 30", seenNewDays)
	}
}

func TestPutAuditRetention_RejectsNegative(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		SetAuditRetentionDaysFunc: func(_ context.Context, _ string, _ int) error {
			t.Error("Set must not be called when input is invalid")
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	in := &UpdateAuditRetentionInput{ID: "proj-a"}
	in.Body.Days = -1
	_, err := srv.handleSetAuditRetention(adminCtx("proj-a"), in)
	if err == nil {
		t.Fatal("expected 400 for negative days, got nil")
	}
	if !strings.Contains(err.Error(), ">= 0") {
		t.Errorf("expected >=0 error message, got %v", err)
	}
}

func TestPutAuditRetention_RejectsOverflowDays(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		SetAuditRetentionDaysFunc: func(_ context.Context, _ string, _ int) error {
			t.Error("Set must not be called when input exceeds max retention")
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	in := &UpdateAuditRetentionInput{ID: "proj-a"}
	in.Body.Days = domain.MaxAuditRetentionDays + 1
	_, err := srv.handleSetAuditRetention(adminCtx("proj-a"), in)
	if err == nil {
		t.Fatal("expected 400 for overflow days, got nil")
	}
	if !strings.Contains(err.Error(), "maximum") {
		t.Errorf("expected maximum error message, got %v", err)
	}
}

func TestPutAuditRetention_ZeroDisablesTrim(t *testing.T) {
	t.Parallel()

	var storedDays atomic.Int64
	storedDays.Store(-1)

	ms := &APIStoreMock{
		GetAuditRetentionDaysFunc: func(_ context.Context, _ string) (int, bool, error) {
			// After the PUT persists 0, the subsequent GET must treat 0
			// as an explicit override — NOT as "absent". The bool flag
			// is the source of truth for presence; a raw 0 from the
			// store is meaningful and distinct from absence.
			switch storedDays.Load() {
			case 0:
				return 0, true, nil
			case -1:
				return 0, false, nil
			default:
				return int(storedDays.Load()), true, nil
			}
		},
		SetAuditRetentionDaysFunc: func(_ context.Context, _ string, days int) error {
			storedDays.Store(int64(days))
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}
	srv := newTestServerWithRetentionConfig(t, ms, 365)

	in := &UpdateAuditRetentionInput{ID: "proj-a"}
	in.Body.Days = 0
	if _, err := srv.handleSetAuditRetention(adminCtx("proj-a"), in); err != nil {
		t.Fatalf("unexpected PUT error: %v", err)
	}
	if storedDays.Load() != 0 {
		t.Fatalf("days not persisted as 0, got %d", storedDays.Load())
	}

	out, err := srv.handleGetAuditRetention(adminCtx("proj-a"), &GetAuditRetentionInput{ID: "proj-a"})
	if err != nil {
		t.Fatalf("unexpected GET error: %v", err)
	}
	if out.Body.Days != 0 {
		t.Errorf("GET days = %d, want 0 (explicit disable)", out.Body.Days)
	}
	if out.Body.InheritedFromDefault {
		t.Errorf("GET inherited_from_default = true, want false (0 is an explicit override)")
	}
}

func TestPutAuditRetention_RequiresAdmin(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	in := &UpdateAuditRetentionInput{ID: "proj-a"}
	in.Body.Days = 30
	_, err := srv.handleSetAuditRetention(nonAdminCtx("proj-a"), in)
	if err == nil {
		t.Fatal("expected 403 for non-admin caller, got nil")
	}
	if !strings.Contains(err.Error(), "admin") {
		t.Errorf("expected admin-required error, got %v", err)
	}
}

func TestPutAuditRetention_RejectsCrossTenant(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	in := &UpdateAuditRetentionInput{ID: "proj-b"}
	in.Body.Days = 30
	_, err := srv.handleSetAuditRetention(adminCtx("proj-a"), in)
	if err == nil {
		t.Fatal("expected 404 for cross-tenant request, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got %v", err)
	}
}
