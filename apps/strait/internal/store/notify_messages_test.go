package store

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestUpdateNotificationMessageStatus(t *testing.T) {
	t.Parallel()

	t.Run("empty toStatus returns field error", func(t *testing.T) {
		t.Parallel()

		q := New(&mockDBTX{})
		err := q.UpdateNotificationMessageStatus(context.Background(), "msg_1", "proj_1", domain.NotifyMessageStatusPending, "", nil)
		if err == nil {
			t.Fatal("UpdateNotificationMessageStatus() error = nil, want non-nil")
		}

		var fieldErr *domain.FieldError
		if !errors.As(err, &fieldErr) {
			t.Fatalf("UpdateNotificationMessageStatus() error = %v, want FieldError", err)
		}
		if fieldErr.Field != "status" {
			t.Fatalf("FieldError.Field = %q, want status", fieldErr.Field)
		}
	})

	t.Run("unsupported field returns field error", func(t *testing.T) {
		t.Parallel()

		q := New(&mockDBTX{})
		err := q.UpdateNotificationMessageStatus(context.Background(), "msg_1", "proj_1", domain.NotifyMessageStatusPending, domain.NotifyMessageStatusDelivered, map[string]any{"unsupported": "x"})
		if err == nil {
			t.Fatal("UpdateNotificationMessageStatus() error = nil, want non-nil")
		}

		var fieldErr *domain.FieldError
		if !errors.As(err, &fieldErr) {
			t.Fatalf("UpdateNotificationMessageStatus() error = %v, want FieldError", err)
		}
		if fieldErr.Field != "unsupported" {
			t.Fatalf("FieldError.Field = %q, want unsupported", fieldErr.Field)
		}
	})

	t.Run("conflict wraps sentinel", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("UPDATE 0"), nil
			},
		}
		q := New(db)

		err := q.UpdateNotificationMessageStatus(context.Background(), "msg_1", "proj_1", domain.NotifyMessageStatusPending, domain.NotifyMessageStatusDelivered, nil)
		if err == nil {
			t.Fatal("UpdateNotificationMessageStatus() error = nil, want non-nil")
		}
		if !errors.Is(err, ErrNotificationMessageStatusConflict) {
			t.Fatalf("UpdateNotificationMessageStatus() error = %v, want ErrNotificationMessageStatusConflict", err)
		}
	})

	t.Run("not found when no fromStatus", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("UPDATE 0"), nil
			},
		}
		q := New(db)

		err := q.UpdateNotificationMessageStatus(context.Background(), "msg_1", "proj_1", "", domain.NotifyMessageStatusDelivered, nil)
		if !errors.Is(err, ErrNotificationMessageNotFound) {
			t.Fatalf("UpdateNotificationMessageStatus() error = %v, want ErrNotificationMessageNotFound", err)
		}
	})
}
