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

func webhookDeliveryScanFn(now time.Time, withOptionalFields bool) func(dest ...any) error {
	return func(dest ...any) error {
		*(dest[0].(*string)) = "delivery-1"
		*(dest[3].(*string)) = "https://example.com/webhook"
		*(dest[5].(*string)) = domain.WebhookStatusPending
		*(dest[6].(*int)) = 1
		*(dest[7].(*int)) = 3
		*(dest[12].(*time.Time)) = now
		*(dest[13].(*time.Time)) = now.Add(time.Second)
		*(dest[18].(*string)) = "project-1"
		if len(dest) > 19 {
			*(dest[19].(*string)) = "project-1"
		}
		if len(dest) > 20 {
			*(dest[20].(*string)) = "org-1"
		}

		if !withOptionalFields {
			return nil
		}

		runID := "run-1"
		jobID := "job-1"
		retryPolicy := domain.WebhookRetryPolicyExponential
		statusCode := 500
		lastError := "failed"
		nextRetry := now.Add(time.Minute)
		deliveredAt := now.Add(2 * time.Minute)
		eventTriggerID := "event-trigger-1"
		subscriptionID := "subscription-1"
		webhookSecret := "secret"
		claimToken := "claim-token"
		leaseExpiresAt := now.Add(3 * time.Minute)

		*(dest[1].(**string)) = &runID
		*(dest[2].(**string)) = &jobID
		*(dest[4].(**string)) = &retryPolicy
		*(dest[8].(**int)) = &statusCode
		*(dest[9].(**string)) = &lastError
		*(dest[10].(**time.Time)) = &nextRetry
		*(dest[11].(**time.Time)) = &deliveredAt
		*(dest[14].(**string)) = &eventTriggerID
		*(dest[15].(**string)) = &subscriptionID
		*(dest[16].(*[]byte)) = []byte(`{"payload":true}`)
		*(dest[17].(**string)) = &webhookSecret
		if len(dest) > 21 {
			*(dest[21].(**string)) = &claimToken
		}
		if len(dest) > 22 {
			*(dest[22].(**time.Time)) = &leaseExpiresAt
		}
		return nil
	}
}

type webhookDeliveryTx struct {
	*fakeTx
	commitErr error
	commits   int
	rollbacks int
}

func (tx *webhookDeliveryTx) Commit(context.Context) error {
	tx.commits++
	return tx.commitErr
}

func (tx *webhookDeliveryTx) Rollback(context.Context) error {
	tx.rollbacks++
	return nil
}

type webhookDeliveryBeginner struct {
	mockDBTX
	tx       *webhookDeliveryTx
	beginErr error
}

func (b *webhookDeliveryBeginner) Begin(context.Context) (pgx.Tx, error) {
	if b.beginErr != nil {
		return nil, b.beginErr
	}
	return b.tx, nil
}

func TestWebhookDeliveryCreateAndPayloadUnit(t *testing.T) {
	t.Parallel()

	t.Run("creates delivery with generated id and explicit payload", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
				require.Contains(t, sql, "INSERT INTO webhook_deliveries")
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = now
					*(dest[1].(*time.Time)) = now
					return nil
				}}
			},
		}
		d := &domain.WebhookDelivery{
			RunID:       "run-1",
			JobID:       "job-1",
			WebhookURL:  "https://example.com/webhook",
			Status:      domain.WebhookStatusPending,
			MaxAttempts: 3,
			Payload:     json.RawMessage(`{"payload":true}`),
			DedupeKey:   "dedupe-1",
		}

		require.NoError(t, New(db).CreateWebhookDelivery(context.Background(), d))
		require.NotEmpty(t, d.ID)
		require.Equal(t, "run-1", args[1])
		require.Equal(t, "job-1", args[2])
		payload, ok := args[15].(json.RawMessage)
		require.True(t, ok)
		require.JSONEq(t, `{"payload":true}`, string(payload))
		require.Equal(t, "dedupe-1", args[16])
	})

	t.Run("dedupe conflict is idempotent", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}
		err := New(db).CreateWebhookDelivery(context.Background(), &domain.WebhookDelivery{DedupeKey: "dedupe-1"})
		require.NoError(t, err)

		err = New(db).CreateWebhookDelivery(context.Background(), &domain.WebhookDelivery{})
		require.ErrorContains(t, err, "create webhook delivery")
	})

	t.Run("payload derives from JSON last error only", func(t *testing.T) {
		t.Parallel()

		require.Nil(t, webhookDeliveryPayload(nil))
		require.Nil(t, webhookDeliveryPayload(&domain.WebhookDelivery{}))
		require.Nil(t, webhookDeliveryPayload(&domain.WebhookDelivery{LastError: "plain text"}))
		require.JSONEq(t, `{"error":true}`, string(webhookDeliveryPayload(&domain.WebhookDelivery{LastError: `{"error":true}`})))
		require.JSONEq(t, `{"explicit":true}`, string(webhookDeliveryPayload(&domain.WebhookDelivery{
			Payload:   json.RawMessage(`{"explicit":true}`),
			LastError: `{"ignored":true}`,
		})))
	})
}

func TestWebhookDeliveryRunWebhookUnit(t *testing.T) {
	t.Parallel()

	t.Run("validates enqueue inputs", func(t *testing.T) {
		t.Parallel()

		q := New(&mockDBTX{})
		tests := []struct {
			name string
			job  *domain.Job
			run  *domain.JobRun
			want string
		}{
			{name: "nil job", run: &domain.JobRun{}, want: "nil job"},
			{name: "nil run", job: &domain.Job{}, want: "nil run"},
			{name: "missing run id", job: &domain.Job{WebhookURL: "https://example.com"}, run: &domain.JobRun{JobID: "job-1"}, want: "missing run id"},
			{name: "missing job id", job: &domain.Job{WebhookURL: "https://example.com"}, run: &domain.JobRun{ID: "run-1"}, want: "missing job id"},
			{name: "missing webhook", job: &domain.Job{}, run: &domain.JobRun{ID: "run-1", JobID: "job-1"}, want: "missing webhook url"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				_, err := q.EnqueueRunWebhook(context.Background(), tc.job, tc.run, 0)
				require.ErrorContains(t, err, tc.want)
			})
		}
	})

	t.Run("enqueues run webhook with default attempts and payload", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
				require.Contains(t, sql, "event_type")
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: func(dest ...any) error {
					nextRetry := now.Add(time.Minute)
					*(dest[0].(**time.Time)) = &nextRetry
					*(dest[1].(*time.Time)) = now
					*(dest[2].(*time.Time)) = now
					return nil
				}}
			},
		}

		d, err := New(db).EnqueueRunWebhook(
			context.Background(),
			&domain.Job{WebhookURL: "https://example.com/webhook", WebhookSecret: "secret"},
			&domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "project-1", Status: domain.StatusCompleted, Attempt: 2, Result: json.RawMessage(`{"ok":true}`)},
			0,
		)
		require.NoError(t, err)
		require.Equal(t, 3, d.MaxAttempts)
		require.Equal(t, domain.WebhookRetryPolicyExponential, d.RetryPolicy)
		require.Equal(t, "secret", args[8])
		require.Contains(t, string(args[9].([]byte)), `"run_id":"run-1"`)
		require.Equal(t, "run.completed", args[10])
		require.Equal(t, "project-1", args[11])

		enqueueErr := errors.New("enqueue failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return enqueueErr }}
		}
		_, err = New(db).EnqueueRunWebhook(context.Background(), &domain.Job{WebhookURL: "https://example.com"}, &domain.JobRun{ID: "run-1", JobID: "job-1"}, 1)
		require.ErrorContains(t, err, "enqueue run webhook")
		require.ErrorIs(t, err, enqueueErr)
	})
}

func TestWebhookDeliveryUpdateClaimAndRetryUnit(t *testing.T) {
	t.Parallel()

	t.Run("updates deliveries and claimed deliveries", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "UPDATE webhook_deliveries")
				require.NotEmpty(t, args)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = now
					return nil
				}}
			},
		}
		d := &domain.WebhookDelivery{ID: "delivery-1", ClaimToken: "claim-token", RetryPolicy: domain.WebhookRetryPolicyFixed, Status: domain.WebhookStatusDelivered}

		require.NoError(t, New(db).UpdateWebhookDelivery(context.Background(), d))
		updated, err := New(db).UpdateClaimedWebhookDelivery(context.Background(), d)
		require.NoError(t, err)
		require.True(t, updated)
		require.Empty(t, d.ClaimToken)
		require.Nil(t, d.LeaseExpiresAt)

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		updated, err = New(db).UpdateClaimedWebhookDelivery(context.Background(), &domain.WebhookDelivery{ID: "delivery-1", ClaimToken: "wrong"})
		require.NoError(t, err)
		require.False(t, updated)

		updateErr := errors.New("update failed")
		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return updateErr }}
		}
		err = New(db).UpdateWebhookDelivery(context.Background(), d)
		require.ErrorIs(t, err, updateErr)
		_, err = New(db).UpdateClaimedWebhookDelivery(context.Background(), d)
		require.ErrorContains(t, err, "update claimed webhook delivery")
		require.ErrorIs(t, err, updateErr)
	})

	t.Run("retries and replays scoped deliveries", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.True(t, strings.Contains(sql, "status IN ('failed', 'dead')") || strings.Contains(sql, "INSERT INTO webhook_deliveries"))
				require.NotEmpty(t, args)
				return &mockRow{scanFn: webhookDeliveryScanFn(now, true)}
			},
		}
		got, err := New(db).RetryWebhookDelivery(context.Background(), "delivery-1")
		require.NoError(t, err)
		require.Equal(t, "delivery-1", got.ID)

		got, err = New(db).ReplayWebhookDelivery(context.Background(), "project-1", "delivery-1")
		require.NoError(t, err)
		require.Equal(t, "delivery-1", got.ID)

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		_, err = New(db).RetryWebhookDelivery(context.Background(), "missing")
		require.ErrorContains(t, err, "webhook delivery not retriable")
		_, err = New(db).ReplayWebhookDelivery(context.Background(), "project-1", "missing")
		require.ErrorContains(t, err, "webhook delivery not found")
	})
}

func TestWebhookDeliveryClaimAndListUnit(t *testing.T) {
	t.Parallel()

	t.Run("claims pending retries with defaults", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		tx := &webhookDeliveryTx{}
		tx.fakeTx = &fakeTx{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "FOR UPDATE SKIP LOCKED")
				require.Equal(t, 100, args[0])
				require.NotEmpty(t, args[1])
				require.NotZero(t, args[2])
				return &mockRows{scanFns: []func(dest ...any) error{webhookDeliveryScanFn(now, true)}}, nil
			},
		}

		got, err := New(&webhookDeliveryBeginner{tx: tx}).ClaimPendingWebhookRetries(context.Background(), 0, 0)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "claim-token", got[0].ClaimToken)
		require.Equal(t, 1, tx.commits)
		require.Equal(t, 1, tx.rollbacks)
	})

	t.Run("claim pending retries maps transaction errors", func(t *testing.T) {
		t.Parallel()

		_, err := New(&mockDBTX{}).ClaimPendingWebhookRetries(context.Background(), 1, time.Second)
		require.ErrorContains(t, err, "db does not support transactions")

		beginErr := errors.New("begin failed")
		_, err = New(&webhookDeliveryBeginner{beginErr: beginErr}).ClaimPendingWebhookRetries(context.Background(), 1, time.Second)
		require.ErrorContains(t, err, "begin tx")
		require.ErrorIs(t, err, beginErr)

		tests := []struct {
			name       string
			queryErr   error
			scanErr    error
			rowErr     error
			commitErr  error
			wantString string
		}{
			{name: "query", queryErr: errors.New("query failed"), wantString: "claim pending webhook retries"},
			{name: "scan", scanErr: errors.New("scan failed"), wantString: "claim pending webhook retries scan"},
			{name: "rows", rowErr: errors.New("rows failed"), wantString: "claim pending webhook retries rows"},
			{name: "commit", commitErr: errors.New("commit failed"), wantString: "commit tx"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				tx := &webhookDeliveryTx{commitErr: tc.commitErr}
				tx.fakeTx = &fakeTx{
					queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
						if tc.queryErr != nil {
							return nil, tc.queryErr
						}
						rows := &mockRows{err: tc.rowErr}
						if tc.scanErr != nil {
							rows.scanFns = []func(dest ...any) error{func(...any) error { return tc.scanErr }}
						}
						return rows, nil
					},
				}

				_, err := New(&webhookDeliveryBeginner{tx: tx}).ClaimPendingWebhookRetries(context.Background(), 1, time.Second)
				require.ErrorContains(t, err, tc.wantString)
			})
		}
	})

	t.Run("lists deliveries with filters and pending queues", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		cursor := now.Add(time.Minute)
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				switch {
				case strings.Contains(sql, "LEFT JOIN jobs"):
					require.Contains(t, sql, "wd.status = $2")
					require.Contains(t, sql, "wd.created_at < $3")
					require.Equal(t, []any{"project-1", domain.WebhookStatusPending, cursor, 10}, args)
					return &mockRows{scanFns: []func(dest ...any) error{webhookDeliveryScanFn(now, true)}}, nil
				case strings.Contains(sql, "LEFT JOIN projects"):
					require.Empty(t, args)
					return &mockRows{scanFns: []func(dest ...any) error{webhookDeliveryScanFn(now, true)}}, nil
				default:
					require.Contains(t, sql, "run_id IS NOT NULL")
					require.Empty(t, args)
					return &mockRows{scanFns: []func(dest ...any) error{webhookDeliveryScanFn(now, false)}}, nil
				}
			},
		}

		list, err := New(db).ListWebhookDeliveries(context.Background(), "project-1", domain.WebhookStatusPending, 10, &cursor)
		require.NoError(t, err)
		require.Len(t, list, 1)

		pending, err := New(db).ListPendingWebhookRetries(context.Background())
		require.NoError(t, err)
		require.Equal(t, "org-1", pending[0].OrgID)

		runPending, err := New(db).ListPendingRunWebhookDeliveries(context.Background())
		require.NoError(t, err)
		require.Len(t, runPending, 1)
	})
}

func TestWebhookDeliveryGetDeleteScanAndCountersUnit(t *testing.T) {
	t.Parallel()

	t.Run("gets delivery and scans optional fields", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "WHERE id = $1")
				require.Equal(t, []any{"delivery-1", "project-1"}, args)
				return &mockRow{scanFn: webhookDeliveryScanFn(now, true)}
			},
		}
		got, err := New(db).GetWebhookDelivery(context.Background(), "project-1", "delivery-1")
		require.NoError(t, err)
		require.Equal(t, "run-1", got.RunID)
		require.Equal(t, "secret", got.WebhookSecret)
		require.JSONEq(t, `{"payload":true}`, string(got.Payload))

		db.queryRowFn = func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
		}
		_, err = New(db).GetWebhookDelivery(context.Background(), "project-1", "missing")
		require.ErrorContains(t, err, "webhook delivery not found")
	})

	t.Run("deletes old deliveries and resets stuck deliveries", func(t *testing.T) {
		t.Parallel()

		before := time.Now().UTC()
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				switch {
				case strings.Contains(sql, "DELETE FROM webhook_deliveries"):
					require.Equal(t, []any{before, 1000}, args)
					return pgconn.NewCommandTag("DELETE 4"), nil
				default:
					require.Contains(t, sql, "UPDATE webhook_deliveries SET next_retry_at")
					require.Empty(t, args)
					return pgconn.NewCommandTag("UPDATE 2"), nil
				}
			},
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int64)) = 6
					return nil
				}}
			},
		}
		deleted, err := New(db).DeleteOldWebhookDeliveries(context.Background(), before, 0)
		require.NoError(t, err)
		require.Equal(t, 4, deleted)

		reset, err := New(db).ResetStuckWebhookDeliveries(context.Background())
		require.NoError(t, err)
		require.EqualValues(t, 2, reset)

		count, err := New(db).CountPendingWebhookDeliveries(context.Background())
		require.NoError(t, err)
		require.EqualValues(t, 6, count)

		execErr := errors.New("exec failed")
		db.execFn = func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, execErr
		}
		_, err = New(db).DeleteOldWebhookDeliveries(context.Background(), before, 10)
		require.ErrorContains(t, err, "delete old webhook deliveries")
		_, err = New(db).ResetStuckWebhookDeliveries(context.Background())
		require.ErrorContains(t, err, "reset stuck webhook deliveries")
	})

	t.Run("scanner handles nil optionals and scan errors", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		got, err := scanWebhookDelivery(&mockRow{scanFn: webhookDeliveryScanFn(now, false)})
		require.NoError(t, err)
		require.Empty(t, got.RunID)
		require.Empty(t, got.Payload)

		got, err = scanWebhookDeliveryWithOrg(&mockRow{scanFn: webhookDeliveryScanFn(now, true)})
		require.NoError(t, err)
		require.Equal(t, "org-1", got.OrgID)

		got, err = scanWebhookDeliveryWithOrgAndClaim(&mockRow{scanFn: webhookDeliveryScanFn(now, true)})
		require.NoError(t, err)
		require.Equal(t, "claim-token", got.ClaimToken)
		require.NotNil(t, got.LeaseExpiresAt)

		scanErr := errors.New("scan failed")
		_, err = scanWebhookDelivery(&mockRow{scanFn: func(...any) error { return scanErr }})
		require.ErrorIs(t, err, scanErr)
		_, err = scanWebhookDeliveryWithOrg(&mockRow{scanFn: func(...any) error { return scanErr }})
		require.ErrorIs(t, err, scanErr)
		_, err = scanClaimedWebhookDeliveryWithOrg(&mockRow{scanFn: func(...any) error { return scanErr }})
		require.ErrorIs(t, err, scanErr)
	})
}
