//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestCreateNotificationChannel(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-create-nc-" + newID()
	ch := &domain.NotificationChannel{
		ProjectID:   projectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "ops-webhook",
		Config:      []byte(`{"url":"https://example.com/hooks/ops"}`),
		Enabled:     true,
	}

	if err := q.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}
	if ch.ID == "" {
		t.Fatal("CreateNotificationChannel() did not set ID")
	}
	if ch.CreatedAt.IsZero() {
		t.Fatal("CreateNotificationChannel() did not set CreatedAt")
	}
	if ch.UpdatedAt.IsZero() {
		t.Fatal("CreateNotificationChannel() did not set UpdatedAt")
	}

	got, err := q.GetNotificationChannel(ctx, ch.ID, projectID)
	if err != nil {
		t.Fatalf("GetNotificationChannel() error = %v", err)
	}
	if got.ID != ch.ID {
		t.Fatalf("ID = %q, want %q", got.ID, ch.ID)
	}
	if got.ProjectID != projectID {
		t.Fatalf("ProjectID = %q, want %q", got.ProjectID, projectID)
	}
	if got.ChannelType != domain.ChannelTypeWebhook {
		t.Fatalf("ChannelType = %q, want %q", got.ChannelType, domain.ChannelTypeWebhook)
	}
	if got.Name != ch.Name {
		t.Fatalf("Name = %q, want %q", got.Name, ch.Name)
	}
	if !got.Enabled {
		t.Fatal("Enabled = false, want true")
	}
}

func TestCreateNotificationChannel_CustomID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	customID := newID()
	ch := &domain.NotificationChannel{
		ID:          customID,
		ProjectID:   "proj-nc-custom-" + newID(),
		ChannelType: domain.ChannelTypeEmail,
		Name:        "email-ch",
		Config:      []byte(`{"to":"team@example.com"}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}
	if ch.ID != customID {
		t.Fatalf("ID = %q, want %q", ch.ID, customID)
	}
}

func TestCreateNotificationChannel_InvalidEncryptionKeyFailsClosed(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("short")
	mustClean(t, ctx)

	ch := &domain.NotificationChannel{
		ProjectID:   "proj-create-nc-invalid-key-" + newID(),
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "ops-webhook",
		Config:      []byte(`{"url":"https://example.com/hooks/ops"}`),
		Enabled:     true,
	}

	if err := q.CreateNotificationChannel(ctx, ch); err == nil {
		t.Fatal("CreateNotificationChannel() error = nil, want encryption failure")
	}
	if _, err := q.GetNotificationChannel(ctx, ch.ID, ch.ProjectID); !errors.Is(err, store.ErrNotificationChannelNotFound) {
		t.Fatalf("GetNotificationChannel() error = %v, want ErrNotificationChannelNotFound", err)
	}
}

func TestUpdateNotificationChannel_InvalidEncryptionKeyFailsClosed(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	ch := &domain.NotificationChannel{
		ProjectID:   "proj-update-nc-invalid-key-" + newID(),
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "ops-webhook",
		Config:      []byte(`{"url":"https://example.com/hooks/ops"}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	q.SetSecretEncryptionKey("short")
	ch.Config = []byte(`{"url":"https://example.com/hooks/updated"}`)
	if err := q.UpdateNotificationChannel(ctx, ch); err == nil {
		t.Fatal("UpdateNotificationChannel() error = nil, want encryption failure")
	}
}

func TestGetNotificationChannel_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetNotificationChannel(ctx, newID(), "proj-missing")
	if !errors.Is(err, store.ErrNotificationChannelNotFound) {
		t.Fatalf("GetNotificationChannel(missing) error = %v, want ErrNotificationChannelNotFound", err)
	}
}

func TestListNotificationChannels(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-list-nc-" + newID()
	otherProjectID := "proj-list-nc-other-" + newID()

	for range 3 {
		ch := &domain.NotificationChannel{
			ProjectID:   projectID,
			ChannelType: domain.ChannelTypeWebhook,
			Name:        "ch-" + newID(),
			Config:      []byte(`{"url":"https://example.com"}`),
			Enabled:     true,
		}
		if err := q.CreateNotificationChannel(ctx, ch); err != nil {
			t.Fatalf("CreateNotificationChannel() error = %v", err)
		}
	}

	// Disabled channel also appears in ListNotificationChannels (but not ListEnabled).
	disabled := &domain.NotificationChannel{
		ProjectID:   projectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "disabled",
		Config:      []byte(`{}`),
		Enabled:     false,
	}
	if err := q.CreateNotificationChannel(ctx, disabled); err != nil {
		t.Fatalf("CreateNotificationChannel(disabled) error = %v", err)
	}

	// Another project.
	other := &domain.NotificationChannel{
		ProjectID:   otherProjectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "other",
		Config:      []byte(`{}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, other); err != nil {
		t.Fatalf("CreateNotificationChannel(other) error = %v", err)
	}

	channels, err := q.ListNotificationChannels(ctx, projectID)
	if err != nil {
		t.Fatalf("ListNotificationChannels() error = %v", err)
	}
	if len(channels) != 4 {
		t.Fatalf("len = %d, want 4 (including disabled)", len(channels))
	}
	for _, ch := range channels {
		if ch.ProjectID != projectID {
			t.Fatalf("ProjectID = %q, want %q", ch.ProjectID, projectID)
		}
	}
}

func TestListEnabledNotificationChannels(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-enabled-nc-" + newID()

	enabled := &domain.NotificationChannel{
		ProjectID:   projectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "enabled",
		Config:      []byte(`{"url":"https://example.com"}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, enabled); err != nil {
		t.Fatalf("CreateNotificationChannel(enabled) error = %v", err)
	}

	disabled := &domain.NotificationChannel{
		ProjectID:   projectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "disabled",
		Config:      []byte(`{}`),
		Enabled:     false,
	}
	if err := q.CreateNotificationChannel(ctx, disabled); err != nil {
		t.Fatalf("CreateNotificationChannel(disabled) error = %v", err)
	}

	channels, err := q.ListEnabledNotificationChannels(ctx, projectID)
	if err != nil {
		t.Fatalf("ListEnabledNotificationChannels() error = %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("len = %d, want 1", len(channels))
	}
	if channels[0].ID != enabled.ID {
		t.Fatalf("ID = %q, want %q", channels[0].ID, enabled.ID)
	}
}

func TestListEnabledNotificationChannelsByProjectIDs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projA := "proj-nc-multi-a-" + newID()
	projB := "proj-nc-multi-b-" + newID()

	chA := &domain.NotificationChannel{
		ProjectID:   projA,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "a-ch",
		Config:      []byte(`{}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, chA); err != nil {
		t.Fatalf("CreateNotificationChannel(a) error = %v", err)
	}

	chB := &domain.NotificationChannel{
		ProjectID:   projB,
		ChannelType: domain.ChannelTypeEmail,
		Name:        "b-ch",
		Config:      []byte(`{}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, chB); err != nil {
		t.Fatalf("CreateNotificationChannel(b) error = %v", err)
	}

	// Disabled in projB should not appear.
	disabledB := &domain.NotificationChannel{
		ProjectID:   projB,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "disabled-b",
		Config:      []byte(`{}`),
		Enabled:     false,
	}
	if err := q.CreateNotificationChannel(ctx, disabledB); err != nil {
		t.Fatalf("CreateNotificationChannel(disabled-b) error = %v", err)
	}

	result, err := q.ListEnabledNotificationChannelsByProjectIDs(ctx, []string{projA, projB})
	if err != nil {
		t.Fatalf("ListEnabledNotificationChannelsByProjectIDs() error = %v", err)
	}
	if len(result[projA]) != 1 {
		t.Fatalf("projA count = %d, want 1", len(result[projA]))
	}
	if len(result[projB]) != 1 {
		t.Fatalf("projB count = %d, want 1", len(result[projB]))
	}

	// Empty input.
	empty, err := q.ListEnabledNotificationChannelsByProjectIDs(ctx, nil)
	if err != nil {
		t.Fatalf("ListEnabledNotificationChannelsByProjectIDs(nil) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty len = %d, want 0", len(empty))
	}
}

func TestUpdateNotificationChannel(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-update-nc-" + newID()
	ch := &domain.NotificationChannel{
		ProjectID:   projectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "original",
		Config:      []byte(`{"url":"https://old.example.com"}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}
	origUpdatedAt := ch.UpdatedAt

	ch.Name = "updated"
	ch.ChannelType = domain.ChannelTypeEmail
	ch.Config = []byte(`{"to":"new@example.com"}`)
	ch.Enabled = false

	if err := q.UpdateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("UpdateNotificationChannel() error = %v", err)
	}
	if !ch.UpdatedAt.After(origUpdatedAt) {
		t.Fatal("UpdatedAt was not advanced")
	}

	got, err := q.GetNotificationChannel(ctx, ch.ID, projectID)
	if err != nil {
		t.Fatalf("GetNotificationChannel(updated) error = %v", err)
	}
	if got.Name != "updated" {
		t.Fatalf("Name = %q, want %q", got.Name, "updated")
	}
	if got.ChannelType != domain.ChannelTypeEmail {
		t.Fatalf("ChannelType = %q, want %q", got.ChannelType, domain.ChannelTypeEmail)
	}
	if got.Enabled {
		t.Fatal("Enabled = true, want false")
	}
}

func TestUpdateNotificationChannel_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	ch := &domain.NotificationChannel{
		ID:          newID(),
		ProjectID:   "proj-nc-missing",
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "ghost",
		Config:      []byte(`{}`),
		Enabled:     true,
	}
	if err := q.UpdateNotificationChannel(ctx, ch); !errors.Is(err, store.ErrNotificationChannelNotFound) {
		t.Fatalf("UpdateNotificationChannel(missing) error = %v, want ErrNotificationChannelNotFound", err)
	}
}

func TestDeleteNotificationChannel(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-delete-nc-" + newID()
	ch := &domain.NotificationChannel{
		ProjectID:   projectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "delete-me",
		Config:      []byte(`{}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	if err := q.DeleteNotificationChannel(ctx, ch.ID, projectID); err != nil {
		t.Fatalf("DeleteNotificationChannel() error = %v", err)
	}

	_, err := q.GetNotificationChannel(ctx, ch.ID, projectID)
	if !errors.Is(err, store.ErrNotificationChannelNotFound) {
		t.Fatalf("GetNotificationChannel(deleted) error = %v, want ErrNotificationChannelNotFound", err)
	}
}

func TestDeleteNotificationChannel_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	if err := q.DeleteNotificationChannel(ctx, newID(), "proj-missing"); !errors.Is(err, store.ErrNotificationChannelNotFound) {
		t.Fatalf("DeleteNotificationChannel(missing) error = %v, want ErrNotificationChannelNotFound", err)
	}
}

func TestCreateNotificationDelivery(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-create-nd-" + newID()
	ch := &domain.NotificationChannel{
		ProjectID:   projectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "ch",
		Config:      []byte(`{}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d := &domain.NotificationDelivery{
		ChannelID:   ch.ID,
		ProjectID:   projectID,
		EventType:   domain.NotificationEventApprovalRequested,
		Payload:     json.RawMessage(`{"step":"review"}`),
		Status:      "pending",
		MaxAttempts: 3,
	}

	if err := q.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}
	if d.ID == "" {
		t.Fatal("CreateNotificationDelivery() did not set ID")
	}
	if d.CreatedAt.IsZero() {
		t.Fatal("CreateNotificationDelivery() did not set CreatedAt")
	}
}

func TestCreateNotificationDelivery_DedupeKeySkipsDuplicate(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-create-nd-dedupe-" + newID()
	ch := &domain.NotificationChannel{
		ProjectID:   projectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "ch",
		Config:      []byte(`{}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	for i := 0; i < 2; i++ {
		d := &domain.NotificationDelivery{
			ChannelID:   ch.ID,
			ProjectID:   projectID,
			EventType:   domain.NotificationEventSpendingLimitWarning,
			Payload:     json.RawMessage(`{"threshold":80}`),
			Status:      "pending",
			MaxAttempts: 3,
			DedupeKey:   "budget:org-1:80:2026-05-16:" + projectID + ":" + ch.ID,
		}
		if err := q.CreateNotificationDelivery(ctx, d); err != nil {
			t.Fatalf("CreateNotificationDelivery(%d) error = %v", i, err)
		}
	}

	var count int
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notification_deliveries WHERE dedupe_key = $1`,
		"budget:org-1:80:2026-05-16:"+projectID+":"+ch.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count deduped deliveries: %v", err)
	}
	if count != 1 {
		t.Fatalf("deduped delivery count = %d, want 1", count)
	}
}

func TestClaimPendingNotificationDeliveries(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-claim-nd-" + newID()
	ch := &domain.NotificationChannel{
		ProjectID:   projectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "ch",
		Config:      []byte(`{}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d := &domain.NotificationDelivery{
		ChannelID:   ch.ID,
		ProjectID:   projectID,
		EventType:   domain.NotificationEventApprovalRequested,
		Payload:     json.RawMessage(`{}`),
		Status:      "pending",
		MaxAttempts: 3,
	}
	if err := q.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

	// Claim.
	claimed, err := q.ClaimPendingNotificationDeliveries(ctx, 10, time.Minute)
	if err != nil {
		t.Fatalf("ClaimPendingNotificationDeliveries() error = %v", err)
	}
	if len(claimed) < 1 {
		t.Fatalf("claimed len = %d, want >= 1", len(claimed))
	}
	// Find our specific delivery in the claimed batch.
	var found *domain.NotificationDelivery
	for i := range claimed {
		if claimed[i].ChannelID == ch.ID {
			found = &claimed[i]
			break
		}
	}
	if found == nil {
		t.Fatal("our delivery not found in claimed batch")
	}
	if found.Status != "processing" {
		t.Fatalf("claimed status = %q, want processing", found.Status)
	}
	if found.ClaimToken == "" {
		t.Fatal("claim token = empty, want non-empty")
	}
	if found.LeaseExpiry == nil {
		t.Fatal("lease expiry = nil, want non-nil")
	}

	// Second claim should not return our delivery again (already processing).
	second, err := q.ClaimPendingNotificationDeliveries(ctx, 10, time.Minute)
	if err != nil {
		t.Fatalf("ClaimPendingNotificationDeliveries(second) error = %v", err)
	}
	for _, s := range second {
		if s.ChannelID == ch.ID {
			t.Fatal("our delivery was re-claimed, should have been excluded (already processing)")
		}
	}
}

func TestUpdateClaimedNotificationDelivery(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-update-nd-" + newID()
	ch := &domain.NotificationChannel{
		ProjectID:   projectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "ch",
		Config:      []byte(`{}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	d := &domain.NotificationDelivery{
		ChannelID:   ch.ID,
		ProjectID:   projectID,
		EventType:   domain.NotificationEventBudgetThreshold,
		Payload:     json.RawMessage(`{}`),
		Status:      "pending",
		MaxAttempts: 3,
	}
	if err := q.CreateNotificationDelivery(ctx, d); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

	claimed, err := q.ClaimPendingNotificationDeliveries(ctx, 1, time.Minute)
	if err != nil {
		t.Fatalf("ClaimPendingNotificationDeliveries() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed len = %d, want 1", len(claimed))
	}

	// Complete the delivery.
	now := time.Now().UTC()
	claimed[0].Status = "delivered"
	claimed[0].Attempts = 1
	claimed[0].DeliveredAt = &now
	updated, err := q.UpdateClaimedNotificationDelivery(ctx, &claimed[0])
	if err != nil {
		t.Fatalf("UpdateClaimedNotificationDelivery() error = %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	// Stale token should not update.
	claimed[0].ClaimToken = "stale-token"
	updated2, err := q.UpdateClaimedNotificationDelivery(ctx, &claimed[0])
	if err != nil {
		t.Fatalf("UpdateClaimedNotificationDelivery(stale) error = %v", err)
	}
	if updated2 {
		t.Fatal("updated(stale) = true, want false")
	}
}

func TestListNotificationDeliveries(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-list-nd-" + newID()
	otherProjectID := "proj-list-nd-other-" + newID()

	ch := &domain.NotificationChannel{
		ProjectID:   projectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "ch",
		Config:      []byte(`{}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, ch); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	otherCh := &domain.NotificationChannel{
		ProjectID:   otherProjectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "other-ch",
		Config:      []byte(`{}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, otherCh); err != nil {
		t.Fatalf("CreateNotificationChannel(other) error = %v", err)
	}

	for range 3 {
		d := &domain.NotificationDelivery{
			ChannelID:   ch.ID,
			ProjectID:   projectID,
			EventType:   domain.NotificationEventApprovalRequested,
			Payload:     json.RawMessage(`{}`),
			Status:      "pending",
			MaxAttempts: 3,
		}
		if err := q.CreateNotificationDelivery(ctx, d); err != nil {
			t.Fatalf("CreateNotificationDelivery() error = %v", err)
		}
	}

	// Other project delivery.
	otherD := &domain.NotificationDelivery{
		ChannelID:   otherCh.ID,
		ProjectID:   otherProjectID,
		EventType:   domain.NotificationEventApprovalRequested,
		Payload:     json.RawMessage(`{}`),
		Status:      "pending",
		MaxAttempts: 3,
	}
	if err := q.CreateNotificationDelivery(ctx, otherD); err != nil {
		t.Fatalf("CreateNotificationDelivery(other) error = %v", err)
	}

	deliveries, err := q.ListNotificationDeliveries(ctx, projectID, 100, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(deliveries) != 3 {
		t.Fatalf("len = %d, want 3", len(deliveries))
	}
	for _, d := range deliveries {
		if d.ProjectID != projectID {
			t.Fatalf("ProjectID = %q, want %q", d.ProjectID, projectID)
		}
	}

	// Verify DESC ordering.
	for i := 1; i < len(deliveries); i++ {
		if deliveries[i-1].CreatedAt.Before(deliveries[i].CreatedAt) {
			t.Fatalf("deliveries not DESC at index %d", i)
		}
	}

	// Cursor pagination.
	page1, err := q.ListNotificationDeliveries(ctx, projectID, 2, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries(page1) error = %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}
	cursor := page1[1].CreatedAt
	page2, err := q.ListNotificationDeliveries(ctx, projectID, 2, &cursor)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries(page2) error = %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2 len = %d, want 1", len(page2))
	}
}
