package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	assert.EqualValues(t, 365, out.Body.
		Days)
	assert.True(t,
		out.Body.InheritedFromDefault,
	)
	assert.Equal(
		t, "proj-a", out.
			Body.ProjectID,
	)

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
	require.NoError(t, err)
	assert.EqualValues(t, 30, out.Body.Days)
	assert.False(
		t, out.Body.InheritedFromDefault,
	)

}

func TestGetAuditRetention_RequiresAdmin(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	_, err := srv.handleGetAuditRetention(nonAdminCtx("proj-a"), &GetAuditRetentionInput{ID: "proj-a"})
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "admin"))

}

func TestGetAuditRetention_RejectsCrossTenant(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	_, err := srv.handleGetAuditRetention(adminCtx("proj-a"), &GetAuditRetentionInput{ID: "proj-b"})
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "not found"))

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
	require.NoError(t, err)
	require.False(t, out == nil ||
		out.Body.
			ProjectID !=
			"proj-a" ||
		out.Body.
			Days != 30)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(
		t, "proj-a", storedProject,
	)
	assert.EqualValues(t, 30, storedDays)
	require.True(
		t, selfAuditHit)
	assert.EqualValues(t, 90, seenOldDays)
	assert.EqualValues(t, 30, seenNewDays)

}

func TestPutAuditRetention_RejectsNegative(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		SetAuditRetentionDaysFunc: func(_ context.Context, _ string, _ int) error {
			assert.Fail(t,

				"Set must not be called when input is invalid")
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	in := &UpdateAuditRetentionInput{ID: "proj-a"}
	in.Body.Days = -1
	_, err := srv.handleSetAuditRetention(adminCtx("proj-a"), in)
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), ">= 0"))

}

func TestPutAuditRetention_RejectsOverflowDays(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		SetAuditRetentionDaysFunc: func(_ context.Context, _ string, _ int) error {
			assert.Fail(t,

				"Set must not be called when input exceeds max retention")
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	in := &UpdateAuditRetentionInput{ID: "proj-a"}
	in.Body.Days = domain.MaxAuditRetentionDays + 1
	_, err := srv.handleSetAuditRetention(adminCtx("proj-a"), in)
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "maximum"))

}

func TestPutAuditRetention_AuditFailureFailsRequest(t *testing.T) {
	t.Parallel()

	var setCalls atomic.Int64
	ms := &APIStoreMock{
		GetAuditRetentionDaysFunc: func(_ context.Context, _ string) (int, bool, error) {
			return 90, true, nil
		},
		SetAuditRetentionDaysFunc: func(_ context.Context, projectID string, days int) error {
			require.False(t, projectID !=
				"proj-a" ||
				days != 30,
			)

			setCalls.Add(1)
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			require.Equal(t, domain.AuditActionRetentionUpdated,

				ev.Action,
			)

			return errors.New("audit unavailable")
		},
	}
	srv := newTestServerWithRetentionConfig(t, ms, 365)

	in := &UpdateAuditRetentionInput{ID: "proj-a"}
	in.Body.Days = 30
	_, err := srv.handleSetAuditRetention(adminCtx("proj-a"), in)
	require.True(
		t, isHumaStatusError(err, 500))
	require.EqualValues(t, 1, setCalls.Load())

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
		require.Failf(t, "test failure",

			"unexpected PUT error: %v", err)
	}
	require.EqualValues(t, 0, storedDays.
		Load())

	out, err := srv.handleGetAuditRetention(adminCtx("proj-a"), &GetAuditRetentionInput{ID: "proj-a"})
	require.NoError(t, err)
	assert.EqualValues(t, 0, out.Body.Days)
	assert.False(
		t, out.Body.InheritedFromDefault,
	)

}

func TestPutAuditRetention_RequiresAdmin(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	in := &UpdateAuditRetentionInput{ID: "proj-a"}
	in.Body.Days = 30
	_, err := srv.handleSetAuditRetention(nonAdminCtx("proj-a"), in)
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "admin"))

}

func TestPutAuditRetention_RejectsCrossTenant(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	in := &UpdateAuditRetentionInput{ID: "proj-b"}
	in.Body.Days = 30
	_, err := srv.handleSetAuditRetention(adminCtx("proj-a"), in)
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "not found"))

}
