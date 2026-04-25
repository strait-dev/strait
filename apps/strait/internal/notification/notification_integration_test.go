//go:build integration

package notification_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/notification"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
)

var testDB *testutil.TestDB

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testDB, err = testutil.SetupTestDB(ctx, "../../migrations")
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
	if err != nil {
		t.Fatalf("clean notification tables: %v", err)
	}
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

// -- Channel CRUD tests --

func TestCreateAndGetNotificationChannel(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	ch := makeChannel("proj-1", domain.ChannelTypeSlack, "my-slack", json.RawMessage(`{"webhook_url":"https://hooks.slack.com/test"}`))

	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}
	if ch.CreatedAt.IsZero() {
		t.Fatal("CreateNotificationChannel() did not set created_at")
	}

	got, err := st.GetNotificationChannel(ctx, ch.ID, ch.ProjectID)
	if err != nil {
		t.Fatalf("GetNotificationChannel() error = %v", err)
	}
	if got.ID != ch.ID {
		t.Errorf("ID = %q, want %q", got.ID, ch.ID)
	}
	if got.ChannelType != domain.ChannelTypeSlack {
		t.Errorf("ChannelType = %q, want %q", got.ChannelType, domain.ChannelTypeSlack)
	}
	if got.Name != "my-slack" {
		t.Errorf("Name = %q, want %q", got.Name, "my-slack")
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true")
	}
}

func TestGetNotificationChannel_NotFound(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	_, err := st.GetNotificationChannel(ctx, newID(), "proj-nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent channel, got nil")
	}
	if err != store.ErrNotificationChannelNotFound {
		t.Fatalf("error = %v, want ErrNotificationChannelNotFound", err)
	}
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
		if err := st.CreateNotificationChannel(ctx, ch); err != nil {
			t.Fatalf("CreateNotificationChannel() error = %v", err)
		}
	}

	channels, err := st.ListNotificationChannels(ctx, projectID)
	if err != nil {
		t.Fatalf("ListNotificationChannels() error = %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("ListNotificationChannels() returned %d channels, want 2", len(channels))
	}
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
		if err := st.CreateNotificationChannel(ctx, ch); err != nil {
			t.Fatalf("CreateNotificationChannel() error = %v", err)
		}
	}

	channels, err := st.ListEnabledNotificationChannels(ctx, projectID)
	if err != nil {
		t.Fatalf("ListEnabledNotificationChannels() error = %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("ListEnabledNotificationChannels() returned %d channels, want 1", len(channels))
	}
	if channels[0].ID != chEnabled.ID {
		t.Errorf("returned channel ID = %q, want %q", channels[0].ID, chEnabled.ID)
	}
}

func TestUpdateNotificationChannel(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	ch := makeChannel("proj-update", domain.ChannelTypeSlack, "original-name", json.RawMessage(`{"webhook_url":"https://orig"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	ch.Name = "updated-name"
	ch.Enabled = false
	if err := st.UpdateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("UpdateNotificationChannel() error = %v", err)
	}

	got, err := st.GetNotificationChannel(ctx, ch.ID, ch.ProjectID)
	if err != nil {
		t.Fatalf("GetNotificationChannel() error = %v", err)
	}
	if got.Name != "updated-name" {
		t.Errorf("Name = %q, want %q", got.Name, "updated-name")
	}
	if got.Enabled {
		t.Error("Enabled = true, want false after update")
	}
}

func TestDeleteNotificationChannel(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	ch := makeChannel("proj-delete", domain.ChannelTypeWebhook, "to-delete", json.RawMessage(`{"url":"https://del"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	if err := st.DeleteNotificationChannel(ctx, ch.ID, ch.ProjectID); err != nil {
		t.Fatalf("DeleteNotificationChannel() error = %v", err)
	}

	_, err := st.GetNotificationChannel(ctx, ch.ID, ch.ProjectID)
	if err != store.ErrNotificationChannelNotFound {
		t.Fatalf("expected ErrNotificationChannelNotFound after delete, got %v", err)
	}
}

func TestDeleteNotificationChannel_NotFound(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	err := st.DeleteNotificationChannel(ctx, newID(), "proj-nope")
	if err != store.ErrNotificationChannelNotFound {
		t.Fatalf("error = %v, want ErrNotificationChannelNotFound", err)
	}
}

// -- Delivery storage tests --

func TestCreateAndListNotificationDeliveries(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-deliveries"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "delivery-ch", json.RawMessage(`{"webhook_url":"https://h"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d1 := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{"project_id":"p1"}`))
	d2 := makeDelivery(ch.ID, projectID, domain.NotificationEventCostAnomaly, json.RawMessage(`{"severity":"high"}`))

	for _, d := range []*domain.NotificationDelivery{d1, d2} {
		if err := st.CreateNotificationDelivery(ctx, d); err != nil {
			t.Fatalf("CreateNotificationDelivery() error = %v", err)
		}
	}

	if d1.CreatedAt.IsZero() {
		t.Fatal("CreateNotificationDelivery() did not set created_at")
	}

	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("ListNotificationDeliveries() returned %d, want 2", len(deliveries))
	}
	// Results are ordered by created_at DESC, so d2 should come first.
	if deliveries[0].ID != d2.ID {
		t.Errorf("first delivery ID = %q, want %q (most recent)", deliveries[0].ID, d2.ID)
	}
}

func TestListNotificationDeliveries_Cursor(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-cursor"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "cursor-ch", json.RawMessage(`{"webhook_url":"https://c"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	// Create 3 deliveries with small time gaps for ordering.
	ids := make([]string, 3)
	for i := range 3 {
		d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
		if err := st.CreateNotificationDelivery(ctx, d); err != nil {
			t.Fatalf("CreateNotificationDelivery() error = %v", err)
		}
		ids[i] = d.ID
	}

	// Fetch first page (limit 2).
	page1, err := st.ListNotificationDeliveries(ctx, projectID, 2, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries page1 error = %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 returned %d, want 2", len(page1))
	}

	// Use the created_at of the last item as cursor.
	cursor := page1[len(page1)-1].CreatedAt
	page2, err := st.ListNotificationDeliveries(ctx, projectID, 2, &cursor)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries page2 error = %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2 returned %d, want 1", len(page2))
	}
}

// -- Claim and processing tests --

func TestClaimPendingNotificationDeliveries(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-claim"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "claim-ch", json.RawMessage(`{"webhook_url":"https://cl"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventSpendingLimitWarning, json.RawMessage(`{}`))
	if err := st.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

	claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 5, 2*time.Minute)
	if err != nil {
		t.Fatalf("ClaimPendingNotificationDeliveries() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d deliveries, want 1", len(claimed))
	}
	if claimed[0].ID != d.ID {
		t.Errorf("claimed delivery ID = %q, want %q", claimed[0].ID, d.ID)
	}
	if claimed[0].Status != "processing" {
		t.Errorf("claimed status = %q, want %q", claimed[0].Status, "processing")
	}
	if claimed[0].ClaimToken == "" {
		t.Error("claimed delivery has empty ClaimToken")
	}

	// A second claim should return nothing since the delivery is now processing.
	claimed2, err := st.ClaimPendingNotificationDeliveries(ctx, 5, 2*time.Minute)
	if err != nil {
		t.Fatalf("second ClaimPendingNotificationDeliveries() error = %v", err)
	}
	if len(claimed2) != 0 {
		t.Fatalf("second claim returned %d deliveries, want 0", len(claimed2))
	}
}

func TestConcurrentClaims_NoDoubleClaim(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-concurrent"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "concurrent-ch", json.RawMessage(`{"webhook_url":"https://cc"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	// Create a single delivery.
	d := makeDelivery(ch.ID, projectID, domain.NotificationEventSpendingLimitReached, json.RawMessage(`{}`))
	if err := st.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

	// Race multiple goroutines to claim the same delivery.
	const concurrency = 10
	results := make(chan []domain.NotificationDelivery, concurrency)

	var wg conc.WaitGroup
	for range concurrency {
		wg.Go(func() {
			claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
			if err != nil {
				t.Errorf("concurrent claim error: %v", err)
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
	if totalClaimed != 1 {
		t.Fatalf("concurrent claims produced %d total claimed deliveries, want exactly 1", totalClaimed)
	}
}

func TestUpdateClaimedNotificationDelivery(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-update-claim"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "update-claim-ch", json.RawMessage(`{"webhook_url":"https://uc"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
	if err := st.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

	claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("ClaimPendingNotificationDeliveries() error = %v, len = %d", err, len(claimed))
	}

	c := &claimed[0]
	now := time.Now()
	c.Status = "delivered"
	c.Attempts = 1
	c.DeliveredAt = &now
	c.LastError = ""

	updated, err := st.UpdateClaimedNotificationDelivery(ctx, c)
	if err != nil {
		t.Fatalf("UpdateClaimedNotificationDelivery() error = %v", err)
	}
	if !updated {
		t.Fatal("UpdateClaimedNotificationDelivery() returned false, want true")
	}

	// Verify the delivery is now in delivered state.
	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("ListNotificationDeliveries() returned %d, want 1", len(deliveries))
	}
	if deliveries[0].Status != "delivered" {
		t.Errorf("delivery status = %q, want %q", deliveries[0].Status, "delivered")
	}
	if deliveries[0].DeliveredAt == nil {
		t.Error("delivery delivered_at is nil after successful update")
	}
}

func TestUpdateClaimedNotificationDelivery_WrongToken(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-wrong-token"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "wrong-token-ch", json.RawMessage(`{"webhook_url":"https://wt"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
	if err := st.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

	claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("ClaimPendingNotificationDeliveries() error = %v, len = %d", err, len(claimed))
	}

	c := &claimed[0]
	c.ClaimToken = "wrong-token-value"
	c.Status = "delivered"
	c.Attempts = 1

	updated, err := st.UpdateClaimedNotificationDelivery(ctx, c)
	if err != nil {
		t.Fatalf("UpdateClaimedNotificationDelivery() error = %v", err)
	}
	if updated {
		t.Fatal("UpdateClaimedNotificationDelivery() returned true with wrong token, want false")
	}
}

// -- Status lifecycle tests --

func TestDeliveryStatusLifecycle_PendingToDelivered(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-lifecycle"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "lifecycle-ch", json.RawMessage(`{"webhook_url":"https://lc"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventSpendingLimitWarning, json.RawMessage(`{}`))
	if err := st.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

	// Verify initial status is pending.
	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if deliveries[0].Status != "pending" {
		t.Fatalf("initial status = %q, want %q", deliveries[0].Status, "pending")
	}

	// Claim: pending -> processing.
	claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("Claim error = %v, len = %d", err, len(claimed))
	}
	if claimed[0].Status != "processing" {
		t.Fatalf("claimed status = %q, want %q", claimed[0].Status, "processing")
	}

	// Mark delivered: processing -> delivered.
	c := &claimed[0]
	now := time.Now()
	c.Status = "delivered"
	c.Attempts = 1
	c.DeliveredAt = &now

	updated, err := st.UpdateClaimedNotificationDelivery(ctx, c)
	if err != nil || !updated {
		t.Fatalf("UpdateClaimedNotificationDelivery() error = %v, updated = %v", err, updated)
	}

	deliveries, err = st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if deliveries[0].Status != "delivered" {
		t.Fatalf("final status = %q, want %q", deliveries[0].Status, "delivered")
	}
}

func TestDeliveryStatusLifecycle_PendingToFailed(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-fail"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "fail-ch", json.RawMessage(`{"webhook_url":"https://fl"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventCostAnomaly, json.RawMessage(`{}`))
	if err := st.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

	claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("Claim error = %v, len = %d", err, len(claimed))
	}

	c := &claimed[0]
	c.Status = "failed"
	c.Attempts = 3
	c.LastError = "send failed after max retries"

	updated, err := st.UpdateClaimedNotificationDelivery(ctx, c)
	if err != nil || !updated {
		t.Fatalf("UpdateClaimedNotificationDelivery() error = %v, updated = %v", err, updated)
	}

	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if deliveries[0].Status != "failed" {
		t.Errorf("final status = %q, want %q", deliveries[0].Status, "failed")
	}
	if deliveries[0].LastError != "send failed after max retries" {
		t.Errorf("last_error = %q, want %q", deliveries[0].LastError, "send failed after max retries")
	}
}

// -- Retry flow tests --

func TestDeliveryRetryFlow(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-retry"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "retry-ch", json.RawMessage(`{"webhook_url":"https://rt"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
	if err := st.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

	// First attempt: claim, fail, set retry in the past so it becomes claimable again.
	claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("first claim error = %v, len = %d", err, len(claimed))
	}
	c := &claimed[0]
	c.Status = "pending"
	c.Attempts = 1
	c.LastError = "temporary failure"
	pastRetry := time.Now().Add(-1 * time.Second)
	c.NextRetryAt = &pastRetry

	updated, err := st.UpdateClaimedNotificationDelivery(ctx, c)
	if err != nil || !updated {
		t.Fatalf("first update error = %v, updated = %v", err, updated)
	}

	// Second attempt: the delivery should be claimable again because next_retry_at is in the past.
	claimed2, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	if err != nil {
		t.Fatalf("second claim error = %v", err)
	}
	if len(claimed2) != 1 {
		t.Fatalf("second claim returned %d deliveries, want 1", len(claimed2))
	}
	if claimed2[0].ID != d.ID {
		t.Errorf("second claim delivery ID = %q, want %q", claimed2[0].ID, d.ID)
	}
}

func TestDeliveryRetryNotClaimableBeforeNextRetryAt(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-retry-future"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "retry-future-ch", json.RawMessage(`{"webhook_url":"https://rf"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
	if err := st.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

	claimed, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim error = %v, len = %d", err, len(claimed))
	}

	c := &claimed[0]
	c.Status = "pending"
	c.Attempts = 1
	c.LastError = "temporary failure"
	futureRetry := time.Now().Add(1 * time.Hour)
	c.NextRetryAt = &futureRetry

	updated, err := st.UpdateClaimedNotificationDelivery(ctx, c)
	if err != nil || !updated {
		t.Fatalf("update error = %v, updated = %v", err, updated)
	}

	// The delivery should not be claimable because next_retry_at is in the future.
	claimed2, err := st.ClaimPendingNotificationDeliveries(ctx, 1, 2*time.Minute)
	if err != nil {
		t.Fatalf("second claim error = %v", err)
	}
	if len(claimed2) != 0 {
		t.Fatalf("second claim returned %d deliveries, want 0 (next_retry_at is in the future)", len(claimed2))
	}
}

// -- Worker integration with real DB --

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
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventSpendingLimitWarning, json.RawMessage(`{"org_id":"org-1"}`))
	if err := st.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

	sender := &fakeSender{}
	w := notification.NewWorker(st, &http.Client{})
	w.RegisterSender(domain.ChannelTypeSlack, sender)

	workerCtx, cancel := context.WithCancel(ctx)
	w.Start(workerCtx)

	// Wait for the worker to process (it processes immediately on start).
	time.Sleep(500 * time.Millisecond)
	cancel()
	w.Stop()

	if sender.callCount() != 1 {
		t.Fatalf("sender.Send() called %d times, want 1", sender.callCount())
	}

	// Verify the delivery is now delivered.
	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("ListNotificationDeliveries() returned %d, want 1", len(deliveries))
	}
	if deliveries[0].Status != "delivered" {
		t.Errorf("delivery status = %q, want %q", deliveries[0].Status, "delivered")
	}
}

func TestWorkerMarksDeliveryFailedAfterMaxAttempts(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-worker-fail"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "worker-fail-ch", json.RawMessage(`{"webhook_url":"https://wf"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
	d.MaxAttempts = 1 // Only one attempt allowed.
	if err := st.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

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
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("ListNotificationDeliveries() returned %d, want 1", len(deliveries))
	}
	if deliveries[0].Status != "failed" {
		t.Errorf("delivery status = %q, want %q", deliveries[0].Status, "failed")
	}
	if deliveries[0].Attempts != 1 {
		t.Errorf("delivery attempts = %d, want 1", deliveries[0].Attempts)
	}
}

var errSendFailed = fmt.Errorf("simulated send failure")

func TestWorkerSetsRetryOnFailureBelowMaxAttempts(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-worker-retry"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "worker-retry-ch", json.RawMessage(`{"webhook_url":"https://wr"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
	d.MaxAttempts = 3
	if err := st.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

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
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("ListNotificationDeliveries() returned %d, want 1", len(deliveries))
	}
	// After one failed attempt below max, status should be back to pending with a next_retry_at.
	if deliveries[0].Status != "pending" {
		t.Errorf("delivery status = %q, want %q", deliveries[0].Status, "pending")
	}
	if deliveries[0].Attempts != 1 {
		t.Errorf("delivery attempts = %d, want 1", deliveries[0].Attempts)
	}
	if deliveries[0].NextRetryAt == nil {
		t.Error("delivery next_retry_at is nil, want a future time for retry backoff")
	}
}

func TestDeleteChannelCascadesDeliveries(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-cascade"
	ch := makeChannel(projectID, domain.ChannelTypeSlack, "cascade-ch", json.RawMessage(`{"webhook_url":"https://cas"}`))
	if err := st.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d := makeDelivery(ch.ID, projectID, domain.NotificationEventBudgetThreshold, json.RawMessage(`{}`))
	if err := st.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

	if err := st.DeleteNotificationChannel(ctx, ch.ID, ch.ProjectID); err != nil {
		t.Fatalf("DeleteNotificationChannel() error = %v", err)
	}

	// Deliveries referencing this channel should be cascade-deleted.
	deliveries, err := st.ListNotificationDeliveries(ctx, projectID, 10, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(deliveries) != 0 {
		t.Fatalf("ListNotificationDeliveries() returned %d after cascade delete, want 0", len(deliveries))
	}
}
