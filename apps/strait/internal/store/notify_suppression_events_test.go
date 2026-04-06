package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestCreateNotifySuppressionEvent(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					*dest[0].(*time.Time) = time.Now().UTC()
					return nil
				}}
			},
		}
		q := New(db)
		event := &domain.NotifySuppressionEvent{
			ProjectID:     "proj_1",
			RecipientType: domain.NotifyRecipientTypeSubscriber,
			RecipientID:   "sub_1",
			Scope:         "global",
			Channel:       "email",
			Action:        domain.NotifySuppressionActionSuppressed,
			Source:        domain.NotifySuppressionSourceProviderCallback,
		}
		if err := q.CreateNotifySuppressionEvent(context.Background(), event); err != nil {
			t.Fatalf("CreateNotifySuppressionEvent() error = %v", err)
		}
		if event.ID == "" {
			t.Fatal("CreateNotifySuppressionEvent() did not set ID")
		}
	})

	t.Run("query error", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
				return &mockRow{scanFn: func(_ ...any) error {
					return errors.New("boom")
				}}
			},
		}
		q := New(db)
		event := &domain.NotifySuppressionEvent{
			ProjectID:     "proj_1",
			RecipientType: domain.NotifyRecipientTypeSubscriber,
			RecipientID:   "sub_1",
			Channel:       "email",
			Action:        domain.NotifySuppressionActionSuppressed,
			Source:        domain.NotifySuppressionSourceProviderCallback,
		}
		if err := q.CreateNotifySuppressionEvent(context.Background(), event); err == nil {
			t.Fatal("CreateNotifySuppressionEvent() error = nil, want non-nil")
		}
	})
}

func TestListNotifySuppressionEvents(t *testing.T) {
	t.Parallel()

	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, errors.New("boom")
		},
	}
	q := New(db)
	_, err := q.ListNotifySuppressionEvents(context.Background(), "proj_1", domain.NotifyRecipientTypeSubscriber, "sub_1", 10, nil)
	if err == nil {
		t.Fatal("ListNotifySuppressionEvents() error = nil, want non-nil")
	}
}

func TestGetLatestNotifySuppressionEvent(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					*dest[0].(*string) = "evt_1"
					*dest[1].(*string) = "proj_1"
					*dest[2].(*string) = domain.NotifyRecipientTypeSubscriber
					*dest[3].(*string) = "sub_1"
					*dest[4].(*string) = "global"
					*dest[5].(*string) = "email"
					*dest[6].(*string) = domain.NotifySuppressionActionSuppressed
					reason := "provider_callback:email.bounced"
					*dest[7].(**string) = &reason
					*dest[8].(*string) = domain.NotifySuppressionSourceProviderCallback
					*dest[9].(*[]byte) = []byte(`{"callback_id":"cb_1"}`)
					*dest[10].(*time.Time) = now
					return nil
				}}
			},
		}
		q := New(db)
		event, err := q.GetLatestNotifySuppressionEvent(context.Background(), "proj_1", domain.NotifyRecipientTypeSubscriber, "sub_1", "", "email")
		if err != nil {
			t.Fatalf("GetLatestNotifySuppressionEvent() error = %v", err)
		}
		if event.Scope != "global" {
			t.Fatalf("Scope = %q, want global", event.Scope)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
				return &mockRow{scanFn: func(_ ...any) error {
					return pgx.ErrNoRows
				}}
			},
		}
		q := New(db)
		_, err := q.GetLatestNotifySuppressionEvent(context.Background(), "proj_1", domain.NotifyRecipientTypeSubscriber, "sub_1", "global", "email")
		if !errors.Is(err, ErrNotifySuppressionEventNotFound) {
			t.Fatalf("GetLatestNotifySuppressionEvent() error = %v, want ErrNotifySuppressionEventNotFound", err)
		}
	})
}

func TestEnableNotificationChannelPreference_QueryError(t *testing.T) {
	t.Parallel()

	db := &mockDBTX{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, errors.New("boom")
		},
	}
	q := New(db)
	if err := q.EnableNotificationChannelPreference(context.Background(), domain.NotifyRecipientTypeSubscriber, "sub_1", "global", "email"); err == nil {
		t.Fatal("EnableNotificationChannelPreference() error = nil, want non-nil")
	}
}
