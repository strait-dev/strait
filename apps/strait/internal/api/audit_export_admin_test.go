package api

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
)

func newTestServerWithCapConfig(t *testing.T, s APIStore, defaultCap int64) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:           "test-secret-value",
		MaxBulkTriggerItems:      500,
		JWTSigningKey:            testJWTSigningKey,
		AuditExportRowCapDefault: defaultCap,
	}
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   s,
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestUpdateAuditExportCap_RequiresAdmin(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	in := &UpdateAuditExportCapInput{ID: "proj-a"}
	in.Body.RowCap = 500
	_, err := srv.handleUpdateAuditExportCap(nonAdminCtx("proj-a"), in)
	if err == nil {
		t.Fatal("expected 403 for non-admin caller, got nil")
	}
	if !strings.Contains(err.Error(), "admin") {
		t.Errorf("expected admin-required error, got %v", err)
	}
}

func TestUpdateAuditExportCap_RejectsCrossTenant(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	// Admin authenticated to proj-a attempts to mutate proj-b via path param.
	in := &UpdateAuditExportCapInput{ID: "proj-b"}
	in.Body.RowCap = 1
	_, err := srv.handleUpdateAuditExportCap(adminCtx("proj-a"), in)
	if err == nil {
		t.Fatal("expected 403 for cross-tenant request, got nil")
	}
	if !strings.Contains(err.Error(), "project") {
		t.Errorf("expected path/project mismatch error, got %v", err)
	}
}

func TestUpdateAuditExportCap_RejectsNegative(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{
		SetAuditExportRowCapFunc: func(_ context.Context, _ string, _ int64) error {
			t.Error("Set must not be called when input is invalid")
			return nil
		},
	}, nil, nil)

	in := &UpdateAuditExportCapInput{ID: "proj-a"}
	in.Body.RowCap = -1
	_, err := srv.handleUpdateAuditExportCap(adminCtx("proj-a"), in)
	if err == nil {
		t.Fatal("expected 400 for negative cap, got nil")
	}
	if !strings.Contains(err.Error(), ">= 0") {
		t.Errorf("expected >=0 error message, got %v", err)
	}
}

func TestUpdateAuditExportCap_PersistsAndEmitsAudit(t *testing.T) {
	t.Parallel()

	var (
		mu               sync.Mutex
		storedProjectID  string
		storedCap        int64
		storedCapProject string
		selfAuditHit     bool
		seenOldCap       float64
		seenNewCap       float64
	)

	ms := &APIStoreMock{
		GetAuditExportRowCapFunc: func(_ context.Context, projectID string) (int64, error) {
			// Simulate a prior cap of 42 on proj-a.
			if projectID == "proj-a" {
				return 42, nil
			}
			return 0, nil
		},
		SetAuditExportRowCapFunc: func(_ context.Context, projectID string, cap int64) error {
			mu.Lock()
			defer mu.Unlock()
			storedCapProject = projectID
			storedProjectID = projectID
			storedCap = cap
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			if ev.Action == domain.AuditActionExportCapUpdated {
				selfAuditHit = true
				var d map[string]any
				_ = json.Unmarshal(ev.Details, &d)
				if v, ok := d["old_cap"].(float64); ok {
					seenOldCap = v
				}
				if v, ok := d["new_cap"].(float64); ok {
					seenNewCap = v
				}
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	in := &UpdateAuditExportCapInput{ID: "proj-a"}
	in.Body.RowCap = 500
	out, err := srv.handleUpdateAuditExportCap(adminCtx("proj-a"), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.Body.ProjectID != "proj-a" || out.Body.RowCap != 500 {
		t.Fatalf("unexpected response: %+v", out)
	}

	mu.Lock()
	defer mu.Unlock()
	if storedCapProject != "proj-a" {
		t.Errorf("Set called with project %q, want proj-a", storedProjectID)
	}
	if storedCap != 500 {
		t.Errorf("persisted cap = %d, want 500", storedCap)
	}
	if !selfAuditHit {
		t.Fatal("audit.export_cap_updated self-audit was not emitted")
	}
	if seenOldCap != 42 {
		t.Errorf("self-audit old_cap = %v, want 42", seenOldCap)
	}
	if seenNewCap != 500 {
		t.Errorf("self-audit new_cap = %v, want 500", seenNewCap)
	}
}

func TestUpdateAuditExportCap_ZeroReinheritsDefault(t *testing.T) {
	t.Parallel()

	var storedCap atomic.Int64
	storedCap.Store(-1)

	ms := &APIStoreMock{
		GetAuditExportRowCapFunc: func(_ context.Context, projectID string) (int64, error) {
			// After the reset, subsequent GetAuditExportRowCap returns 0
			// (i.e. no override). resolveExportRowCap should then fall
			// through to the configured server default.
			if storedCap.Load() == 0 {
				return 0, nil
			}
			return 500, nil
		},
		SetAuditExportRowCapFunc: func(_ context.Context, _ string, cap int64) error {
			storedCap.Store(cap)
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}

	// Default-aware server so the fallback can be asserted.
	srv := newTestServerWithCapConfig(t, ms, 7777)

	in := &UpdateAuditExportCapInput{ID: "proj-a"}
	in.Body.RowCap = 0
	if _, err := srv.handleUpdateAuditExportCap(adminCtx("proj-a"), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if storedCap.Load() != 0 {
		t.Fatalf("cap was not persisted as 0, got %d", storedCap.Load())
	}

	// After re-inheriting, resolveExportRowCap returns the config default.
	effective := srv.resolveExportRowCap(context.Background(), "proj-a")
	if effective != 7777 {
		t.Errorf("resolveExportRowCap = %d, want config default 7777", effective)
	}
}

func TestExportAuditEvents_PerProjectCap(t *testing.T) {
	t.Parallel()

	// Seed 100 events; per-project cap = 10. Stream must terminate early
	// and emit the _capped sentinel.
	const seeded = 100
	const cap int64 = 10

	var streamedCount atomic.Int64
	ms := &APIStoreMock{
		GetAuditExportRowCapFunc: func(_ context.Context, _ string) (int64, error) { return cap, nil },
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			for i := 0; i < seeded; i++ {
				ev := &domain.AuditEvent{
					ID:        "ev",
					ProjectID: "proj-a",
					Action:    domain.AuditActionJobTriggered,
					CreatedAt: time.Now().UTC(),
					Details:   json.RawMessage(`{"run_id":"r"}`),
				}
				if err := fn(ev); err != nil {
					return err
				}
				streamedCount.Add(1)
			}
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}

	srv := newTestServerWithCapConfig(t, ms, 1_000_000)

	// Exercise the stream helper directly to avoid wiring the full HTTP
	// path. 0 flusher/canFlush: buffer to in-memory writer.
	var buf strings.Builder
	exported, capped, err := srv.streamAuditNDJSON(
		context.Background(), &buf, nil, false,
		"proj-a", "", "",
		time.Unix(0, 0), time.Now().Add(time.Hour),
		cap,
	)
	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}
	if !capped {
		t.Fatal("expected stream to cap, it did not")
	}
	if exported != int(cap) {
		t.Errorf("exported = %d, want %d", exported, cap)
	}
	body := buf.String()
	if !strings.Contains(body, `"_capped":true`) {
		t.Errorf("body missing _capped sentinel: %q", body)
	}
	// StreamAuditEvents short-circuits via errExportCapReached after cap rows.
	if got := streamedCount.Load(); got != cap {
		t.Errorf("stream delivered %d events, want %d before short-circuit", got, cap)
	}
}
