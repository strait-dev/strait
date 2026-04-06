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
