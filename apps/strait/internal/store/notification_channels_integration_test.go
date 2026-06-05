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

	"github.com/stretchr/testify/require"
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
	require.NoError(t, q.CreateNotificationChannel(ctx, ch))
	require.NotEqual(t, "",

		ch.ID)
	require.False(t, ch.CreatedAt.
		IsZero())
	require.False(t, ch.UpdatedAt.
		IsZero())

	got, err := q.GetNotificationChannel(ctx, ch.ID, projectID)
	require.NoError(t, err)
	require.Equal(t, ch.ID,

		got.ID)
	require.Equal(t, projectID,

		got.ProjectID,
	)
	require.Equal(t, domain.
		ChannelTypeWebhook,

		got.ChannelType)
	require.Equal(t, ch.Name,

		got.Name,
	)
	require.True(t, got.Enabled)

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
	require.NoError(t, q.CreateNotificationChannel(ctx, ch))
	require.Equal(t, customID,

		ch.ID)

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
	require.Error(t, q.CreateNotificationChannel(ctx, ch))

	if _, err := q.GetNotificationChannel(ctx, ch.ID, ch.ProjectID); !errors.Is(err, store.ErrNotificationChannelNotFound) {
		require.Failf(t, "test failure",

			"GetNotificationChannel() error = %v, want ErrNotificationChannelNotFound", err)
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
	require.NoError(t, q.CreateNotificationChannel(ctx, ch))

	q.SetSecretEncryptionKey("short")
	ch.Config = []byte(`{"url":"https://example.com/hooks/updated"}`)
	require.Error(t, q.UpdateNotificationChannel(ctx, ch))

}

func TestGetNotificationChannel_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetNotificationChannel(ctx, newID(), "proj-missing")
	require.True(t, errors.Is(err, store.
		ErrNotificationChannelNotFound,
	))

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
		require.NoError(t, q.CreateNotificationChannel(ctx, ch))

	}

	// Disabled channel also appears in ListNotificationChannels (but not ListEnabled).
	disabled := &domain.NotificationChannel{
		ProjectID:   projectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "disabled",
		Config:      []byte(`{}`),
		Enabled:     false,
	}
	require.NoError(t, q.CreateNotificationChannel(ctx, disabled))

	// Another project.
	other := &domain.NotificationChannel{
		ProjectID:   otherProjectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "other",
		Config:      []byte(`{}`),
		Enabled:     true,
	}
	require.NoError(t, q.CreateNotificationChannel(ctx, other))

	channels, err := q.ListNotificationChannels(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, channels,

		4)

	for _, ch := range channels {
		require.Equal(t, projectID,

			ch.ProjectID,
		)

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
	require.NoError(t, q.CreateNotificationChannel(ctx, enabled))

	disabled := &domain.NotificationChannel{
		ProjectID:   projectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "disabled",
		Config:      []byte(`{}`),
		Enabled:     false,
	}
	require.NoError(t, q.CreateNotificationChannel(ctx, disabled))

	channels, err := q.ListEnabledNotificationChannels(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, channels,

		1)
	require.Equal(t, enabled.
		ID, channels[0].ID)

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
	require.NoError(t, q.CreateNotificationChannel(ctx, chA))

	chB := &domain.NotificationChannel{
		ProjectID:   projB,
		ChannelType: domain.ChannelTypeEmail,
		Name:        "b-ch",
		Config:      []byte(`{}`),
		Enabled:     true,
	}
	require.NoError(t, q.CreateNotificationChannel(ctx, chB))

	// Disabled in projB should not appear.
	disabledB := &domain.NotificationChannel{
		ProjectID:   projB,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "disabled-b",
		Config:      []byte(`{}`),
		Enabled:     false,
	}
	require.NoError(t, q.CreateNotificationChannel(ctx, disabledB))

	result, err := q.ListEnabledNotificationChannelsByProjectIDs(ctx, []string{projA, projB})
	require.NoError(t, err)
	require.Len(t, result[projA], 1)
	require.Len(t, result[projB], 1)

	// Empty input.
	empty, err := q.ListEnabledNotificationChannelsByProjectIDs(ctx, nil)
	require.NoError(t, err)
	require.Len(t, empty, 0)

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
	require.NoError(t, q.CreateNotificationChannel(ctx, ch))

	origUpdatedAt := ch.UpdatedAt

	ch.Name = "updated"
	ch.ChannelType = domain.ChannelTypeEmail
	ch.Config = []byte(`{"to":"new@example.com"}`)
	ch.Enabled = false
	require.NoError(t, q.UpdateNotificationChannel(ctx, ch))
	require.True(t, ch.UpdatedAt.
		After(origUpdatedAt))

	got, err := q.GetNotificationChannel(ctx, ch.ID, projectID)
	require.NoError(t, err)
	require.Equal(t, "updated",

		got.Name,
	)
	require.Equal(t, domain.
		ChannelTypeEmail,
		got.
			ChannelType)
	require.False(t, got.Enabled)

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
		require.Failf(t, "test failure",

			"UpdateNotificationChannel(missing) error = %v, want ErrNotificationChannelNotFound", err)
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
	require.NoError(t, q.CreateNotificationChannel(ctx, ch))
	require.NoError(t, q.DeleteNotificationChannel(ctx, ch.ID, projectID))

	_, err := q.GetNotificationChannel(ctx, ch.ID, projectID)
	require.True(t, errors.Is(err, store.
		ErrNotificationChannelNotFound,
	))

}

func TestDeleteNotificationChannel_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	if err := q.DeleteNotificationChannel(ctx, newID(), "proj-missing"); !errors.Is(err, store.ErrNotificationChannelNotFound) {
		require.Failf(t, "test failure",

			"DeleteNotificationChannel(missing) error = %v, want ErrNotificationChannelNotFound", err)
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
	require.NoError(t, q.CreateNotificationChannel(ctx, ch))

	d := &domain.NotificationDelivery{
		ChannelID:   ch.ID,
		ProjectID:   projectID,
		EventType:   domain.NotificationEventApprovalRequested,
		Payload:     json.RawMessage(`{"step":"review"}`),
		Status:      "pending",
		MaxAttempts: 3,
	}
	require.NoError(t, q.CreateNotificationDelivery(ctx, d))
	require.NotEqual(t, "",

		d.ID)
	require.False(t, d.CreatedAt.
		IsZero())

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
	require.NoError(t, q.CreateNotificationChannel(ctx, ch))

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
		require.NoError(t, q.CreateNotificationDelivery(ctx, d))

	}

	var count int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notification_deliveries WHERE dedupe_key = $1`,

		"budget:org-1:80:2026-05-16:"+
			projectID+":"+ch.ID).Scan(&count))
	require.EqualValues(t, 1, count)

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
	require.NoError(t, q.CreateNotificationChannel(ctx, ch))

	d := &domain.NotificationDelivery{
		ChannelID:   ch.ID,
		ProjectID:   projectID,
		EventType:   domain.NotificationEventApprovalRequested,
		Payload:     json.RawMessage(`{}`),
		Status:      "pending",
		MaxAttempts: 3,
	}
	require.NoError(t, q.CreateNotificationDelivery(ctx, d))

	// Claim.
	claimed, err := q.ClaimPendingNotificationDeliveries(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.GreaterOrEqual(
		t,
		len(claimed), 1)

	// Find our specific delivery in the claimed batch.
	var found *domain.NotificationDelivery
	for i := range claimed {
		if claimed[i].ChannelID == ch.ID {
			found = &claimed[i]
			break
		}
	}
	require.NotNil(t, found)
	require.Equal(t, "processing",

		found.
			Status)
	require.NotEqual(t, "",

		found.ClaimToken,
	)
	require.NotNil(t, found.
		LeaseExpiry,
	)

	// Second claim should not return our delivery again (already processing).
	second, err := q.ClaimPendingNotificationDeliveries(ctx, 10, time.Minute)
	require.NoError(t, err)

	for _, s := range second {
		require.NotEqual(t, ch.
			ID,
			s.ChannelID,
		)

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
	require.NoError(t, q.CreateNotificationChannel(ctx, ch))

	d := &domain.NotificationDelivery{
		ChannelID:   ch.ID,
		ProjectID:   projectID,
		EventType:   domain.NotificationEventBudgetThreshold,
		Payload:     json.RawMessage(`{}`),
		Status:      "pending",
		MaxAttempts: 3,
	}
	require.NoError(t, q.CreateNotificationDelivery(ctx, d))

	claimed, err := q.ClaimPendingNotificationDeliveries(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed,

		1)

	// Complete the delivery.
	now := time.Now().UTC()
	claimed[0].Status = "delivered"
	claimed[0].Attempts = 1
	claimed[0].DeliveredAt = &now
	updated, err := q.UpdateClaimedNotificationDelivery(ctx, &claimed[0])
	require.NoError(t, err)
	require.True(t, updated)

	// Stale token should not update.
	claimed[0].ClaimToken = "stale-token"
	updated2, err := q.UpdateClaimedNotificationDelivery(ctx, &claimed[0])
	require.NoError(t, err)
	require.False(t, updated2)

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
	require.NoError(t, q.CreateNotificationChannel(ctx, ch))

	otherCh := &domain.NotificationChannel{
		ProjectID:   otherProjectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "other-ch",
		Config:      []byte(`{}`),
		Enabled:     true,
	}
	require.NoError(t, q.CreateNotificationChannel(ctx, otherCh))

	for range 3 {
		d := &domain.NotificationDelivery{
			ChannelID:   ch.ID,
			ProjectID:   projectID,
			EventType:   domain.NotificationEventApprovalRequested,
			Payload:     json.RawMessage(`{}`),
			Status:      "pending",
			MaxAttempts: 3,
		}
		require.NoError(t, q.CreateNotificationDelivery(ctx, d))

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
	require.NoError(t, q.CreateNotificationDelivery(ctx, otherD))

	deliveries, err := q.ListNotificationDeliveries(ctx, projectID, 100, nil)
	require.NoError(t, err)
	require.Len(t, deliveries,

		3)

	for _, d := range deliveries {
		require.Equal(t, projectID,

			d.ProjectID,
		)

	}

	for i := 1; i < len(deliveries); i++ {
		require.False(t, deliveries[i-1].
			CreatedAt.Before(deliveries[i].CreatedAt))

	}

	// Cursor pagination.
	page1, err := q.ListNotificationDeliveries(ctx, projectID, 2, nil)
	require.NoError(t, err)
	require.Len(t, page1, 2)

	cursor := page1[1].CreatedAt
	page2, err := q.ListNotificationDeliveries(ctx, projectID, 2, &cursor)
	require.NoError(t, err)
	require.Len(t, page2, 1)

}
