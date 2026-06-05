package grpc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/config"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// stubPlanLimitEnforcer is a hand-rolled implementation of planLimitEnforcer
// that records calls and returns canned values. We don't use a moq because
// the interface is defined in this package and only used at one call site.
type stubPlanLimitEnforcer struct {
	mu sync.Mutex

	// Configured behavior.
	orgIDForProject map[string]string
	orgLookupErr    error
	limit           int            // -1 means unlimited
	limitErrByOrg   map[string]int // optional override: org -> threshold

	// Recorded calls.
	checkCalls    []checkWorkerLimitCall
	orgLookupHits int
}

type checkWorkerLimitCall struct {
	OrgID         string
	CurrentActive int
}

func (s *stubPlanLimitEnforcer) GetActiveProjectOrgID(_ context.Context, projectID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orgLookupHits++
	if s.orgLookupErr != nil {
		return "", s.orgLookupErr
	}
	return s.orgIDForProject[projectID], nil
}

func (s *stubPlanLimitEnforcer) CheckWorkerConnectionLimit(_ context.Context, orgID string, currentActive int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkCalls = append(s.checkCalls, checkWorkerLimitCall{OrgID: orgID, CurrentActive: currentActive})

	threshold := s.limit
	if t, ok := s.limitErrByOrg[orgID]; ok {
		threshold = t
	}
	if threshold == -1 {
		return nil
	}
	if currentActive >= threshold {
		return fmt.Errorf("worker connections cap %d reached", threshold)
	}
	return nil
}

func (s *stubPlanLimitEnforcer) callsLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.checkCalls)
}

type releaseRecordingReservationEnforcer struct {
	stubPlanLimitEnforcer

	reserveErr           error
	reserveCalls         int
	lastReservationOrgID string
	lastReservationID    string
	lastReservationLease time.Duration

	releaseCalls atomic.Int64
}

func (r *releaseRecordingReservationEnforcer) ReserveWorkerConnection(_ context.Context, orgID, reservationID string, lease time.Duration) (func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reserveCalls++
	r.lastReservationOrgID = orgID
	r.lastReservationID = reservationID
	r.lastReservationLease = lease
	if r.reserveErr != nil {
		return nil, r.reserveErr
	}
	return func() {
		r.releaseCalls.Add(1)
	}, nil
}

func (r *releaseRecordingReservationEnforcer) RenewWorkerConnection(context.Context, string, string, time.Duration) error {
	return nil
}

func TestCheckPlanConnectionLimit_UsesDistributedReservation(t *testing.T) {
	t.Parallel()

	enforcer := &releaseRecordingReservationEnforcer{
		stubPlanLimitEnforcer: stubPlanLimitEnforcer{
			orgIDForProject: map[string]string{"proj-a": "org-1"},
			limit:           0,
		},
	}
	svc := &workerService{
		registry:        NewConnectionRegistry(),
		billingEnforcer: enforcer,
		cfg:             &config.Config{WorkerHeartbeatTimeout: 10 * time.Second},
	}

	orgID, release, err := svc.checkPlanConnectionLimit(context.Background(), "proj-a", "reservation-1")
	require.NoError(t, err)
	require.Equal(
		t, "org-1",
		orgID,
	)
	require.NotNil(t, release)
	require.Equal(t, 0, enforcer.
		callsLen(),
	)

	enforcer.mu.Lock()
	reserveCalls := enforcer.reserveCalls
	reservationOrgID := enforcer.lastReservationOrgID
	reservationID := enforcer.lastReservationID
	reservationLease := enforcer.lastReservationLease
	enforcer.mu.Unlock()
	require.Equal(t, 1, reserveCalls)
	require.False(
		t, reservationOrgID !=
			"org-1" ||
			reservationID !=
				"reservation-1",
	)
	require.Equal(
		t, 30*time.
			Second,
		reservationLease,
	)

	release()
	require.EqualValues(t, 1, enforcer.
		releaseCalls.
		Load())
}

func TestCheckPlanConnectionLimit_ReservationDenialIsResourceExhausted(t *testing.T) {
	t.Parallel()

	enforcer := &releaseRecordingReservationEnforcer{
		stubPlanLimitEnforcer: stubPlanLimitEnforcer{
			orgIDForProject: map[string]string{"proj-a": "org-1"},
			limit:           -1,
		},
		reserveErr: errors.New("worker connection cap reached"),
	}
	svc := &workerService{
		registry:        NewConnectionRegistry(),
		billingEnforcer: enforcer,
	}

	orgID, release, err := svc.checkPlanConnectionLimit(context.Background(), "proj-a", "reservation-1")
	require.Error(
		t, err)
	require.Equal(
		t, codes.ResourceExhausted,

		status.Code(
			err))
	require.Equal(
		t, "org-1",
		orgID,
	)
	require.NotNil(t, release)
	require.EqualValues(t, 0, enforcer.
		releaseCalls.
		Load())
	require.Equal(t, 0, enforcer.
		callsLen(),
	)
}

func TestRegisterWorkerStream_ReleasesReservationWhenRegistryRejects(t *testing.T) {
	t.Parallel()

	registry := NewConnectionRegistry()
	existing := makeWorker("worker-1", "proj-a", "key-old", []string{"default"}, 1)
	existing.OrgID = "org-1"
	require.NoError(t, registry.
		Register(existing))

	enforcer := &releaseRecordingReservationEnforcer{
		stubPlanLimitEnforcer: stubPlanLimitEnforcer{
			orgIDForProject: map[string]string{"proj-a": "org-1"},
			limit:           -1,
		},
	}
	svc := &workerService{
		registry:        registry,
		billingEnforcer: enforcer,
	}

	_, release, err := svc.registerWorkerStream(
		context.Background(),
		&domain.APIKey{ID: "key-new", ProjectID: "proj-a", EnvironmentID: "env-a"},
		"proj-a",
		&workerv1.WorkerRegistration{
			WorkerId:       "worker-1",
			Queues:         []string{"default"},
			SlotsTotal:     1,
			SlotsAvailable: 1,
		},
		"proj-a",
		"key-new",
	)
	require.Error(
		t, err)
	require.Nil(t, release)
	require.EqualValues(t, 1, enforcer.
		releaseCalls.
		Load())
	require.Equal(t, 1, registry.
		CountByOrg("org-1"))
}

// TestRegistry_CountByOrg verifies that CountByOrg only counts entries whose
// OrgID matches the supplied value, and treats empty as zero.
func TestRegistry_CountByOrg(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()

	w1 := makeWorker("w1", "proj-a", "key-1", []string{"q"}, 1)
	w1.OrgID = "org-1"
	w2 := makeWorker("w2", "proj-a", "key-2", []string{"q"}, 1)
	w2.OrgID = "org-1"
	w3 := makeWorker("w3", "proj-b", "key-3", []string{"q"}, 1)
	w3.OrgID = "org-2"
	w4 := makeWorker("w4", "proj-c", "key-4", []string{"q"}, 1)
	// Empty OrgID — must not count toward any org.

	for _, w := range []*ConnectedWorker{w1, w2, w3, w4} {
		require.NoError(t, r.Register(w))
	}
	assert.Equal(t, 2, r.CountByOrg("org-1"))
	assert.Equal(t, 1, r.CountByOrg("org-2"))
	assert.Equal(t, 0, r.CountByOrg("org-3"))
	assert.Equal(t, 0, r.CountByOrg(""))
}

// TestRegistry_CountByOrg_Concurrent exercises CountByOrg under concurrent
// register / deregister to surface any data race.
func TestRegistry_CountByOrg_Concurrent(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 1000
	r.maxStreamsPerAPIKey = 1000

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		{
			i := i
			concWG.Go(func() {
				defer wg.Done()
				w := makeWorker(fmt.Sprintf("w-%d", i), "proj-a", fmt.Sprintf("key-%d", i), []string{"q"}, 1)
				w.OrgID = "org-1"
				_ = r.Register(w)
			})
		}
	}

	// Concurrently call CountByOrg.
	var readDone atomic.Bool
	concWG.Go(func() {
		for !readDone.Load() {
			_ = r.CountByOrg("org-1")
		}
	})

	wg.Wait()
	readDone.Store(true)
	assert.Equal(t,
		n, r.CountByOrg("org-1"))
}

// gatingResult mirrors the real stream gating logic in stream.go so we can
// unit-test it without spinning up a full bidirectional gRPC stream. If the
// real call site changes, this test will rot; that's acceptable — the
// adversarial integration tests in stream_*_integration_test.go cover the
// wired path.
func gatingResult(ctx context.Context, edition domain.Edition, enforcer planLimitEnforcer, registry *ConnectionRegistry, projectID string) (orgID string, blocked bool) {
	if enforcer == nil {
		return "", edition.RequiresHTTPModeGating()
	}
	orgID, err := enforcer.GetActiveProjectOrgID(ctx, projectID)
	if err != nil {
		return "", true
	}
	if orgID == "" {
		return "", true
	}
	currentActive := registry.CountByOrg(orgID)
	if err := enforcer.CheckWorkerConnectionLimit(ctx, orgID, currentActive); err != nil {
		return orgID, true
	}
	return orgID, false
}

func TestStreamGating_CommunityNilEnforcerFailsOpen(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	if _, blocked := gatingResult(context.Background(), domain.EditionCommunity, nil, r, "proj-a"); blocked {
		require.Fail(t,

			"expected community nil enforcer to fail open, got blocked")
	}
}

func TestStreamGating_CloudNilEnforcerFailsClosed(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	if _, blocked := gatingResult(context.Background(), domain.EditionCloud, nil, r, "proj-a"); !blocked {
		require.Fail(t,

			"expected cloud nil enforcer to fail closed, got allowed")
	}
}

func TestCheckPlanConnectionLimit_CloudNilEnforcerUnavailable(t *testing.T) {
	t.Parallel()

	svc := &workerService{
		registry: NewConnectionRegistry(),
		edition:  domain.EditionCloud,
	}

	orgID, release, err := svc.checkPlanConnectionLimit(context.Background(), "proj-a", "reservation-1")
	require.Error(
		t, err)
	require.Equal(
		t, codes.Unavailable,

		status.
			Code(err))
	require.Empty(
		t, orgID,
	)
	require.NotNil(t, release)
}

func TestCheckPlanConnectionLimit_CommunityNilEnforcerAllows(t *testing.T) {
	t.Parallel()

	svc := &workerService{
		registry: NewConnectionRegistry(),
		edition:  domain.EditionCommunity,
	}

	orgID, release, err := svc.checkPlanConnectionLimit(context.Background(), "proj-a", "reservation-1")
	require.NoError(t, err)
	require.Empty(
		t, orgID,
	)
	require.NotNil(t, release)
}

func TestCheckPlanConnectionLimit_CommunityConfiguredEnforcerBypassesPlanGate(t *testing.T) {
	t.Parallel()

	enforcer := &releaseRecordingReservationEnforcer{
		stubPlanLimitEnforcer: stubPlanLimitEnforcer{
			orgLookupErr: errors.New("billing store should not be called"),
			limit:        0,
		},
	}
	svc := &workerService{
		registry:        NewConnectionRegistry(),
		billingEnforcer: enforcer,
		edition:         domain.EditionCommunity,
	}

	orgID, release, err := svc.checkPlanConnectionLimit(context.Background(), "proj-a", "reservation-1")
	require.NoError(t, err)
	require.Empty(
		t, orgID,
	)
	require.NotNil(t, release)
	require.Equal(t, 0, enforcer.
		orgLookupHits,
	)
	require.Equal(t, 0, enforcer.
		reserveCalls,
	)
}

// TestStreamGating_OrgLookupError_FailsClosed verifies that an explicit DB
// error during org resolution blocks the connection rather than bypassing the
// worker connection plan cap.
func TestStreamGating_OrgLookupError_FailsClosed(t *testing.T) {
	t.Parallel()

	enforcer := &stubPlanLimitEnforcer{
		orgLookupErr: errors.New("db down"),
		limit:        0, // would block everything if we reached this
	}
	r := NewConnectionRegistry()
	orgID, blocked := gatingResult(context.Background(), domain.EditionCloud, enforcer, r, "proj-a")
	require.True(t,
		blocked)
	assert.Empty(t,
		orgID,
	)
	assert.Equal(t, 0, enforcer.
		callsLen())
}

func TestCheckPlanConnectionLimit_OrgLookupErrorIsUnavailable(t *testing.T) {
	t.Parallel()

	enforcer := &stubPlanLimitEnforcer{
		orgLookupErr: errors.New("db down"),
		limit:        -1,
	}
	svc := &workerService{
		registry:        NewConnectionRegistry(),
		billingEnforcer: enforcer,
	}

	orgID, release, err := svc.checkPlanConnectionLimit(context.Background(), "proj-a", "reservation-1")
	require.Error(
		t, err)
	require.Equal(
		t, codes.Unavailable,

		status.
			Code(err))
	require.Empty(
		t, orgID,
	)
	require.NotNil(t, release)
	assert.Equal(t, 0, enforcer.
		callsLen())
}

// TestStreamGating_UnresolvedOrg_FailsClosed verifies that an empty OrgID
// (project not bound to an org) blocks cloud connections rather than bypassing
// the worker connection plan cap.
func TestStreamGating_UnresolvedOrg_FailsClosed(t *testing.T) {
	t.Parallel()

	enforcer := &stubPlanLimitEnforcer{
		orgIDForProject: map[string]string{}, // no entry → returns ""
		limit:           0,
	}
	r := NewConnectionRegistry()
	if _, blocked := gatingResult(context.Background(), domain.EditionCloud, enforcer, r, "proj-a"); !blocked {
		require.Fail(t,

			"expected fail-closed with unresolved org, got allowed")
	}
	assert.Equal(t, 0, enforcer.
		callsLen())
}

func TestCheckPlanConnectionLimit_UnresolvedOrgIsUnavailable(t *testing.T) {
	t.Parallel()

	enforcer := &stubPlanLimitEnforcer{
		orgIDForProject: map[string]string{},
		limit:           -1,
	}
	svc := &workerService{
		registry:        NewConnectionRegistry(),
		billingEnforcer: enforcer,
		edition:         domain.EditionCloud,
	}

	orgID, release, err := svc.checkPlanConnectionLimit(context.Background(), "proj-a", "reservation-1")
	require.Error(
		t, err)
	require.Equal(
		t, codes.Unavailable,

		status.
			Code(err))
	require.Empty(
		t, orgID,
	)
	require.NotNil(t, release)
	assert.Equal(t, 0, enforcer.
		callsLen())
}

// TestStreamGating_BelowLimit_Allows verifies happy path: org has 2 active
// workers under a 5-worker cap, the new one is allowed.
func TestStreamGating_BelowLimit_Allows(t *testing.T) {
	t.Parallel()

	enforcer := &stubPlanLimitEnforcer{
		orgIDForProject: map[string]string{"proj-a": "org-1"},
		limit:           5,
	}
	r := NewConnectionRegistry()
	for i := range 2 {
		w := makeWorker(fmt.Sprintf("existing-%d", i), "proj-a", fmt.Sprintf("key-%d", i), []string{"q"}, 1)
		w.OrgID = "org-1"
		require.NoError(t, r.Register(w))
	}

	orgID, blocked := gatingResult(context.Background(), domain.EditionCloud, enforcer, r, "proj-a")
	require.False(
		t, blocked,
	)
	assert.Equal(t,
		"org-1",
		orgID)

	if calls := enforcer.checkCalls; len(calls) != 1 || calls[0].CurrentActive != 2 {
		assert.Failf(t, "test failure",

			"check calls = %+v, want one call with CurrentActive=2", calls)
	}
}

// TestStreamGating_AtLimit_Blocks verifies that once active == cap, the next
// connect is rejected. The real stream.go translates this rejection to
// codes.ResourceExhausted; we only verify the gating decision here.
func TestStreamGating_AtLimit_Blocks(t *testing.T) {
	t.Parallel()

	enforcer := &stubPlanLimitEnforcer{
		orgIDForProject: map[string]string{"proj-a": "org-1"},
		limit:           3,
	}
	r := NewConnectionRegistry()
	for i := range 3 {
		w := makeWorker(fmt.Sprintf("existing-%d", i), "proj-a", fmt.Sprintf("key-%d", i), []string{"q"}, 1)
		w.OrgID = "org-1"
		require.NoError(t, r.Register(w))
	}

	if _, blocked := gatingResult(context.Background(), domain.EditionCloud, enforcer, r, "proj-a"); !blocked {
		require.Fail(t,

			"expected block at 3/3, got allow")
	}
}

// TestStreamGating_OverLimit_Blocks covers the "downgrade resulted in over-cap"
// scenario: existing connections > cap. New connections still must be rejected.
func TestStreamGating_OverLimit_Blocks(t *testing.T) {
	t.Parallel()

	enforcer := &stubPlanLimitEnforcer{
		orgIDForProject: map[string]string{"proj-a": "org-1"},
		limit:           1, // org was Pro (25), now Free (1)
	}
	r := NewConnectionRegistry()
	// Seed 5 existing connections that survived a downgrade.
	for i := range 5 {
		w := makeWorker(fmt.Sprintf("survivor-%d", i), "proj-a", fmt.Sprintf("key-%d", i), []string{"q"}, 1)
		w.OrgID = "org-1"
		require.NoError(t, r.Register(w))
	}
	if _, blocked := gatingResult(context.Background(), domain.EditionCloud, enforcer, r, "proj-a"); !blocked {
		require.Fail(t,

			"expected block at 5/1, got allow")
	}
}

// TestStreamGating_UnlimitedTier_NeverBlocks verifies that limit=-1 (unlimited,
// e.g. Enterprise) accepts any number of connections.
func TestStreamGating_UnlimitedTier_NeverBlocks(t *testing.T) {
	t.Parallel()

	enforcer := &stubPlanLimitEnforcer{
		orgIDForProject: map[string]string{"proj-a": "org-1"},
		limit:           -1,
	}
	r := NewConnectionRegistry()
	for i := range 50 {
		w := makeWorker(fmt.Sprintf("w-%d", i), "proj-a", fmt.Sprintf("key-%d", i), []string{"q"}, 1)
		w.OrgID = "org-1"
		require.NoError(t, r.Register(w))
	}
	if _, blocked := gatingResult(context.Background(), domain.EditionCloud, enforcer, r, "proj-a"); blocked {
		require.Fail(t,

			"expected allow at 50/unlimited, got blocked")
	}
}

// TestStreamGating_PerOrgIsolation verifies that org-A's connections do not
// count toward org-B's quota.
func TestStreamGating_PerOrgIsolation(t *testing.T) {
	t.Parallel()

	enforcer := &stubPlanLimitEnforcer{
		orgIDForProject: map[string]string{"proj-a": "org-1", "proj-b": "org-2"},
		limit:           2,
	}
	r := NewConnectionRegistry()
	// Saturate org-1.
	for i := range 2 {
		w := makeWorker(fmt.Sprintf("a-%d", i), "proj-a", fmt.Sprintf("ka-%d", i), []string{"q"}, 1)
		w.OrgID = "org-1"
		require.NoError(t, r.Register(w))
	}

	// org-1 is at cap, must be blocked.
	if _, blocked := gatingResult(context.Background(), domain.EditionCloud, enforcer, r, "proj-a"); !blocked {
		require.Fail(t,

			"expected block for org-1 (saturated)")
	}

	// org-2 is empty, must be allowed.
	if _, blocked := gatingResult(context.Background(), domain.EditionCloud, enforcer, r, "proj-b"); blocked {
		require.Fail(t,

			"expected allow for org-2 (empty)")
	}
}
