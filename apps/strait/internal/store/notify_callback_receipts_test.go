package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestRecordNotifyProviderCallbackReceipt(t *testing.T) {
	t.Parallel()

	t.Run("inserted", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					*dest[0].(*string) = "rcpt_1"
					return nil
				}}
			},
		}

		q := New(db)
		inserted, err := q.RecordNotifyProviderCallbackReceipt(
			context.Background(),
			"proj_1",
			"provider_1",
			"resend",
			"cb_1",
			"email.delivered",
			"msg_1",
			"hash1",
			time.Now().UTC().Add(time.Hour),
		)
		if err != nil {
			t.Fatalf("RecordNotifyProviderCallbackReceipt() error = %v", err)
		}
		if !inserted {
			t.Fatal("RecordNotifyProviderCallbackReceipt() inserted = false, want true")
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
				return &mockRow{scanFn: func(_ ...any) error {
					return pgx.ErrNoRows
				}}
			},
		}

		q := New(db)
		inserted, err := q.RecordNotifyProviderCallbackReceipt(
			context.Background(),
			"proj_1",
			"provider_1",
			"resend",
			"cb_1",
			"email.delivered",
			"msg_1",
			"hash1",
			time.Now().UTC().Add(time.Hour),
		)
		if err != nil {
			t.Fatalf("RecordNotifyProviderCallbackReceipt() error = %v", err)
		}
		if inserted {
			t.Fatal("RecordNotifyProviderCallbackReceipt() inserted = true, want false")
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
		_, err := q.RecordNotifyProviderCallbackReceipt(
			context.Background(),
			"proj_1",
			"provider_1",
			"resend",
			"cb_1",
			"email.delivered",
			"msg_1",
			"hash1",
			time.Now().UTC().Add(time.Hour),
		)
		if err == nil {
			t.Fatal("RecordNotifyProviderCallbackReceipt() error = nil, want non-nil")
		}
	})
}

func TestDeleteNotifyProviderCallbackReceipt(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("DELETE 1"), nil
			},
		}
		q := New(db)
		if err := q.DeleteNotifyProviderCallbackReceipt(context.Background(), "proj_1", "provider_1", "cb_1"); err != nil {
			t.Fatalf("DeleteNotifyProviderCallbackReceipt() error = %v", err)
		}
	})

	t.Run("exec error", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, errors.New("boom")
			},
		}
		q := New(db)
		if err := q.DeleteNotifyProviderCallbackReceipt(context.Background(), "proj_1", "provider_1", "cb_1"); err == nil {
			t.Fatal("DeleteNotifyProviderCallbackReceipt() error = nil, want non-nil")
		}
	})
}

func TestDeleteExpiredNotifyProviderCallbackReceipts(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("DELETE 2"), nil
			},
		}
		q := New(db)
		deleted, err := q.DeleteExpiredNotifyProviderCallbackReceipts(context.Background(), 100)
		if err != nil {
			t.Fatalf("DeleteExpiredNotifyProviderCallbackReceipts() error = %v", err)
		}
		if deleted != 2 {
			t.Fatalf("DeleteExpiredNotifyProviderCallbackReceipts() = %d, want 2", deleted)
		}
	})

	t.Run("exec error", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, errors.New("boom")
			},
		}
		q := New(db)
		if _, err := q.DeleteExpiredNotifyProviderCallbackReceipts(context.Background(), 100); err == nil {
			t.Fatal("DeleteExpiredNotifyProviderCallbackReceipts() error = nil, want non-nil")
		}
	})
}
