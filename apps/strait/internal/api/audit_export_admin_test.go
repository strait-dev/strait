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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Error(t, err)
	assert.Contains(t,
		err.Error(), "admin")
}

func TestUpdateAuditExportCap_RejectsCrossTenant(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	// Admin authenticated to proj-a attempts to mutate proj-b via path param.
	// Cross-tenant must surface as 404 (not 403) to avoid leaking which
	// projects exist when scanned for admin mistakes.
	in := &UpdateAuditExportCapInput{ID: "proj-b"}
	in.Body.RowCap = 1
	_, err := srv.handleUpdateAuditExportCap(adminCtx("proj-a"), in)
	require.Error(t, err)
	assert.Contains(t,
		err.Error(), "not found")
}

func TestUpdateAuditExportCap_RejectsNegative(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{
		SetAuditExportRowCapFunc: func(_ context.Context, _ string, _ int64) error {
			assert.Fail(t,

				"Set must not be called when input is invalid")
			return nil
		},
	}, nil, nil)

	in := &UpdateAuditExportCapInput{ID: "proj-a"}
	in.Body.RowCap = -1
	_, err := srv.handleUpdateAuditExportCap(adminCtx("proj-a"), in)
	require.Error(t, err)
	assert.Contains(t,
		err.Error(), ">= 0")
}

func TestUpdateAuditExportCap_PersistsAndEmitsAudit(t *testing.T) {
	t.Parallel()

	var (
		mu               sync.Mutex
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
	require.NoError(t, err)
	require.False(t, out == nil ||
		out.Body.ProjectID !=
			"proj-a" ||
		out.Body.RowCap !=

			500)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(
		t, "proj-a", storedCapProject,
	)
	assert.EqualValues(t, 500, storedCap)
	require.True(
		t, selfAuditHit)
	assert.InDelta(t, 42, seenOldCap, 1e-9)
	assert.InDelta(t, 500, seenNewCap, 1e-9)
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
		require.Failf(t, "test failure",

			"unexpected error: %v", err)
	}
	require.EqualValues(t, 0, storedCap.
		Load())

	// After re-inheriting, resolveExportRowCap returns the config default.
	effective := srv.resolveExportRowCap(context.Background(), "proj-a")
	assert.EqualValues(t, 7777, effective)
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
			for range seeded {
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
	require.NoError(t, err)
	require.True(
		t, capped)
	assert.Equal(
		t, int(cap), exported,
	)

	body := buf.String()
	assert.Contains(t,
		body, `"_capped":true`)
	assert.Equal(
		t, cap, streamedCount.
			Load(),
	)

	// StreamAuditEvents short-circuits via errExportCapReached after cap rows.
}
