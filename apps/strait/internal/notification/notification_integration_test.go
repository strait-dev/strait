//go:build integration

package notification_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/notification"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testDB *testutil.TestDB

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testDB, err = testutil.SetupSharedTestDB(ctx, "../../migrations", "notification")
	if err != nil {
		log.Fatalf("setup test db: %v", err)
	}

	code := m.Run()
	testDB.Cleanup(ctx)
	os.Exit(code)
}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func mustStore(t *testing.T) *store.Queries {
	t.Helper()
	return store.New(testDB.Pool)
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := testDB.Pool.Exec(ctx, `TRUNCATE TABLE notification_deliveries, notification_channels CASCADE`)
	require.NoError(t, err)

}

func makeChannel(projectID, channelType, name string, config json.RawMessage) *domain.NotificationChannel {
	return &domain.NotificationChannel{
		ID:          newID(),
		ProjectID:   projectID,
		ChannelType: channelType,
		Name:        name,
		Config:      config,
		Enabled:     true,
	}
}

func makeDelivery(channelID, projectID, eventType string, payload json.RawMessage) *domain.NotificationDelivery {
	return &domain.NotificationDelivery{
		ID:          newID(),
		ChannelID:   channelID,
		ProjectID:   projectID,
		EventType:   eventType,
		Payload:     payload,
		Status:      "pending",
		MaxAttempts: 3,
	}
}

// -- Channel CRUD tests --.

func TestCreateAndGetNotificationChannel(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	ch := makeChannel("proj-1", domain.ChannelTypeSlack, "my-slack", json.RawMessage(`{"webhook_url":"https://hooks.slack.com/test"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))
	require.False(t, ch.CreatedAt.
		IsZero())

	got, err := st.GetNotificationChannel(ctx, ch.ID, ch.ProjectID)
	require.NoError(t, err)
	assert.Equal(t, ch.ID, got.ID)
	assert.Equal(t, domain.ChannelTypeSlack,

		got.ChannelType,
	)
	assert.Equal(t, "my-slack", got.
		Name)
	assert.True(t, got.Enabled)

}

func TestGetNotificationChannel_NotFound(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	_, err := st.GetNotificationChannel(ctx, newID(), "proj-nonexistent")
	require.Error(t, err)
	require.True(t, errors.Is(err,

		store.ErrNotificationChannelNotFound,
	),
	)

}

func TestListNotificationChannels(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-list"

	ch1 := makeChannel(projectID, domain.ChannelTypeSlack, "slack-1", json.RawMessage(`{"webhook_url":"https://s1"}`))
	ch2 := makeChannel(projectID, domain.ChannelTypeDiscord, "discord-1", json.RawMessage(`{"webhook_url":"https://d1"}`))
	ch3 := makeChannel("proj-other", domain.ChannelTypeWebhook, "webhook-other", json.RawMessage(`{"url":"https://w1"}`))

	for _, ch := range []*domain.NotificationChannel{ch1, ch2, ch3} {
		require.NoError(t, st.CreateNotificationChannel(ctx,
			ch))

	}

	channels, err := st.ListNotificationChannels(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, channels, 2)

}

func TestListEnabledNotificationChannels(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-enabled"

	chEnabled := makeChannel(projectID, domain.ChannelTypeSlack, "enabled-ch", json.RawMessage(`{"webhook_url":"https://e"}`))
	chDisabled := makeChannel(projectID, domain.ChannelTypeDiscord, "disabled-ch", json.RawMessage(`{"webhook_url":"https://d"}`))
	chDisabled.Enabled = false

	for _, ch := range []*domain.NotificationChannel{chEnabled, chDisabled} {
		require.NoError(t, st.CreateNotificationChannel(ctx,
			ch))

	}

	channels, err := st.ListEnabledNotificationChannels(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, channels, 1)
	assert.Equal(t, chEnabled.ID,
		channels[0].ID)

}

func TestUpdateNotificationChannel(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	ch := makeChannel("proj-update", domain.ChannelTypeSlack, "original-name", json.RawMessage(`{"webhook_url":"https://orig"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	ch.Name = "updated-name"
	ch.Enabled = false
	require.NoError(t, st.UpdateNotificationChannel(ctx,
		ch))

	got, err := st.GetNotificationChannel(ctx, ch.ID, ch.ProjectID)
	require.NoError(t, err)
	assert.Equal(t, "updated-name",

		got.Name,
	)
	assert.False(t, got.Enabled)

}

func TestDeleteNotificationChannel(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	ch := makeChannel("proj-delete", domain.ChannelTypeWebhook, "to-delete", json.RawMessage(`{"url":"https://del"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))
	require.NoError(t, st.DeleteNotificationChannel(ctx,
		ch.ID, ch.ProjectID,
	))

	_, err := st.GetNotificationChannel(ctx, ch.ID, ch.ProjectID)
	require.True(t, errors.Is(err,

		store.ErrNotificationChannelNotFound,
	),
	)

}

func TestDeleteNotificationChannel_NotFound(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	err := st.DeleteNotificationChannel(ctx, newID(), "proj-nope")
	require.True(t, errors.Is(err,

		store.ErrNotificationChannelNotFound,
	),
	)

}

// -- Delivery storage tests --.

func TestCreateAndListNotificationDeliveries(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-deliveries"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "delivery-ch", json.RawMessage(`{"webhook_url":"https://h"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	d1 := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{"project_id":"p1"}`))
	d2 := makeDelivery(ch.ID, projectID, domain.NotificationEventCostAnomaly, json.RawMessage(`{"severity":"high"}`))

	for _, d := range []*domain.NotificationDelivery{d1, d2} {
		require.NoError(t, st.CreateNotificationDelivery(ctx,
			d))

	}
	require.False(t, d1.CreatedAt.
		IsZero())

	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	require.NoError(t, err)
	require.Len(t, deliveries, 2)
	assert.Equal(t, d2.ID, deliveries[0].ID)

	// Results are ordered by created_at DESC, so d2 should come first.

}

func TestListNotificationDeliveries_Cursor(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-cursor"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "cursor-ch", json.RawMessage(`{"webhook_url":"https://c"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	// Create 3 deliveries with small time gaps for ordering.
	ids := make([]string, 3)
	for i := range 3 {
		d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
		require.NoError(t, st.CreateNotificationDelivery(ctx,
			d))

		ids[i] = d.ID
	}

	// Fetch first page (limit 2).
	page1, err := st.ListNotificationDeliveries(ctx, projectID, 2, nil)
	require.NoError(t, err)
	require.Len(t, page1, 2)

	// Use the created_at of the last item as cursor.
	cursor := page1[len(page1)-1].CreatedAt
	page2, err := st.ListNotificationDeliveries(ctx, projectID, 2, &cursor)
	require.NoError(t, err)
	require.Len(t, page2, 1)

}

// -- Claim and processing tests --.

func TestClaimPendingNotificationDeliveries(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-claim"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "claim-ch", json.RawMessage(`{"webhook_url":"https://cl"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventSpendingLimitWarning, json.RawMessage(`{}`))
	require.NoError(t, st.CreateNotificationDelivery(ctx,
		d))

	claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 5, 2*time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, d.ID, claimed[0].ID)
	assert.Equal(t, "processing",
		claimed[0].
			Status)
	assert.NotEqual(t, "", claimed[0].ClaimToken)

	// A second claim should return nothing since the delivery is now processing.
	claimed2, err := st.ClaimPendingNotificationDeliveries(ctx, 5, 2*time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed2, 0)

}

func TestConcurrentClaims_NoDoubleClaim(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-concurrent"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "concurrent-ch", json.RawMessage(`{"webhook_url":"https://cc"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	// Create a single delivery.
	d := makeDelivery(ch.ID, projectID, domain.NotificationEventSpendingLimitReached, json.RawMessage(`{}`))
	require.NoError(t, st.CreateNotificationDelivery(ctx,
		d))

	// Race multiple goroutines to claim the same delivery.
	const concurrency = 10
	results := make(chan []domain.NotificationDelivery, concurrency)

	var wg conc.WaitGroup
	for range concurrency {
		wg.Go(func() {
			claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
			if !assert.NoError(t, err) {
				return
			}
			results <- claimed
		})
	}
	wg.Wait()
	close(results)

	totalClaimed := 0
	for claimed := range results {
		totalClaimed += len(claimed)
	}
	require.Equal(t, 1, totalClaimed)

}

func TestUpdateClaimedNotificationDelivery(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-update-claim"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "update-claim-ch", json.RawMessage(`{"webhook_url":"https://uc"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
	require.NoError(t, st.CreateNotificationDelivery(ctx,
		d))

	claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	require.False(t, err != nil ||

		len(claimed) != 1)

	c := &claimed[0]
	now := time.Now()
	c.Status = "delivered"
	c.Attempts = 1
	c.DeliveredAt = &now
	c.LastError = ""

	updated, err := st.UpdateClaimedNotificationDelivery(ctx, c)
	require.NoError(t, err)
	require.True(t, updated)

	// Verify the delivery is now in delivered state.
	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, "delivered", deliveries[0].Status)
	assert.NotNil(t, deliveries[0].
		DeliveredAt,
	)

}

func TestUpdateClaimedNotificationDelivery_WrongToken(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-wrong-token"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "wrong-token-ch", json.RawMessage(`{"webhook_url":"https://wt"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
	require.NoError(t, st.CreateNotificationDelivery(ctx,
		d))

	claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	require.False(t, err != nil ||

		len(claimed) != 1)

	c := &claimed[0]
	c.ClaimToken = "wrong-token-value"
	c.Status = "delivered"
	c.Attempts = 1

	updated, err := st.UpdateClaimedNotificationDelivery(ctx, c)
	require.NoError(t, err)
	require.False(t, updated)

}

// -- Status lifecycle tests --.

func TestDeliveryStatusLifecycle_PendingToDelivered(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-lifecycle"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "lifecycle-ch", json.RawMessage(`{"webhook_url":"https://lc"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventSpendingLimitWarning, json.RawMessage(`{}`))
	require.NoError(t, st.CreateNotificationDelivery(ctx,
		d))

	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	require.NoError(t, err)
	require.Equal(t, "pending", deliveries[0].Status)

	// Claim: pending -> processing.
	claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	require.False(t, err != nil ||

		len(claimed) != 1)
	require.Equal(t, "processing",

		claimed[0].Status)

	// Mark delivered: processing -> delivered.
	c := &claimed[0]
	now := time.Now()
	c.Status = "delivered"
	c.Attempts = 1
	c.DeliveredAt = &now

	updated, err := st.UpdateClaimedNotificationDelivery(ctx, c)
	require.False(t, err != nil ||

		!updated)

	deliveries, err = st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	require.NoError(t, err)
	require.Equal(t, "delivered",
		deliveries[0].Status)

}

func TestDeliveryStatusLifecycle_PendingToFailed(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-fail"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "fail-ch", json.RawMessage(`{"webhook_url":"https://fl"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventCostAnomaly, json.RawMessage(`{}`))
	require.NoError(t, st.CreateNotificationDelivery(ctx,
		d))

	claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	require.False(t, err != nil ||

		len(claimed) != 1)

	c := &claimed[0]
	c.Status = "failed"
	c.Attempts = 3
	c.LastError = "send failed after max retries"

	updated, err := st.UpdateClaimedNotificationDelivery(ctx, c)
	require.False(t, err != nil ||

		!updated)

	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	require.NoError(t, err)
	assert.Equal(t, "failed", deliveries[0].
		Status)
	assert.Equal(t, "send failed after max retries",

		deliveries[0].LastError,
	)

}

// -- Retry flow tests --.

func TestDeliveryRetryFlow(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-retry"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "retry-ch", json.RawMessage(`{"webhook_url":"https://rt"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
	require.NoError(t, st.CreateNotificationDelivery(ctx,
		d))

	// First attempt: claim, fail, set retry in the past so it becomes claimable again.
	claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	require.False(t, err != nil ||

		len(claimed) != 1)

	c := &claimed[0]
	c.Status = "pending"
	c.Attempts = 1
	c.LastError = "temporary failure"
	pastRetry := time.Now().Add(-1 * time.Second)
	c.NextRetryAt = &pastRetry

	updated, err := st.UpdateClaimedNotificationDelivery(ctx, c)
	require.False(t, err != nil ||

		!updated)

	// Second attempt: the delivery should be claimable again because next_retry_at is in the past.
	claimed2, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed2, 1)
	assert.Equal(t, d.ID, claimed2[0].ID)

}

func TestDeliveryRetryNotClaimableBeforeNextRetryAt(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-retry-future"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "retry-future-ch", json.RawMessage(`{"webhook_url":"https://rf"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
	require.NoError(t, st.CreateNotificationDelivery(ctx,
		d))

	claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	require.False(t, err != nil ||

		len(claimed) != 1)

	c := &claimed[0]
	c.Status = "pending"
	c.Attempts = 1
	c.LastError = "temporary failure"
	futureRetry := time.Now().Add(1 * time.Hour)
	c.NextRetryAt = &futureRetry

	updated, err := st.UpdateClaimedNotificationDelivery(ctx, c)
	require.False(t, err != nil ||

		!updated)

	// The delivery should not be claimable because next_retry_at is in the future.
	claimed2, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed2, 0)

}

// -- Worker integration with real DB --.

// fakeSender records calls and optionally returns an error.
type fakeSender struct {
	mu       sync.Mutex
	calls    int
	sendFunc func(ctx context.Context, ch *domain.NotificationChannel, d *domain.NotificationDelivery) error
}

func (f *fakeSender) Send(ctx context.Context, ch *domain.NotificationChannel, d *domain.NotificationDelivery) error {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.sendFunc != nil {
		return f.sendFunc(ctx, ch, d)
	}
	return nil
}

func (f *fakeSender) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestWorkerProcessesDeliveryFromRealDB(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-worker"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "worker-ch", json.RawMessage(`{"webhook_url":"https://wk"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventSpendingLimitWarning, json.RawMessage(`{"org_id":"org-1"}`))
	require.NoError(t, st.CreateNotificationDelivery(ctx,
		d))

	sender := &fakeSender{}
	w := notification.NewWorker(st, &http.Client{})
	w.RegisterSender(domain.ChannelTypeSlack, sender)

	workerCtx, cancel := context.WithCancel(ctx)
	w.Start(workerCtx)

	// Wait for the worker to process (it processes immediately on start).
	time.Sleep(500 * time.Millisecond)
	cancel()
	w.Stop()
	require.Equal(t, 1, sender.callCount())

	// Verify the delivery is now delivered.
	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, "delivered", deliveries[0].Status)

}

func TestWorkerSkipsQueuedDeliveryAfterChannelDisabled(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-worker-disabled"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "worker-disabled-ch", json.RawMessage(`{"webhook_url":"https://disabled"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventSpendingLimitWarning, json.RawMessage(`{"org_id":"org-disabled"}`))
	require.NoError(t, st.CreateNotificationDelivery(ctx,
		d))

	ch.Enabled = false
	require.NoError(t, st.UpdateNotificationChannel(ctx,
		ch))

	sender := &fakeSender{}
	w := notification.NewWorker(st, &http.Client{})
	w.RegisterSender(domain.ChannelTypeSlack, sender)

	workerCtx, cancel := context.WithCancel(ctx)
	w.Start(workerCtx)

	time.Sleep(500 * time.Millisecond)
	cancel()
	w.Stop()
	require.Equal(t, 0, sender.callCount())

	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	require.Equal(t, "failed", deliveries[0].
		Status)
	require.Equal(t, 0, deliveries[0].Attempts)
	require.True(t, strings.Contains(deliveries[0].LastError,
		"disabled",
	))

}

func TestWorkerDispatchesFastDeliveryWhileSlowEndpointBlocks(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	chSlow := makeChannel("proj-worker-slow", domain.ChannelTypeSlack, "slow-ch", json.RawMessage(`{"webhook_url":"https://slow"}`))
	chFast := makeChannel("proj-worker-fast", domain.ChannelTypeSlack, "fast-ch", json.RawMessage(`{"webhook_url":"https://fast"}`))
	for _, ch := range []*domain.NotificationChannel{chSlow, chFast} {
		require.NoError(t, st.CreateNotificationChannel(ctx,
			ch))

	}

	dSlow := makeDelivery(chSlow.ID, chSlow.ProjectID, domain.NotificationEventSpendingLimitWarning, json.RawMessage(`{"org_id":"slow"}`))
	dFast := makeDelivery(chFast.ID, chFast.ProjectID, domain.NotificationEventSpendingLimitWarning, json.RawMessage(`{"org_id":"fast"}`))
	for _, d := range []*domain.NotificationDelivery{dSlow, dFast} {
		require.NoError(t, st.CreateNotificationDelivery(ctx,
			d))

	}

	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	fastSent := make(chan struct{})
	sender := &fakeSender{
		sendFunc: func(_ context.Context, ch *domain.NotificationChannel, _ *domain.NotificationDelivery) error {
			switch ch.ProjectID {
			case chSlow.ProjectID:
				close(slowStarted)
				<-releaseSlow
			case chFast.ProjectID:
				close(fastSent)
			}
			return nil
		},
	}
	w := notification.NewWorker(st, &http.Client{})
	w.RegisterSender(domain.ChannelTypeSlack, sender)

	workerCtx, cancel := context.WithCancel(ctx)
	w.Start(workerCtx)
	defer func() {
		cancel()
		w.Stop()
	}()

	select {
	case <-slowStarted:
	case <-time.After(time.Second):
		require.FailNow(t, "slow delivery was not started")
	}
	select {
	case <-fastSent:
	case <-time.After(250 * time.Millisecond):
		require.FailNow(t, "fast delivery was blocked behind slow endpoint")
	}
	close(releaseSlow)

	deadline := time.After(2 * time.Second)
	for {
		deliveries, err := st.ListNotificationDeliveries(ctx, chFast.ProjectID, 10, nil)
		require.NoError(t, err)

		if len(deliveries) == 1 && deliveries[0].Status == "delivered" {
			return
		}
		select {
		case <-deadline:
			require.FailNowf(t, "fast delivery was not marked delivered", "%+v", deliveries)
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func TestWorkerMarksDeliveryFailedAfterMaxAttempts(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-worker-fail"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "worker-fail-ch", json.RawMessage(`{"webhook_url":"https://wf"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
	d.MaxAttempts = 1
	require.NoError(t, st.CreateNotificationDelivery(ctx,
		d))

	// Only one attempt allowed.

	sender := &fakeSender{
		sendFunc: func(_ context.Context, _ *domain.NotificationChannel, _ *domain.NotificationDelivery) error {
			return errSendFailed
		},
	}
	w := notification.NewWorker(st, &http.Client{})
	w.RegisterSender(domain.ChannelTypeSlack, sender)

	workerCtx, cancel := context.WithCancel(ctx)
	w.Start(workerCtx)

	time.Sleep(500 * time.Millisecond)
	cancel()
	w.Stop()

	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, "failed", deliveries[0].
		Status)
	assert.Equal(t, 1, deliveries[0].Attempts)

}

var errSendFailed = fmt.Errorf("simulated send failure")

func TestWorkerSetsRetryOnFailureBelowMaxAttempts(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-worker-retry"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "worker-retry-ch", json.RawMessage(`{"webhook_url":"https://wr"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
	d.MaxAttempts = 3
	require.NoError(t, st.CreateNotificationDelivery(ctx,
		d))

	sender := &fakeSender{
		sendFunc: func(_ context.Context, _ *domain.NotificationChannel, _ *domain.NotificationDelivery) error {
			return errSendFailed
		},
	}
	w := notification.NewWorker(st, &http.Client{})
	w.RegisterSender(domain.ChannelTypeSlack, sender)

	workerCtx, cancel := context.WithCancel(ctx)
	w.Start(workerCtx)

	time.Sleep(500 * time.Millisecond)
	cancel()
	w.Stop()

	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, "pending", deliveries[0].
		Status)
	assert.Equal(t, 1, deliveries[0].Attempts)
	assert.NotNil(t, deliveries[0].
		NextRetryAt,
	)

	// After one failed attempt below max, status should be back to pending with a next_retry_at.

}

func TestDeleteChannelCascadesDeliveries(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-cascade"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "cascade-ch", json.RawMessage(`{"webhook_url":"https://cas"}`))
	require.NoError(t, st.CreateNotificationChannel(ctx,
		ch))

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
	require.NoError(t, st.CreateNotificationDelivery(ctx,
		d))
	require.NoError(t, st.DeleteNotificationChannel(ctx,
		ch.ID, ch.ProjectID,
	))

	// Deliveries referencing this channel should be cascade-deleted.
	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	require.NoError(t, err)
	require.Len(t, deliveries, 0)

}
