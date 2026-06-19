package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func webhookSubscriptionScanFn(now time.Time) func(dest ...any) error {
	return func(dest ...any) error {
		*dest[0].(*string) = "sub-1"
		*dest[1].(*string) = "project-1"
		*dest[2].(*string) = "https://example.com/webhook"
		*dest[3].(*[]string) = []string{domain.WebhookEventRunCompleted, domain.WebhookEventRunFailed}
		*dest[4].(*string) = "secret-current"
		*dest[5].(*bool) = true
		*dest[6].(*time.Time) = now
		return nil
	}
}

func TestWebhookSubscriptionCreateAndLimitsUnit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	var insertCalls int
	db := &mockDBTX{}
	db.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
		switch {
		case strings.Contains(sql, "pg_try_advisory_xact_lock"):
			require.Contains(t, args[0].(string), "webhook_")
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*bool) = true
				return nil
			}}
		case strings.Contains(sql, "current_setting"):
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*string) = "project-ctx"
				return nil
			}}
		case strings.Contains(sql, "COUNT(*)") && strings.Contains(sql, "projects WHERE org_id"):
			require.Equal(t, "org-1", args[0])
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*int) = 1
				return nil
			}}
		case strings.Contains(sql, "COUNT(*)") && strings.Contains(sql, "WHERE project_id = $1 AND active = true"):
			require.Equal(t, "project-1", args[0])
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*int) = 1
				return nil
			}}
		case strings.Contains(sql, "INSERT INTO webhook_subscriptions"):
			insertCalls++
			require.NotEmpty(t, args[0])
			require.Equal(t, "project-1", args[1])
			require.Equal(t, "https://example.com/webhook", args[2])
			require.Equal(t, []string{domain.WebhookEventRunCompleted}, args[3])
			require.Equal(t, "secret-current", args[4])
			require.True(t, args[5].(bool))
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*time.Time) = now
				return nil
			}}
		default:
			require.Failf(t, "unexpected query", "%s", sql)
			return &mockRow{}
		}
	}
	var execs []string
	db.execFn = func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
		execs = append(execs, sql)
		switch {
		case strings.Contains(sql, "set_config('app.current_project_id', '', true)"):
			require.Empty(t, args)
		case strings.Contains(sql, "set_config('app.current_project_id', $1, true)"):
			require.Equal(t, []any{"project-ctx"}, args)
		default:
			require.Failf(t, "unexpected exec", "%s", sql)
		}
		return pgconn.NewCommandTag("SELECT 1"), nil
	}

	q := New(db)
	sub := &domain.WebhookSubscription{
		ProjectID:  "project-1",
		WebhookURL: "https://example.com/webhook",
		EventTypes: []string{domain.WebhookEventRunCompleted},
		Secret:     "secret-current",
		Active:     true,
	}
	require.NoError(t, q.CreateWebhookSubscriptionWithLimits(context.Background(), sub, "org-1", 2, 2))
	require.NotEmpty(t, sub.ID)
	require.Equal(t, now, sub.CreatedAt)
	require.Equal(t, 1, insertCalls)
	require.Len(t, execs, 2)
	require.Contains(t, execs[0], "set_config('app.current_project_id', '', true)")
	require.Contains(t, execs[1], "set_config('app.current_project_id', $1, true)")

	limitDB := &mockDBTX{queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
		return &mockRow{scanFn: func(dest ...any) error {
			switch {
			case strings.Contains(sql, "pg_try_advisory_xact_lock"):
				*dest[0].(*bool) = true
			case strings.Contains(sql, "current_setting"):
				*dest[0].(*string) = ""
			case strings.Contains(sql, "COUNT(*)") && strings.Contains(sql, "projects WHERE org_id"):
				*dest[0].(*int) = 3
			default:
				require.Failf(t, "unexpected query", "%s", sql)
			}
			return nil
		}}
	}}
	err := New(limitDB).CreateWebhookSubscriptionWithLimits(
		context.Background(),
		&domain.WebhookSubscription{ProjectID: "project-1"},
		"org-1",
		3,
		-1,
	)
	require.ErrorIs(t, err, ErrWebhookEndpointLimitExceeded)

	err = New(limitDB).CreateWebhookSubscriptionWithOrgLimit(
		context.Background(),
		&domain.WebhookSubscription{ProjectID: "project-1"},
		"org-1",
		3,
	)
	require.ErrorIs(t, err, ErrWebhookEndpointLimitExceeded)

	projectLimitDB := &mockDBTX{queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
		return &mockRow{scanFn: func(dest ...any) error {
			switch {
			case strings.Contains(sql, "pg_try_advisory_xact_lock"):
				*dest[0].(*bool) = true
			case strings.Contains(sql, "COUNT(*)") && strings.Contains(sql, "WHERE project_id = $1 AND active = true"):
				*dest[0].(*int) = 2
			default:
				require.Failf(t, "unexpected query", "%s", sql)
			}
			return nil
		}}
	}}
	err = New(projectLimitDB).CreateWebhookSubscriptionWithLimits(
		context.Background(),
		&domain.WebhookSubscription{ProjectID: "project-1"},
		"",
		-1,
		2,
	)
	require.ErrorIs(t, err, ErrWebhookProjectLimitExceeded)
}

func TestWebhookSubscriptionDuplicateAndLockErrorsUnit(t *testing.T) {
	t.Parallel()

	duplicate := &pgconn.PgError{Code: "23505"}
	db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
		return &mockRow{scanFn: func(...any) error { return duplicate }}
	}}
	err := New(db).CreateWebhookSubscription(context.Background(), &domain.WebhookSubscription{})
	require.ErrorIs(t, err, ErrWebhookSubscriptionDuplicate)

	lockErr := errors.New("lock failed")
	lockDB := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
		return &mockRow{scanFn: func(...any) error { return lockErr }}
	}}
	err = New(lockDB).CreateWebhookSubscriptionWithLimits(
		context.Background(),
		&domain.WebhookSubscription{ProjectID: "project-1"},
		"org-1",
		1,
		-1,
	)
	require.ErrorIs(t, err, lockErr)
	require.Contains(t, err.Error(), "lock webhook endpoint limit")
}

func TestWebhookSubscriptionOrgCountRestoreErrorJoinsOriginalUnit(t *testing.T) {
	t.Parallel()

	countErr := errors.New("count failed")
	restoreErr := errors.New("restore failed")
	var queryRows int
	db := &mockDBTX{}
	db.queryRowFn = func(_ context.Context, sql string, _ ...any) pgx.Row {
		queryRows++
		switch {
		case strings.Contains(sql, "current_setting"):
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*string) = "project-ctx"
				return nil
			}}
		case strings.Contains(sql, "COUNT(*)"):
			return &mockRow{scanFn: func(...any) error { return countErr }}
		default:
			require.Failf(t, "unexpected query", "%s", sql)
			return &mockRow{}
		}
	}
	db.execFn = func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
		switch {
		case strings.Contains(sql, "set_config('app.current_project_id', '', true)"):
			require.Empty(t, args)
			return pgconn.NewCommandTag("SELECT 1"), nil
		case strings.Contains(sql, "set_config('app.current_project_id', $1, true)"):
			require.Equal(t, []any{"project-ctx"}, args)
			return pgconn.CommandTag{}, restoreErr
		default:
			require.Failf(t, "unexpected exec", "%s", sql)
			return pgconn.CommandTag{}, nil
		}
	}

	_, err := New(db).countWebhookSubscriptionsByOrgIgnoringProjectRLS(context.Background(), "org-1")
	require.ErrorIs(t, err, countErr)
	require.ErrorIs(t, err, restoreErr)
	require.Equal(t, 2, queryRows)
}

func TestWebhookSubscriptionListGetDeleteAndSecretsUnit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	grace := now.Add(time.Hour)
	db := &mockDBTX{}
	db.queryFn = func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
		require.Contains(t, sql, "WHERE project_id = $1 AND active = TRUE")
		require.Equal(t, []any{"project-1"}, args)
		return &mockRows{scanFns: []func(dest ...any) error{
			webhookSubscriptionScanFn(now),
			webhookSubscriptionScanFn(now.Add(time.Minute)),
		}}, nil
	}
	db.queryRowFn = func(_ context.Context, sql string, args ...any) pgx.Row {
		switch {
		case strings.Contains(sql, "SELECT id, project_id"):
			require.Equal(t, []any{"sub-1"}, args)
			return &mockRow{scanFn: webhookSubscriptionScanFn(now)}
		case strings.Contains(sql, "SELECT secret, previous_secret"):
			require.Equal(t, []any{"sub-1"}, args)
			return &mockRow{scanFn: func(dest ...any) error {
				prev := "secret-previous"
				*dest[0].(*string) = "secret-current"
				*dest[1].(**string) = &prev
				*dest[2].(**time.Time) = &grace
				return nil
			}}
		default:
			require.Failf(t, "unexpected query", "%s", sql)
			return &mockRow{}
		}
	}
	var deleteExecs []string
	db.execFn = func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
		deleteExecs = append(deleteExecs, sql)
		switch {
		case strings.Contains(sql, "UPDATE webhook_deliveries SET subscription_id = NULL"):
			require.Equal(t, []any{"sub-1"}, args)
			return pgconn.NewCommandTag("UPDATE 2"), nil
		case strings.Contains(sql, "DELETE FROM webhook_subscriptions"):
			require.Equal(t, []any{"sub-1"}, args)
			return pgconn.NewCommandTag("DELETE 1"), nil
		case strings.Contains(sql, "UPDATE webhook_subscriptions") && strings.Contains(sql, "previous_secret = secret"):
			require.Equal(t, "sub-1", args[0])
			require.Equal(t, "secret-new", args[1])
			require.Equal(t, grace, args[2])
			return pgconn.NewCommandTag("UPDATE 1"), nil
		default:
			require.Failf(t, "unexpected exec", "%s", sql)
			return pgconn.CommandTag{}, nil
		}
	}

	q := New(db)
	subs, err := q.ListWebhookSubscriptions(context.Background(), "project-1")
	require.NoError(t, err)
	require.Len(t, subs, 2)
	require.Equal(t, []string{domain.WebhookEventRunCompleted, domain.WebhookEventRunFailed}, subs[0].EventTypes)

	got, err := q.GetWebhookSubscription(context.Background(), "sub-1")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/webhook", got.WebhookURL)

	current, previous, gotGrace, err := q.GetWebhookSubscriptionSecrets(context.Background(), "sub-1")
	require.NoError(t, err)
	require.Equal(t, "secret-current", current)
	require.Equal(t, "secret-previous", previous)
	require.Equal(t, &grace, gotGrace)

	require.NoError(t, q.RotateWebhookSecret(context.Background(), "sub-1", "secret-new", grace))
	require.NoError(t, q.DeleteWebhookSubscription(context.Background(), "sub-1"))
	require.Len(t, deleteExecs, 3)
}

func TestWebhookSubscriptionErrorPathsUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "list query error wraps",
			run: func(t *testing.T) {
				t.Helper()
				queryErr := errors.New("query failed")
				db := &mockDBTX{queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return nil, queryErr
				}}
				_, err := New(db).ListWebhookSubscriptions(context.Background(), "project-1")
				require.ErrorIs(t, err, queryErr)
				require.Contains(t, err.Error(), "list webhook subscriptions")
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
				_, err := New(db).ListWebhookSubscriptions(context.Background(), "project-1")
				require.ErrorIs(t, err, scanErr)
				require.Contains(t, err.Error(), "list webhook subscriptions scan")
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
				_, err := New(db).ListWebhookSubscriptions(context.Background(), "project-1")
				require.ErrorIs(t, err, rowsErr)
				require.Contains(t, err.Error(), "list webhook subscriptions rows")
			},
		},
		{
			name: "get not found maps sentinel",
			run: func(t *testing.T) {
				t.Helper()
				db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
					return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
				}}
				_, err := New(db).GetWebhookSubscription(context.Background(), "missing")
				require.ErrorIs(t, err, ErrWebhookSubscriptionNotFound)
			},
		},
		{
			name: "delete detach error wraps",
			run: func(t *testing.T) {
				t.Helper()
				execErr := errors.New("detach failed")
				db := &mockDBTX{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, execErr
				}}
				err := New(db).DeleteWebhookSubscription(context.Background(), "sub-1")
				require.ErrorIs(t, err, execErr)
				require.Contains(t, err.Error(), "detach webhook deliveries")
			},
		},
		{
			name: "delete not found maps sentinel",
			run: func(t *testing.T) {
				t.Helper()
				var calls int
				db := &mockDBTX{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
					calls++
					if calls == 1 {
						return pgconn.NewCommandTag("UPDATE 0"), nil
					}
					return pgconn.NewCommandTag("DELETE 0"), nil
				}}
				err := New(db).DeleteWebhookSubscription(context.Background(), "missing")
				require.ErrorIs(t, err, ErrWebhookSubscriptionNotFound)
			},
		},
		{
			name: "rotate not found maps sentinel",
			run: func(t *testing.T) {
				t.Helper()
				db := &mockDBTX{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("UPDATE 0"), nil
				}}
				err := New(db).RotateWebhookSecret(context.Background(), "missing", "new", time.Now())
				require.ErrorIs(t, err, ErrWebhookSubscriptionNotFound)
			},
		},
		{
			name: "secrets not found returns empty",
			run: func(t *testing.T) {
				t.Helper()
				db := &mockDBTX{queryRowFn: func(context.Context, string, ...any) pgx.Row {
					return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
				}}
				current, previous, grace, err := New(db).GetWebhookSubscriptionSecrets(context.Background(), "missing")
				require.NoError(t, err)
				require.Empty(t, current)
				require.Empty(t, previous)
				require.Nil(t, grace)
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
