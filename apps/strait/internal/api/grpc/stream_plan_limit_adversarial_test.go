package grpc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

// TestStreamGating_RaceAtCap simulates many simultaneous connect attempts
// against an org that is already at the connection cap. The gating decision
// must reject every one of them — not let one through due to a race window.
//
// Note: this test exercises the gatingResult helper and the registry; the real
// stream.go uses CountByOrg under the registry's RLock, then falls through to
// CheckWorkerConnectionLimit which is a stateless arithmetic check. There is
// a TOCTOU window between counting and registering, but that window is bounded
// by the maxStreamsPerProject quota also enforced at Register time. This test
// proves the explicit plan gate rejects concurrent attempts when the count is
// already at limit.
func TestStreamGating_RaceAtCap(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	enforcer := &stubPlanLimitEnforcer{
		orgIDForProject: map[string]string{"proj-a": "org-1"},
		limit:           5,
	}
	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 1000
	r.maxStreamsPerAPIKey = 1000
	for i := range 5 {
		w := makeWorker(fmt.Sprintf("seed-%d", i), "proj-a", fmt.Sprintf("key-seed-%d", i), []string{"q"}, 1)
		w.OrgID = "org-1"
		require.NoError(t, r.
			Register(w))

	}

	const attempts = 50
	var blocked atomic.Int64
	var allowed atomic.Int64
	var wg sync.WaitGroup
	wg.Add(attempts)
	for range attempts {
		concWG.Go(func() {
			defer wg.Done()
			if _, b := gatingResult(context.Background(), domain.EditionCloud, enforcer, r, "proj-a"); b {
				blocked.Add(1)
			} else {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 0, allowed.
		Load())
	require.EqualValues(
		t, attempts,
		blocked.
			Load())

}

// TestStreamGating_ReconnectStorm verifies that when N existing connections
// drop and immediately reconnect, an outside attacker connecting in the gap
// cannot exploit a stale count to bypass the cap. Concretely: after the drop
// + register cycle, gatingResult for a new worker on a saturated org must
// still reject.
func TestStreamGating_ReconnectStorm(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	enforcer := &stubPlanLimitEnforcer{
		orgIDForProject: map[string]string{"proj-a": "org-1"},
		limit:           5,
	}
	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 1000
	r.maxStreamsPerAPIKey = 1000

	// Seed 5 active connections.
	tokens := make([]uint64, 5)
	for i := range 5 {
		w := makeWorker(fmt.Sprintf("w-%d", i), "proj-a", fmt.Sprintf("k-%d", i), []string{"q"}, 1)
		w.OrgID = "org-1"
		require.NoError(t, r.
			Register(w))

		tokens[i] = w.regToken
	}

	// Storm: each existing worker simultaneously drops + reconnects.
	var wg sync.WaitGroup
	wg.Add(5)
	for i := range 5 {
		{
			i := i
			concWG.Go(func() {
				defer wg.Done()
				r.Deregister(fmt.Sprintf("w-%d", i), tokens[i])
				w := makeWorker(fmt.Sprintf("w-%d", i), "proj-a", fmt.Sprintf("k-%d", i), []string{"q"}, 1)
				w.OrgID = "org-1"
				_ = r.Register(w)
			})
		}
	}

	// While the storm is in flight, an attacker connect attempt: must reject
	// or be allowed only transiently (count drops to 4 between deregister and
	// re-register). The strict invariant we test below is the *post-storm*
	// state: once the storm settles, the count is exactly 5 and a new
	// connect is rejected.
	wg.Wait()
	require.EqualValues(t, 5, r.
		CountByOrg("org-1"))

	if _, blocked := gatingResult(context.Background(), domain.EditionCloud, enforcer, r, "proj-a"); !blocked {
		require.Fail(t,

			"post-storm new connect at cap was allowed; want blocked")
	}
}

// TestStreamGating_OrgScopingCannotBeSpoofedViaProjectID verifies that the
// gate trusts the enforcer's GetActiveProjectOrgID, not any caller-supplied
// org metadata. A worker connecting with project P is gated against P's
// resolved org, period.
//
// In stream.go the project is derived from the authenticated API key, so the
// caller cannot spoof it. This test locks in the contract: even if a
// hypothetical caller passed a different projectID, the enforcer's mapping
// is authoritative and the count is taken against the resolved org.
func TestStreamGating_OrgScopingCannotBeSpoofedViaProjectID(t *testing.T) {
	t.Parallel()

	enforcer := &stubPlanLimitEnforcer{
		orgIDForProject: map[string]string{
			"proj-saturated": "org-saturated",
			"proj-spacious":  "org-spacious",
		},
		limit: 1,
	}
	r := NewConnectionRegistry()

	// Saturated org: 1/1.
	w := makeWorker("seed", "proj-saturated", "k-seed", []string{"q"}, 1)
	w.OrgID = "org-saturated"
	require.NoError(t, r.
		Register(w))

	// Connect attempts for saturated org → blocked.
	if orgID, blocked := gatingResult(context.Background(), domain.EditionCloud, enforcer, r, "proj-saturated"); !blocked || orgID != "org-saturated" {
		require.Failf(t, "test failure",

			"saturated org gating: orgID=%q blocked=%v, want org-saturated true", orgID, blocked)
	}
	// Connect attempts for spacious org → allowed.
	if orgID, blocked := gatingResult(context.Background(), domain.EditionCloud, enforcer, r, "proj-spacious"); blocked || orgID != "org-spacious" {
		require.Failf(t, "test failure",

			"spacious org gating: orgID=%q blocked=%v, want org-spacious false", orgID, blocked)
	}
}

// TestStreamGating_EmptyOrgIDCannotBeUsedToBypass verifies that the registry
// does NOT count workers with empty OrgID toward any org's quota. A
// misbehaving worker that somehow registered with an empty OrgID cannot pad
// another org's count, but it also cannot be evicted by another org's
// connect storm. Cloud registration fails closed before registration when org
// resolution returns empty; this test covers already-registered empty-org rows.
func TestStreamGating_EmptyOrgIDCannotBeUsedToBypass(t *testing.T) {
	t.Parallel()

	enforcer := &stubPlanLimitEnforcer{
		orgIDForProject: map[string]string{"proj-a": "org-1"},
		limit:           1,
	}
	r := NewConnectionRegistry()

	// One worker registered with empty OrgID (should not count for org-1).
	wEmpty := makeWorker("ghost", "proj-a", "k-ghost", []string{"q"}, 1)
	require.NoError(t, r.
		Register(wEmpty))
	require.EqualValues(t, 0, r.
		CountByOrg("org-1"))

	// New connect for org-1: count is 0/1 → allowed (the ghost did not pad
	// the org's count). This confirms the gate evaluates the real org-1 set.
	if _, blocked := gatingResult(context.Background(), domain.EditionCloud, enforcer, r, "proj-a"); blocked {
		require.Fail(t,

			"empty-org worker should not pad org-1 count; gate must allow")
	}
}

// TestStreamGating_DowngradeMidSession verifies the documented behavior on
// downgrade: existing connections are not evicted, but a new connect after
// the cap drops below the active count is rejected.
func TestStreamGating_DowngradeMidSession(t *testing.T) {
	t.Parallel()

	enforcer := &stubPlanLimitEnforcer{
		orgIDForProject: map[string]string{"proj-a": "org-1"},
		limit:           10, // initially Pro
	}
	r := NewConnectionRegistry()

	// Seed 5 active connections under the Pro cap.
	for i := range 5 {
		w := makeWorker(fmt.Sprintf("w-%d", i), "proj-a", fmt.Sprintf("k-%d", i), []string{"q"}, 1)
		w.OrgID = "org-1"
		require.NoError(t, r.
			Register(w))

	}
	require.EqualValues(t, 5, r.
		CountByOrg("org-1"))

	// Downgrade: cap drops to 1. Existing connections survive (we don't evict).
	enforcer.mu.Lock()
	enforcer.limit = 1
	enforcer.mu.Unlock()
	require.EqualValues(t, 5, r.
		CountByOrg("org-1"))

	// Existing connections still in the registry.

	// New connect: 5 active, cap 1 → blocked.
	if _, blocked := gatingResult(context.Background(), domain.EditionCloud, enforcer, r, "proj-a"); !blocked {
		require.Fail(t,

			"post-downgrade new connect was allowed; want blocked")
	}
}
