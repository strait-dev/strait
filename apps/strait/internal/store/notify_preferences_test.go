package store

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestDisableNotificationChannelPreference(t *testing.T) {
	t.Parallel()

	t.Run("empty channel returns field error", func(t *testing.T) {
		t.Parallel()

		q := New(&mockDBTX{})
		err := q.DisableNotificationChannelPreference(context.Background(), domain.NotifyRecipientTypeSubscriber, "sub_1", "global", "")
		if err == nil {
			t.Fatal("DisableNotificationChannelPreference() error = nil, want non-nil")
		}

		var fieldErr *domain.FieldError
		if !errors.As(err, &fieldErr) {
			t.Fatalf("DisableNotificationChannelPreference() error = %v, want FieldError", err)
		}
		if fieldErr.Field != "channel" {
			t.Fatalf("FieldError.Field = %q, want channel", fieldErr.Field)
		}
	})

	t.Run("defaults empty scope to global", func(t *testing.T) {
		t.Parallel()

		capturedArgs := []any{}
		db := &mockDBTX{
			execFn: func(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
				capturedArgs = append(capturedArgs, args...)
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			},
		}
		q := New(db)

		err := q.DisableNotificationChannelPreference(context.Background(), domain.NotifyRecipientTypeSubscriber, "sub_1", "", "email")
		if err != nil {
			t.Fatalf("DisableNotificationChannelPreference() error = %v", err)
		}
		if len(capturedArgs) < 3 {
			t.Fatalf("captured args len = %d, want >= 3", len(capturedArgs))
		}
		if scope, ok := capturedArgs[2].(string); !ok || scope != "global" {
			t.Fatalf("scope arg = %#v, want global", capturedArgs[2])
		}
	})

	t.Run("exec error wraps", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, errors.New("boom")
			},
		}
		q := New(db)

		err := q.DisableNotificationChannelPreference(context.Background(), domain.NotifyRecipientTypeSubscriber, "sub_1", "global", "email")
		if err == nil {
			t.Fatal("DisableNotificationChannelPreference() error = nil, want non-nil")
		}
	})
}

func TestEnableNotificationChannelPreference(t *testing.T) {
	t.Parallel()

	capturedArgs := []any{}
	db := &mockDBTX{
		execFn: func(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
			capturedArgs = append(capturedArgs, args...)
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	q := New(db)

	err := q.EnableNotificationChannelPreference(context.Background(), domain.NotifyRecipientTypeSubscriber, "sub_1", "", "email")
	if err != nil {
		t.Fatalf("EnableNotificationChannelPreference() error = %v", err)
	}
	if len(capturedArgs) < 5 {
		t.Fatalf("captured args len = %d, want >= 5", len(capturedArgs))
	}
	if enabled, ok := capturedArgs[4].(string); !ok || enabled != "true" {
		t.Fatalf("enabled arg = %#v, want true", capturedArgs[4])
	}
}
