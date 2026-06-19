package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingBillingDispatcher captures every billing webhook event the
// downgrade applier emits so tests can assert reason + per-id fan-out.
type recordingBillingDispatcher struct {
	mu     sync.Mutex
	events []dispatchedEvent
}

type dispatchedEvent struct {
	orgID     string
	eventType string
	payload   []byte
}

func (r *recordingBillingDispatcher) DispatchBillingEvent(_ context.Context, orgID, eventType string, payload []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, dispatchedEvent{
		orgID:     orgID,
		eventType: eventType,
		payload:   append([]byte(nil), payload...),
	})
	return nil
}

func (r *recordingBillingDispatcher) snapshot() []dispatchedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]dispatchedEvent, len(r.events))
	copy(out, r.events)
	return out
}

func httpModeDisabledLimits() billing.OrgPlanLimits {
	return billing.OrgPlanLimits{
		PlanTier:                domain.PlanTier("worker-only"),
		MaxProjectsPerOrg:       -1,
		MaxScheduledJobs:        -1,
		MaxWebhookEndpoints:     -1,
		MaxEnvironments:         -1,
		MaxLogDrainsPerOrg:      -1,
		MaxNotificationChannels: -1,
		MaxMembersPerOrg:        -1,
		AllowsHTTPMode:          false,
	}
}

// TestDowngradeApplier_DispatchesScheduleSuspended_OnCronTrim covers the
// Pro→Free path where DeactivateExcessCronJobs returns multiple job IDs and
// confirms one schedule.suspended event fires per ID with reason
// plan_downgrade_cron_limit.
func TestDowngradeApplier_DispatchesScheduleSuspended_OnCronTrim(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-time.Hour)
	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-cron", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
	}
	// Override DeactivateExcessCronJobs to return a fixed ID set so we can
	// assert the dispatch fans out once per id.
	disp := &recordingBillingDispatcher{}
	enforcer := newTestEnforcer(t)
	applier := NewDowngradeApplier(
		&cronTrimmingStore{mockDowngradeStore: store, cronIDs: []string{"job-1", "job-2", "job-3"}},
		enforcer,
		time.Minute,
	).WithBillingDispatcher(disp)
	applier.apply(context.Background())

	events := disp.snapshot()
	cronEvents := 0
	for _, e := range events {
		if e.eventType == domain.WebhookEventScheduleSuspended {
			cronEvents++
		}
	}
	require.Equal(t, 3,
		cronEvents,
	)
}

func TestEnforcePlanResourceLimits_DispatchesHTTPPauseWhenTargetDisallowsHTTPMode(t *testing.T) {
	t.Parallel()

	store := &mockDowngradeStore{pauseHTTPIDs: []string{"job-http-1", "job-http-2"}}
	disp := &recordingBillingDispatcher{}

	err := enforceResolvedPlanResourceLimits(
		context.Background(),
		store,
		nil,
		disp,
		"org-http",
		"worker-only",
		httpModeDisabledLimits(),
	)
	require.NoError(t, err)
	require.Equal(t, []pauseHTTPCall{
		{orgID: "org-http", reason: "plan_downgrade"},
	}, store.pauseHTTPCalls)

	events := disp.snapshot()
	require.Len(t, events, 2)
	for i, event := range events {
		assert.Equal(t, "org-http", event.orgID)
		assert.Equal(t, domain.WebhookEventScheduleSuspended, event.eventType)

		var envelope billing.BillingEventEnvelope
		require.NoError(t, json.Unmarshal(event.payload, &envelope))
		assert.Equal(t, "org-http", envelope.OrgID)
		assert.Equal(t, "worker-only", envelope.PlanTier)
		assert.Equal(t, domain.WebhookEventScheduleSuspended, envelope.EventType)
		assert.Equal(t, "plan_downgrade_http_mode", envelope.Detail["reason"])
		assert.Equal(t, store.pauseHTTPIDs[i], envelope.Detail["schedule_id"])
	}
}

func TestEnforcePlanResourceLimits_ReturnsHTTPPauseError(t *testing.T) {
	t.Parallel()

	pauseErr := errors.New("pause failed")
	store := &mockDowngradeStore{pauseHTTPErr: pauseErr}

	err := enforceResolvedPlanResourceLimits(
		context.Background(),
		store,
		nil,
		nil,
		"org-http",
		"worker-only",
		httpModeDisabledLimits(),
	)
	require.ErrorIs(t, err, pauseErr)
	require.ErrorContains(t, err, "pause http jobs")
	require.Equal(t, []pauseHTTPCall{
		{orgID: "org-http", reason: "plan_downgrade"},
	}, store.pauseHTTPCalls)
}

// TestDowngradeApplier_NoDispatch_WhenNoIDsReturned guards the fast-path: if
// DeactivateExcessCronJobs / PauseHTTPJobsByOrg return empty slices, no event
// is dispatched (regression for accidental "always fan out" loops).
func TestDowngradeApplier_NoDispatch_WhenNoIDsReturned(t *testing.T) {
	t.Parallel()

	pro := "pro"
	pastEnd := time.Now().Add(-time.Hour)
	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-empty", PlanTier: "scale", PendingPlanTier: &pro, CurrentPeriodEnd: &pastEnd},
		},
	}

	disp := &recordingBillingDispatcher{}
	enforcer := newTestEnforcer(t)
	applier := NewDowngradeApplier(store, enforcer, time.Minute).WithBillingDispatcher(disp)
	applier.apply(context.Background())

	for _, e := range disp.snapshot() {
		assert.NotEqual(t,
			domain.WebhookEventScheduleSuspended,

			e.eventType)
	}
}

// cronTrimmingStore overrides DeactivateExcessCronJobs to return a fixed ID
// set without mutating mockDowngradeStore's shared signature behaviour.
type cronTrimmingStore struct {
	*mockDowngradeStore
	cronIDs []string
}

func (c *cronTrimmingStore) DeactivateExcessCronJobs(_ context.Context, _ string, _ int) ([]string, error) {
	return c.cronIDs, nil
}
