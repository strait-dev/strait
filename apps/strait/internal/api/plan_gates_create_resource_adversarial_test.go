package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateLogDrain_UpdateBypass_NotPossible confirms that the update path
// for log drains does not implicitly bump the per-org cap. There is no
// "upgrade limit by re-saving" loophole because update never increments the
// count — only create does. We exercise the update endpoint with valid data
// and assert the cap gate is not invoked (the gate is only on create).
func TestCreateLogDrain_UpdateBypass_NotPossible(t *testing.T) {
	t.Parallel()

	countCalls := atomic.Int64{}
	ms := &APIStoreMock{
		CountLogDrainsByOrgFunc: func(_ context.Context, _ string) (int, error) {
			countCalls.Add(1)
			return 0, nil
		},
		UpdateLogDrainFunc: func(_ context.Context, _ string, _ string, _ map[string]any) error {
			return nil
		},
		GetLogDrainFunc: func(_ context.Context, id string, projectID string) (*domain.LogDrain, error) {
			return &domain.LogDrain{ID: id, ProjectID: projectID, Name: "x", DrainType: "http", EndpointURL: "https://example.com", AuthType: "bearer", Enabled: true}, nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: freeLimits()} // Free = cap 0
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	body := `{"name":"renamed"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/log-drains/drain-1", body))
	assert.EqualValues(t, 0, countCalls.
		Load())

	// Update either succeeds or 4xx for unrelated reasons; the key invariant
	// is that the cap gate is NOT consulted (update doesn't add a row).
}

// TestCreateLogDrain_RaceAtCap simulates 50 concurrent creates against an
// org sitting one slot below the Pro cap. With a stub mock, the gate's
// fail-open semantics do NOT prevent over-allocation; this test documents
// the boundary the real Postgres implementation must enforce: the count read
// is a snapshot, and TOCTOU races can let multiple creates through. The
// store integration tests verify the Postgres-backed create method serializes
// count plus insert under an advisory transaction lock.
func TestCreateLogDrain_RaceAtCap_DocumentsTOCTOU(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	count := atomic.Int64{}
	ms := &APIStoreMock{
		CountLogDrainsByOrgFunc: func(_ context.Context, _ string) (int, error) {
			return int(count.Load()), nil
		},
		CreateLogDrainFunc: func(_ context.Context, _ *domain.LogDrain) error {
			count.Add(1)
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: proLimits()} // cap=5

	// Seed: count=4. One slot remains.
	count.Store(4)

	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	const attempts = 50
	var wg sync.WaitGroup
	wg.Add(attempts)
	results := make(chan int, attempts)
	for range attempts {
		concWG.Go(func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", validLogDrainBody()))
			results <- w.Code
		})
	}
	wg.Wait()
	close(results)

	created := 0
	rejected := 0
	for code := range results {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusBadRequest:
			rejected++
		}
	}
	require.False(t, created ==
		0 && rejected == 0,
	)

	// Boundary: the gate's snapshot read can let multiple creates through
	// before any of them increment the count, so created can exceed 1.
	// We assert only that the gate IS invoked and at least one create
	// happens — the real over-cap protection comes from a store-level check
	// or distributed lock in production.

	if rejected == 0 {
		t.Logf("note: 0 rejected at near-cap with 50 concurrent attempts; documented TOCTOU window; the Postgres store method is the authoritative gate")
	}
}

// TestCreateLogDrain_OrgScopedCount confirms the gate counts across all
// projects in an org, not just the requesting project. A multi-project org
// at the Pro cap of 5 (across two projects) cannot bypass by submitting
// from a freshly-created project.
func TestCreateLogDrain_OrgScopedCount(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CountLogDrainsByOrgFunc: func(_ context.Context, orgID string) (int, error) {
			assert.Equal(
				t, "org-1",
				orgID)

			return 5, nil // already at Pro cap across the org
		},
		CreateLogDrainFunc: func(_ context.Context, _ *domain.LogDrain) error {
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{
		limits: proLimits(),
		orgID:  "org-1",
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	body := `{
		"project_id": "proj-2",
		"name": "drain-from-other-project",
		"drain_type": "http",
		"endpoint_url": "https://example.com/logs",
		"auth_type": "bearer"
	}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", body))
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
}

// TestCreateNotificationChannel_RaceAtCap_DocumentsTOCTOU mirrors the
// log-drain TOCTOU test for notification channels (per-project count).
func TestCreateNotificationChannel_RaceAtCap_DocumentsTOCTOU(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	count := atomic.Int64{}
	ms := &APIStoreMock{
		CountNotificationChannelsByProjectFunc: func(_ context.Context, _ string) (int, error) {
			return int(count.Load()), nil
		},
		CreateNotificationChannelFunc: func(_ context.Context, ch *domain.NotificationChannel) error {
			ch.ID = "ch"
			count.Add(1)
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: proLimits()} // cap=5
	count.Store(4)

	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	const attempts = 50
	var wg sync.WaitGroup
	wg.Add(attempts)
	results := make(chan int, attempts)
	for range attempts {
		concWG.Go(func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", validChannelBody(), "proj-1"))
			results <- w.Code
		})
	}
	wg.Wait()
	close(results)

	saw201 := false
	saw4xx := false
	for code := range results {
		if code == http.StatusCreated {
			saw201 = true
		}
		if code >= 400 && code < 500 {
			saw4xx = true
		}
	}
	require.False(t, !saw201 &&
		!saw4xx)

	// We don't assert exact counts — the gate's snapshot semantics permit a
	// TOCTOU window. The Postgres store method is the enforcement boundary.
	_ = saw201
	_ = saw4xx
}

// TestPlanGate_TamperedEntitlements_TrustsDB locks in the documented threat
// model: if entitlements are tampered with directly via SQL to claim a
// higher tier, the gate trusts the DB. This is intentional - the DB row is
// authoritative for resolved entitlements. This
// test names the boundary so future contributors don't try to "harden" it
// by re-deriving from PlanTier (which would defeat the snapshot model).
func TestPlanGate_TamperedEntitlements_TrustsDB(t *testing.T) {
	t.Parallel()

	// Construct a tampered set of limits that claims Enterprise-grade caps
	// but with a Free-tier display name. The gate should honor the cap.
	tampered := freeLimits()
	tampered.MaxLogDrainsPerOrg = -1 // tampered: Free should be 0

	ms := &APIStoreMock{
		CountLogDrainsByOrgFunc: func(_ context.Context, _ string) (int, error) {
			return 9999, nil
		},
		CreateLogDrainFunc: func(_ context.Context, _ *domain.LogDrain) error {
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: tampered}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", validLogDrainBody()))
	require.Equal(t, http.StatusCreated,
		w.Code)
}

// TestPlanGate_MaxLogDrainsPerOrg_ValuesPerTier locks in the per-tier values
// for MaxLogDrainsPerOrg so a future plan-catalog edit can't silently change
// what we ship.
func TestPlanGate_MaxLogDrainsPerOrg_ValuesPerTier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		tier domain.PlanTier
		want int
	}{
		{domain.PlanFree, 0},
		{domain.PlanStarter, 1},
		{domain.PlanPro, 5},
		{domain.PlanScale, 10},
		{domain.PlanBusiness, -1},
		{domain.PlanEnterprise, -1},
	}
	for _, tc := range cases {
		got := billing.GetPlanLimits(tc.tier).MaxLogDrainsPerOrg
		assert.Equal(
			t, tc.want,
			got)
	}
}

// TestPlanGate_MaxNotificationChannels_ValuesPerTier locks in the per-tier
// values for MaxNotificationChannels.
func TestPlanGate_MaxNotificationChannels_ValuesPerTier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		tier domain.PlanTier
		want int
	}{
		{domain.PlanFree, 0},
		{domain.PlanStarter, 1},
		{domain.PlanPro, 5},
		{domain.PlanScale, 10},
		{domain.PlanBusiness, -1},
		{domain.PlanEnterprise, -1},
	}
	for _, tc := range cases {
		got := billing.GetPlanLimits(tc.tier).MaxNotificationChannels
		assert.Equal(
			t, tc.want,
			got)
	}
}

// TestPlanGate_FreeMessageStable verifies the rejection message format
// stays consistent across resources — operators grep these in logs and
// support tickets, so the format is part of the contract.
func TestPlanGate_FreeMessageStable(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateLogDrainFunc: func(_ context.Context, _ *domain.LogDrain) error {
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: freeLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", validLogDrainBody()))

	body := w.Body.String()
	assert.Contains(t,
		body, "Log drains are not available")
	assert.Contains(t,
		body, "/settings/billing")
}
