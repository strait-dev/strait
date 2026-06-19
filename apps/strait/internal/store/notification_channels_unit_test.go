package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

const notificationUnitEncryptionKey = "0123456789abcdef0123456789abcdef"

func notificationChannelScanFn(t *testing.T, q *Queries, now time.Time, id, projectID string, enabled bool) func(dest ...any) error {
	t.Helper()

	return func(dest ...any) error {
		config := []byte(`{"url":"https://example.com/hook"}`)
		if q.secretEncryptionKey != "" {
			enc, err := q.secretEncryptor()
			require.NoError(t, err)
			config, err = enc.Encrypt(config)
			require.NoError(t, err)
		}

		*dest[0].(*string) = id
		*dest[1].(*string) = projectID
		*dest[2].(*string) = "slack"
		*dest[3].(*string) = "Ops"
		*dest[4].(*json.RawMessage) = json.RawMessage(config)
		*dest[5].(*bool) = enabled
		*dest[6].(*time.Time) = now
		*dest[7].(*time.Time) = now.Add(time.Minute)
		return nil
	}
}

func notificationDeliveryScanFn(now time.Time, includeClaimFields bool) func(dest ...any) error {
	return func(dest ...any) error {
		lastError := "temporary failure"
		nextRetry := now.Add(time.Minute)
		deliveredAt := now.Add(2 * time.Minute)

		*dest[0].(*string) = "delivery-1"
		*dest[1].(*string) = "channel-1"
		*dest[2].(*string) = "project-1"
		*dest[3].(*string) = domain.NotificationEventCostAnomaly
		*dest[4].(*json.RawMessage) = json.RawMessage(`{"run_id":"run-1"}`)
		*dest[5].(*string) = "processing"
		*dest[6].(*int) = 2
		*dest[7].(*int) = 5
		*dest[8].(**string) = &lastError
		*dest[9].(**time.Time) = &nextRetry
		*dest[10].(**time.Time) = &deliveredAt

		if includeClaimFields {
			leaseExpiry := now.Add(3 * time.Minute)
			*dest[11].(*string) = "claim-token"
			*dest[12].(**time.Time) = &leaseExpiry
			*dest[13].(*time.Time) = now
			*dest[14].(*time.Time) = now.Add(time.Second)
			return nil
		}

		*dest[11].(*time.Time) = now
		*dest[12].(*time.Time) = now.Add(time.Second)
		return nil
	}
}

type notificationDeliveryTx struct {
	*fakeTx
	commitErr error
	commits   int
	rollbacks int
}

func (tx *notificationDeliveryTx) Commit(context.Context) error {
	tx.commits++
	return tx.commitErr
}

func (tx *notificationDeliveryTx) Rollback(context.Context) error {
	tx.rollbacks++
	return nil
}

type notificationDeliveryBeginner struct {
	mockDBTX
	tx       *notificationDeliveryTx
	beginErr error
}

func (b *notificationDeliveryBeginner) Begin(context.Context) (pgx.Tx, error) {
	if b.beginErr != nil {
		return nil, b.beginErr
	}
	return b.tx, nil
}

func TestNotificationChannelCreateAndProjectLimitUnit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	db := &mockDBTX{}
	db.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
		switch {
		case strings.Contains(sql, "pg_try_advisory_xact_lock"):
			require.Equal(t, "notification_channel_limit:project-1", args[0])
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*bool) = true
				return nil
			}}
		case strings.Contains(sql, "COUNT(*)") && strings.Contains(sql, "notification_channels"):
			require.Equal(t, "project-1", args[0])
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*int) = 1
				return nil
			}}
		case strings.Contains(sql, "INSERT INTO notification_channels"):
			require.NotEmpty(t, args[0])
			require.Equal(t, "project-1", args[1])
			require.Equal(t, "slack", args[2])
			require.Equal(t, "Ops", args[3])
			require.NotEqual(t, json.RawMessage(`{"url":"https://example.com/hook"}`), args[4])
			require.True(t, args[5].(bool))
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*time.Time) = now
				*dest[1].(*time.Time) = now.Add(time.Minute)
				return nil
			}}
		default:
			require.Failf(t, "unexpected query", "%s", sql)
			return &mockRow{}
		}
	}

	q := New(db)
	q.SetSecretEncryptionKey(notificationUnitEncryptionKey)
	ch := &domain.NotificationChannel{
		ProjectID:   "project-1",
		ChannelType: "slack",
		Name:        "Ops",
		Config:      json.RawMessage(`{"url":"https://example.com/hook"}`),
		Enabled:     true,
	}
	require.NoError(t, q.CreateNotificationChannelWithProjectLimit(context.Background(), ch, 2))
	require.NotEmpty(t, ch.ID)
	require.Equal(t, now, ch.CreatedAt)
	require.Equal(t, now.Add(time.Minute), ch.UpdatedAt)

	limitDB := &mockDBTX{queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
		return &mockRow{scanFn: func(dest ...any) error {
			switch {
			case strings.Contains(sql, "pg_try_advisory_xact_lock"):
				*dest[0].(*bool) = true
			case strings.Contains(sql, "COUNT(*)"):
				*dest[0].(*int) = 3
			default:
				require.Failf(t, "unexpected query", "%s", sql)
			}
			return nil
		}}
	}}
	err := New(limitDB).CreateNotificationChannelWithProjectLimit(
		context.Background(),
		&domain.NotificationChannel{ProjectID: "project-1", Name: "extra"},
		3,
	)
	require.ErrorIs(t, err, ErrNotificationChannelLimitExceeded)
}

func TestNotificationChannelReadListUpdateDeleteUnit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	q := New(&mockDBTX{})
	q.SetSecretEncryptionKey(notificationUnitEncryptionKey)
	db := &mockDBTX{}
	db.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
		switch {
		case strings.Contains(sql, "SELECT id, project_id, channel_type"):
			require.Equal(t, []any{"channel-1", "project-1"}, args)
			return &mockRow{scanFn: notificationChannelScanFn(t, q, now, "channel-1", "project-1", true)}
		case strings.Contains(sql, "UPDATE notification_channels"):
			require.Equal(t, []any{"channel-1", "Renamed", "email"}, args[:3])
			require.NotEmpty(t, args[3])
			require.False(t, args[4].(bool))
			require.Equal(t, "project-1", args[5])
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*time.Time) = now.Add(2 * time.Minute)
				return nil
			}}
		default:
			require.Failf(t, "unexpected query", "%s", sql)
			return &mockRow{}
		}
	}
	db.queryFn = func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
		switch {
		case strings.Contains(sql, "WHERE project_id = $1 AND enabled = true") && strings.Contains(sql, "ORDER BY created_at DESC"):
			require.Equal(t, []any{"project-1"}, args)
			return &mockRows{scanFns: []func(dest ...any) error{
				notificationChannelScanFn(t, q, now, "channel-1", "project-1", true),
			}}, nil
		case strings.Contains(sql, "WHERE project_id = $1") && strings.Contains(sql, "ORDER BY created_at DESC"):
			require.Equal(t, []any{"project-1"}, args)
			return &mockRows{scanFns: []func(dest ...any) error{
				notificationChannelScanFn(t, q, now, "channel-1", "project-1", true),
				notificationChannelScanFn(t, q, now.Add(time.Minute), "channel-2", "project-1", false),
			}}, nil
		case strings.Contains(sql, "project_id = ANY($1)"):
			require.Equal(t, []any{[]string{"project-1", "project-2"}}, args)
			return &mockRows{scanFns: []func(dest ...any) error{
				notificationChannelScanFn(t, q, now, "channel-1", "project-1", true),
				notificationChannelScanFn(t, q, now, "channel-3", "project-2", true),
			}}, nil
		default:
			require.Failf(t, "unexpected query", "%s", sql)
			return nil, nil
		}
	}
	db.execFn = func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
		require.Contains(t, sql, "DELETE FROM notification_channels")
		require.Equal(t, []any{"channel-1", "project-1"}, args)
		return pgconn.NewCommandTag("DELETE 1"), nil
	}
	q.db = db

	got, err := q.GetNotificationChannel(context.Background(), "channel-1", "project-1")
	require.NoError(t, err)
	require.JSONEq(t, `{"url":"https://example.com/hook"}`, string(got.Config))

	all, err := q.ListNotificationChannels(context.Background(), "project-1")
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.False(t, all[1].Enabled)

	enabled, err := q.ListEnabledNotificationChannels(context.Background(), "project-1")
	require.NoError(t, err)
	require.Len(t, enabled, 1)

	grouped, err := q.ListEnabledNotificationChannelsByProjectIDs(context.Background(), []string{"project-1", "project-2"})
	require.NoError(t, err)
	require.Len(t, grouped["project-1"], 1)
	require.Len(t, grouped["project-2"], 1)

	empty, err := q.ListEnabledNotificationChannelsByProjectIDs(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, empty)

	got.Name = "Renamed"
	got.ChannelType = "email"
	got.Enabled = false
	require.NoError(t, q.UpdateNotificationChannel(context.Background(), got))
	require.Equal(t, now.Add(2*time.Minute), got.UpdatedAt)

	require.NoError(t, q.DeleteNotificationChannel(context.Background(), "channel-1", "project-1"))
}

func TestNotificationChannelErrorPathsUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "get not found maps sentinel",
			run: func(t *testing.T) {
				t.Helper()
				db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
					return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
				}}
				_, err := New(db).GetNotificationChannel(context.Background(), "missing", "project-1")
				require.ErrorIs(t, err, ErrNotificationChannelNotFound)
			},
		},
		{
			name: "list query error wraps",
			run: func(t *testing.T) {
				t.Helper()
				queryErr := errors.New("query failed")
				db := &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return nil, queryErr
				}}
				_, err := New(db).ListNotificationChannels(context.Background(), "project-1")
				require.ErrorIs(t, err, queryErr)
				require.Contains(t, err.Error(), "list notification channels")
			},
		},
		{
			name: "list scan error wraps",
			run: func(t *testing.T) {
				t.Helper()
				scanErr := errors.New("scan failed")
				db := &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return &mockRows{scanFns: []func(dest ...any) error{func(...any) error { return scanErr }}}, nil
				}}
				_, err := New(db).ListEnabledNotificationChannels(context.Background(), "project-1")
				require.ErrorIs(t, err, scanErr)
				require.Contains(t, err.Error(), "list enabled notification channels scan")
			},
		},
		{
			name: "list rows error wraps",
			run: func(t *testing.T) {
				t.Helper()
				rowsErr := errors.New("rows failed")
				db := &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return &mockRows{err: rowsErr}, nil
				}}
				_, err := New(db).ListEnabledNotificationChannelsByProjectIDs(context.Background(), []string{"project-1"})
				require.ErrorIs(t, err, rowsErr)
				require.Contains(t, err.Error(), "list enabled channels by project IDs rows")
			},
		},
		{
			name: "update not found maps sentinel",
			run: func(t *testing.T) {
				t.Helper()
				db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
					return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
				}}
				err := New(db).UpdateNotificationChannel(context.Background(), &domain.NotificationChannel{ID: "missing", ProjectID: "project-1"})
				require.ErrorIs(t, err, ErrNotificationChannelNotFound)
			},
		},
		{
			name: "delete not found maps sentinel",
			run: func(t *testing.T) {
				t.Helper()
				db := &mockDBTX{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("DELETE 0"), nil
				}}
				err := New(db).DeleteNotificationChannel(context.Background(), "missing", "project-1")
				require.ErrorIs(t, err, ErrNotificationChannelNotFound)
			},
		},
		{
			name: "delete exec error wraps",
			run: func(t *testing.T) {
				t.Helper()
				execErr := errors.New("delete failed")
				db := &mockDBTX{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, execErr
				}}
				err := New(db).DeleteNotificationChannel(context.Background(), "channel-1", "project-1")
				require.ErrorIs(t, err, execErr)
				require.Contains(t, err.Error(), "delete notification channel")
			},
		},
		{
			name: "invalid encryption key fails closed",
			run: func(t *testing.T) {
				t.Helper()
				q := New(&mockDBTX{})
				q.SetSecretEncryptionKey("too-short")
				err := q.CreateNotificationChannel(context.Background(), &domain.NotificationChannel{Config: json.RawMessage(`{"x":true}`)})
				require.Error(t, err)
				require.Contains(t, err.Error(), "create notification channel config encryptor")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

func TestNotificationDeliveryCreateClaimAndListUnit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	db := &mockDBTX{}
	db.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
		require.Contains(t, sql, "INSERT INTO notification_deliveries")
		require.NotEmpty(t, args[0])
		require.Equal(t, "channel-1", args[1])
		require.Equal(t, "project-1", args[2])
		require.Equal(t, domain.NotificationEventCostAnomaly, args[3])
		require.JSONEq(t, `{"run_id":"run-1"}`, string(args[4].(json.RawMessage)))
		require.Equal(t, "pending", args[5])
		require.Equal(t, 3, args[6])
		require.Nil(t, args[7])
		require.Equal(t, "dedupe-1", args[8])
		return &mockRow{scanFn: func(dest ...any) error {
			*dest[0].(*time.Time) = now
			*dest[1].(*time.Time) = now.Add(time.Second)
			return nil
		}}
	}
	q := New(db)
	delivery := &domain.NotificationDelivery{
		ChannelID:   "channel-1",
		ProjectID:   "project-1",
		EventType:   domain.NotificationEventCostAnomaly,
		Payload:     json.RawMessage(`{"run_id":"run-1"}`),
		Status:      "pending",
		MaxAttempts: 3,
		DedupeKey:   "dedupe-1",
	}
	require.NoError(t, q.CreateNotificationDelivery(context.Background(), delivery))
	require.NotEmpty(t, delivery.ID)
	require.Equal(t, now, delivery.CreatedAt)

	db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
		return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
	}
	require.NoError(t, q.CreateNotificationDelivery(context.Background(), &domain.NotificationDelivery{DedupeKey: "dedupe-1"}))

	tx := &notificationDeliveryTx{fakeTx: &fakeTx{}}
	tx.queryFn = func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
		require.Contains(t, sql, "FOR UPDATE SKIP LOCKED")
		require.Equal(t, 10, args[0])
		require.NotEmpty(t, args[1])
		require.IsType(t, time.Time{}, args[2])
		return &mockRows{scanFns: []func(dest ...any) error{notificationDeliveryScanFn(now, true)}}, nil
	}
	beginner := &notificationDeliveryBeginner{tx: tx}
	claimed, err := New(beginner).ClaimPendingNotificationDeliveries(context.Background(), 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, "claim-token", claimed[0].ClaimToken)
	require.NotNil(t, claimed[0].LeaseExpiry)
	require.Equal(t, 1, tx.commits)
	require.Equal(t, 1, tx.rollbacks)

	db.queryFn = func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
		require.Contains(t, sql, "created_at < $2")
		require.Equal(t, []any{"project-1", now, 20}, args)
		return &mockRows{scanFns: []func(dest ...any) error{notificationDeliveryScanFn(now, false)}}, nil
	}
	listed, err := q.ListNotificationDeliveries(context.Background(), "project-1", 20, &now)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "temporary failure", listed[0].LastError)
}

func TestNotificationDeliveryUpdateAndErrorPathsUnit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	db := &mockDBTX{execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
		require.Contains(t, sql, "UPDATE notification_deliveries")
		require.Equal(t, "delivery-1", args[0])
		require.Equal(t, "claim-token", args[1])
		require.Equal(t, "delivered", args[2])
		require.Equal(t, 1, args[3])
		require.Empty(t, args[4])
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}}
	updated, err := New(db).UpdateClaimedNotificationDelivery(context.Background(), &domain.NotificationDelivery{
		ID:          "delivery-1",
		ClaimToken:  "claim-token",
		Status:      "delivered",
		Attempts:    1,
		DeliveredAt: &now,
	})
	require.NoError(t, err)
	require.True(t, updated)

	db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag("UPDATE 0"), nil
	}
	updated, err = New(db).UpdateClaimedNotificationDelivery(context.Background(), &domain.NotificationDelivery{})
	require.NoError(t, err)
	require.False(t, updated)

	errDB := &mockDBTX{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, errors.New("exec failed")
	}}
	_, err = New(errDB).UpdateClaimedNotificationDelivery(context.Background(), &domain.NotificationDelivery{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "update claimed notification delivery")

	_, err = New(&mockDBTX{}).ClaimPendingNotificationDeliveries(context.Background(), 1, time.Minute)
	require.Error(t, err)
	require.Contains(t, err.Error(), "db does not support transactions")

	beginErr := errors.New("begin failed")
	_, err = New(&notificationDeliveryBeginner{beginErr: beginErr}).ClaimPendingNotificationDeliveries(context.Background(), 1, time.Minute)
	require.ErrorIs(t, err, beginErr)

	queryErr := errors.New("query failed")
	tx := &notificationDeliveryTx{fakeTx: &fakeTx{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
		return nil, queryErr
	}}}
	_, err = New(&notificationDeliveryBeginner{tx: tx}).ClaimPendingNotificationDeliveries(context.Background(), 1, time.Minute)
	require.ErrorIs(t, err, queryErr)
}

func TestNotificationConfigDecryptFallbackUnit(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"url":"plain"}`)
	q := New(&mockDBTX{})
	got := q.decryptNotificationConfig("channel-1", raw)
	require.Equal(t, raw, got)
	got[0] = '{'
	require.Equal(t, byte('{'), raw[0])

	q.SetSecretEncryptionKey("invalid")
	got = q.decryptNotificationConfig("channel-1", raw)
	require.Equal(t, raw, got)

	q.SetSecretEncryptionKey(notificationUnitEncryptionKey)
	got = q.decryptNotificationConfig("channel-1", []byte("not ciphertext"))
	require.Equal(t, []byte("not ciphertext"), got)
}
