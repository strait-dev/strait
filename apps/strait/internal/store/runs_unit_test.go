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

func fillRunScanDest(dest []any, now time.Time, includeOptional bool) {
	*(dest[0].(*string)) = "run-1"
	*(dest[1].(*string)) = "job-1"
	*(dest[2].(*string)) = "project-1"
	*(dest[3].(*domain.RunStatus)) = domain.StatusCompleted
	*(dest[4].(*int)) = 2
	*(dest[5].(*[]byte)) = []byte(`{"input":true}`)
	*(dest[6].(*[]byte)) = []byte(`{"ok":true}`)
	*(dest[7].(*[]byte)) = []byte(`{"tenant":"acme"}`)
	*(dest[10].(*string)) = domain.TriggerManual
	*(dest[18].(*int)) = 9
	*(dest[20].(*int)) = 3
	*(dest[21].(*time.Time)) = now
	*(dest[24].(*bool)) = true
	*(dest[26].(*int)) = 4
	*(dest[33].(*bool)) = true
	if len(dest) > 35 {
		*(dest[35].(*int64)) = 42
	}

	if !includeOptional {
		return
	}

	runErr := "failed"
	errorClass := "network"
	parentRunID := "parent-1"
	idempotencyKey := "idem-1"
	workflowStepRunID := "step-run-1"
	continuationOf := "previous-run"
	jobVersionID := "version-1"
	createdBy := "user-1"
	batchID := "batch-1"
	concurrencyKey := "tenant-1"
	executionMode := string(domain.ExecutionModeWorker)
	replayedRunID := "replay-1"
	scheduledAt := now.Add(time.Minute)
	startedAt := now.Add(2 * time.Minute)
	finishedAt := now.Add(3 * time.Minute)
	heartbeatAt := now.Add(4 * time.Minute)
	nextRetryAt := now.Add(5 * time.Minute)
	expiresAt := now.Add(6 * time.Minute)

	*(dest[8].(**string)) = &runErr
	*(dest[9].(**string)) = &errorClass
	*(dest[11].(**time.Time)) = &scheduledAt
	*(dest[12].(**time.Time)) = &startedAt
	*(dest[13].(**time.Time)) = &finishedAt
	*(dest[14].(**time.Time)) = &heartbeatAt
	*(dest[15].(**time.Time)) = &nextRetryAt
	*(dest[16].(**time.Time)) = &expiresAt
	*(dest[17].(**string)) = &parentRunID
	*(dest[19].(**string)) = &idempotencyKey
	*(dest[22].(**string)) = &workflowStepRunID
	*(dest[23].(*[]byte)) = []byte(`{"total_ms":42}`)
	*(dest[25].(**string)) = &continuationOf
	*(dest[27].(*[]byte)) = []byte(`{"region":"iad"}`)
	*(dest[28].(**string)) = &jobVersionID
	*(dest[29].(**string)) = &createdBy
	*(dest[30].(**string)) = &batchID
	*(dest[31].(**string)) = &concurrencyKey
	*(dest[32].(**string)) = &executionMode
	*(dest[34].(**string)) = &replayedRunID
}

func runScanFn(now time.Time, includeOptional bool) func(dest ...any) error {
	return func(dest ...any) error {
		fillRunScanDest(dest, now, includeOptional)
		return nil
	}
}

func TestCreateRunDefaultsAndErrorsUnit(t *testing.T) {
	t.Parallel()

	t.Run("applies generated defaults and nullable insert arguments", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		scheduledAt := createdAt.Add(time.Hour)
		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
				require.Contains(t, sql, "INSERT INTO job_runs")
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = createdAt
					return nil
				}}
			},
		}
		run := &domain.JobRun{
			JobID:       "job-1",
			ProjectID:   "project-1",
			ScheduledAt: &scheduledAt,
			Payload:     json.RawMessage(`{"input":true}`),
			Metadata:    map[string]string{"tenant": "acme"},
			Tags:        map[string]string{"region": "iad"},
		}

		require.NoError(t, New(db).CreateRun(context.Background(), run))
		require.NotEmpty(t, run.ID)
		require.Equal(t, createdAt, run.CreatedAt)
		require.Equal(t, domain.StatusDelayed, run.Status)
		require.Equal(t, 1, run.Attempt)
		require.Equal(t, domain.TriggerManual, run.TriggeredBy)
		require.Len(t, args, 36)
		require.Equal(t, run.ID, args[0])
		require.Equal(t, domain.StatusDelayed, args[3])
		require.Equal(t, 1, args[4])
		require.JSONEq(t, `{"input":true}`, string(args[5].(json.RawMessage)))
		require.Nil(t, args[6])
		require.Nil(t, args[7])
		require.Equal(t, domain.TriggerManual, args[8])
		require.Nil(t, args[15])
		require.Nil(t, args[17])
		require.JSONEq(t, `{"region":"iad"}`, string(args[23].([]byte)))
		require.Nil(t, args[24])
		require.Nil(t, args[25])
		require.Nil(t, args[26])
		require.Nil(t, args[27])
		require.Equal(t, string(domain.ExecutionModeHTTP), args[28])
		require.Equal(t, defaultJobQueueName, args[29])
		require.JSONEq(t, `{"tenant":"acme"}`, string(args[34].([]byte)))
		require.Equal(t, false, args[35])
	})

	t.Run("preserves explicit execution mode queue and active status", func(t *testing.T) {
		t.Parallel()

		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, gotArgs ...any) pgx.Row {
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*time.Time)) = time.Now().UTC()
					return nil
				}}
			},
		}
		run := &domain.JobRun{
			ID:            "run-1",
			JobID:         "job-1",
			ProjectID:     "project-1",
			Status:        domain.StatusExecuting,
			Attempt:       3,
			TriggeredBy:   "workflow",
			ExecutionMode: domain.ExecutionModeWorker,
			QueueName:     "critical",
		}

		require.NoError(t, New(db).CreateRun(context.Background(), run))
		require.Equal(t, domain.StatusExecuting, args[3])
		require.Equal(t, 3, args[4])
		require.Equal(t, "workflow", args[8])
		require.Equal(t, string(domain.ExecutionModeWorker), args[28])
		require.Equal(t, "critical", args[29])
	})

	t.Run("maps idempotent empty insert to conflict", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}
		err := New(db).CreateRun(context.Background(), &domain.JobRun{
			JobID:          "job-1",
			ProjectID:      "project-1",
			IdempotencyKey: "idem-1",
		})
		require.ErrorIs(t, err, domain.ErrIdempotencyConflict)
	})

	t.Run("wraps non-idempotent insert errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("write failed") }}
			},
		}
		err := New(db).CreateRun(context.Background(), &domain.JobRun{
			JobID:     "job-1",
			ProjectID: "project-1",
		})
		require.ErrorContains(t, err, "create run: write failed")
		require.NotErrorIs(t, err, domain.ErrIdempotencyConflict)
	})
}

func TestRunStatusAndTokenStateFallbacksUnit(t *testing.T) {
	t.Parallel()

	t.Run("status returns hot row without history query", func(t *testing.T) {
		t.Parallel()

		calls := 0
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				calls++
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*domain.RunStatus)) = domain.StatusExecuting
					return nil
				}}
			},
		}
		status, err := New(db).GetRunStatus(context.Background(), "run-1")
		require.NoError(t, err)
		require.Equal(t, domain.StatusExecuting, status)
		require.Equal(t, 1, calls)
	})

	t.Run("status falls back to history and maps missing rows", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				if sql == `SELECT status FROM job_runs_history WHERE id = $1` {
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*domain.RunStatus)) = domain.StatusCompleted
						return nil
					}}
				}
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}
		status, err := New(db).GetRunStatus(context.Background(), "run-1")
		require.NoError(t, err)
		require.Equal(t, domain.StatusCompleted, status)

		missingDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}
		_, err = New(missingDB).GetRunStatus(context.Background(), "missing-run")
		require.ErrorIs(t, err, ErrRunNotFound)
	})

	t.Run("status wraps hot and history errors", func(t *testing.T) {
		t.Parallel()

		hotDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("hot failed") }}
			},
		}
		_, err := New(hotDB).GetRunStatus(context.Background(), "run-1")
		require.ErrorContains(t, err, "get run status: hot failed")

		calls := 0
		historyDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				calls++
				if calls == 1 {
					return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
				}
				return &mockRow{scanFn: func(...any) error { return errors.New("history failed") }}
			},
		}
		_, err = New(historyDB).GetRunStatus(context.Background(), "run-1")
		require.ErrorContains(t, err, "get run status: history fallback: history failed")
	})

	t.Run("token state returns hot and history rows", func(t *testing.T) {
		t.Parallel()

		hotDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*domain.RunStatus)) = domain.StatusWaiting
					*(dest[1].(*int)) = 4
					*(dest[2].(*string)) = "project-1"
					return nil
				}}
			},
		}
		status, attempt, projectID, err := New(hotDB).GetRunTokenState(context.Background(), "run-1")
		require.NoError(t, err)
		require.Equal(t, domain.StatusWaiting, status)
		require.Equal(t, 4, attempt)
		require.Equal(t, "project-1", projectID)

		calls := 0
		historyDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				calls++
				if calls == 1 {
					return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
				}
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*domain.RunStatus)) = domain.StatusCompleted
					*(dest[1].(*int)) = 2
					*(dest[2].(*string)) = "project-history"
					return nil
				}}
			},
		}
		status, attempt, projectID, err = New(historyDB).GetRunTokenState(context.Background(), "run-1")
		require.NoError(t, err)
		require.Equal(t, domain.StatusCompleted, status)
		require.Equal(t, 2, attempt)
		require.Equal(t, "project-history", projectID)
	})

	t.Run("token state maps and wraps errors", func(t *testing.T) {
		t.Parallel()

		hotDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("hot token failed") }}
			},
		}
		_, _, _, err := New(hotDB).GetRunTokenState(context.Background(), "run-1")
		require.ErrorContains(t, err, "get run token state: hot token failed")

		missingDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}
		_, _, _, err = New(missingDB).GetRunTokenState(context.Background(), "run-1")
		require.ErrorIs(t, err, ErrRunNotFound)

		calls := 0
		historyDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				calls++
				if calls == 1 {
					return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
				}
				return &mockRow{scanFn: func(...any) error { return errors.New("history token failed") }}
			},
		}
		_, _, _, err = New(historyDB).GetRunTokenState(context.Background(), "run-1")
		require.ErrorContains(t, err, "get run token state: history fallback: history token failed")
	})
}

func TestEnsureRunActiveForAttemptUnit(t *testing.T) {
	t.Parallel()

	t.Run("accepts active matching attempt", func(t *testing.T) {
		t.Parallel()

		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, gotArgs ...any) pgx.Row {
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = true
					return nil
				}}
			},
		}
		err := New(db).EnsureRunActiveForAttempt(context.Background(), "run-1", 3)
		require.NoError(t, err)
		require.Equal(t, []any{"run-1", 3}, args)
	})

	t.Run("returns conflict when run is not active for attempt", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*bool)) = false
					return nil
				}}
			},
		}
		err := New(db).EnsureRunActiveForAttempt(context.Background(), "run-1", 3)
		require.ErrorIs(t, err, ErrRunConflict)
		require.ErrorContains(t, err, "run run-1 is not active for attempt 3")
	})

	t.Run("wraps scan errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("read failed") }}
			},
		}
		err := New(db).EnsureRunActiveForAttempt(context.Background(), "run-1", 3)
		require.ErrorContains(t, err, "ensure active run: read failed")
	})
}

func TestRunLookupUnit(t *testing.T) {
	t.Parallel()

	t.Run("get run scans hot row", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var query string
		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
				query = sql
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: runScanFn(now, true)}
			},
		}

		run, err := New(db).GetRun(context.Background(), "run-1")

		require.NoError(t, err)
		require.Contains(t, query, "FROM job_runs jr")
		require.Contains(t, query, "job_run_lifecycle_events")
		require.Equal(t, []any{"run-1"}, args)
		require.Equal(t, "run-1", run.ID)
		require.Equal(t, "job-1", run.JobID)
		require.Equal(t, "project-1", run.ProjectID)
		require.Equal(t, domain.StatusCompleted, run.Status)
		require.Equal(t, 2, run.Attempt)
		require.JSONEq(t, `{"input":true}`, string(run.Payload))
		require.JSONEq(t, `{"ok":true}`, string(run.Result))
		require.Equal(t, map[string]string{"tenant": "acme"}, run.Metadata)
		require.Equal(t, "failed", run.Error)
		require.Equal(t, "network", run.ErrorClass)
		require.NotNil(t, run.ExecutionTrace)
		require.Equal(t, int64(42), run.ExecutionTrace.TotalMs)
		require.Equal(t, map[string]string{"region": "iad"}, run.Tags)
		require.Equal(t, domain.ExecutionModeWorker, run.ExecutionMode)
		require.True(t, run.DebugMode)
		require.True(t, run.IsRollback)
		require.Equal(t, "replay-1", run.ReplayedRunID)
	})

	t.Run("get run falls back to history", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		calls := 0
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				calls++
				if strings.Contains(sql, "FROM job_runs_history") {
					return &mockRow{scanFn: runScanFn(now, false)}
				}
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}

		run, err := New(db).GetRun(context.Background(), "run-1")

		require.NoError(t, err)
		require.Equal(t, "run-1", run.ID)
		require.Equal(t, 2, calls)
	})

	t.Run("get run maps missing and wraps hot and history errors", func(t *testing.T) {
		t.Parallel()

		missingDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}
		run, err := New(missingDB).GetRun(context.Background(), "missing")
		require.Nil(t, run)
		require.ErrorIs(t, err, ErrRunNotFound)

		hotDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("hot failed") }}
			},
		}
		_, err = New(hotDB).GetRun(context.Background(), "run-1")
		require.ErrorContains(t, err, "get run: hot failed")

		calls := 0
		historyDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				calls++
				if calls == 1 {
					return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
				}
				return &mockRow{scanFn: func(...any) error { return errors.New("history failed") }}
			},
		}
		_, err = New(historyDB).GetRun(context.Background(), "run-1")
		require.ErrorContains(t, err, "get run: history fallback: get run from history: history failed")
	})

	t.Run("get run with cache version scans hot and history rows", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		hotDB := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				require.Contains(t, sql, "COALESCE(v.cache_version, jr.cache_version)")
				return &mockRow{scanFn: runScanFn(now, true)}
			},
		}
		run, version, err := New(hotDB).GetRunWithCacheVersion(context.Background(), "run-1")
		require.NoError(t, err)
		require.Equal(t, "run-1", run.ID)
		require.Equal(t, int64(42), version)

		calls := 0
		historyDB := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				calls++
				if strings.Contains(sql, "FROM job_runs_history") {
					return &mockRow{scanFn: runScanFn(now, false)}
				}
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}
		run, version, err = New(historyDB).GetRunWithCacheVersion(context.Background(), "run-1")
		require.NoError(t, err)
		require.Equal(t, "run-1", run.ID)
		require.Equal(t, int64(42), version)
		require.Equal(t, 2, calls)
	})

	t.Run("get run with cache version maps missing and wraps errors", func(t *testing.T) {
		t.Parallel()

		missingDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}
		run, version, err := New(missingDB).GetRunWithCacheVersion(context.Background(), "missing")
		require.Nil(t, run)
		require.Zero(t, version)
		require.ErrorIs(t, err, ErrRunNotFound)

		hotDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("hot failed") }}
			},
		}
		_, _, err = New(hotDB).GetRunWithCacheVersion(context.Background(), "run-1")
		require.ErrorContains(t, err, "get run with cache version: hot failed")

		calls := 0
		historyDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				calls++
				if calls == 1 {
					return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
				}
				return &mockRow{scanFn: func(...any) error { return errors.New("history failed") }}
			},
		}
		_, _, err = New(historyDB).GetRunWithCacheVersion(context.Background(), "run-1")
		require.ErrorContains(t, err, "get run with cache version: history fallback: get run from history with cache version: history failed")
	})
}

func TestRunLookupByIdempotencyAndPayloadUnit(t *testing.T) {
	t.Parallel()

	t.Run("gets run by idempotency key", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
				require.Contains(t, sql, "jr.idempotency_key = $2")
				require.Contains(t, sql, "NOW() - INTERVAL '24 hours'")
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: runScanFn(now, false)}
			},
		}

		run, err := New(db).GetRunByIdempotencyKey(context.Background(), "job-1", "idem-1")

		require.NoError(t, err)
		require.Equal(t, "run-1", run.ID)
		require.Equal(t, []any{"job-1", "idem-1"}, args)
	})

	t.Run("idempotency lookup maps not found to nil and wraps scan errors", func(t *testing.T) {
		t.Parallel()

		missingDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}
		run, err := New(missingDB).GetRunByIdempotencyKey(context.Background(), "job-1", "idem-1")
		require.NoError(t, err)
		require.Nil(t, run)

		errDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("scan failed") }}
			},
		}
		_, err = New(errDB).GetRunByIdempotencyKey(context.Background(), "job-1", "idem-1")
		require.ErrorContains(t, err, "get run by idempotency key: scan failed")
	})

	t.Run("finds recent run by payload", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		payload := json.RawMessage(`{"input":true}`)
		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
				require.Contains(t, sql, "payload = $2::jsonb")
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: runScanFn(now, false)}
			},
		}

		run, err := New(db).FindRecentRunByPayload(context.Background(), "job-1", payload, now)

		require.NoError(t, err)
		require.Equal(t, "run-1", run.ID)
		require.Equal(t, "job-1", args[0])
		require.JSONEq(t, `{"input":true}`, string(args[1].(json.RawMessage)))
		require.Equal(t, now, args[2])
	})

	t.Run("payload lookup sends nil empty payload and maps errors", func(t *testing.T) {
		t.Parallel()

		var args []any
		missingDB := &mockDBTX{
			queryRowFn: func(_ context.Context, _ string, gotArgs ...any) pgx.Row {
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}
		run, err := New(missingDB).FindRecentRunByPayload(context.Background(), "job-1", nil, time.Time{})
		require.NoError(t, err)
		require.Nil(t, run)
		require.Nil(t, args[1])

		errDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("scan failed") }}
			},
		}
		_, err = New(errDB).FindRecentRunByPayload(context.Background(), "job-1", nil, time.Time{})
		require.ErrorContains(t, err, "find recent run by payload: scan failed")
	})
}

func TestRunAggregateHelpersUnit(t *testing.T) {
	t.Parallel()

	t.Run("counts runs for job since", func(t *testing.T) {
		t.Parallel()

		since := time.Now().UTC()
		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
				require.Contains(t, sql, "SELECT COUNT(*)")
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 12
					return nil
				}}
			},
		}

		count, err := New(db).CountRunsForJobSince(context.Background(), "job-1", since)

		require.NoError(t, err)
		require.Equal(t, 12, count)
		require.Equal(t, []any{"job-1", since}, args)
	})

	t.Run("wraps count errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("count failed") }}
			},
		}
		_, err := New(db).CountRunsForJobSince(context.Background(), "job-1", time.Time{})
		require.ErrorContains(t, err, "count runs for job since: count failed")
	})

	t.Run("checks descendant terminal state", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name             string
			nonTerminalCount int
			want             bool
		}{
			{name: "all terminal", nonTerminalCount: 0, want: true},
			{name: "has active descendants", nonTerminalCount: 2, want: false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				db := &mockDBTX{
					queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
						require.Contains(t, sql, "WITH RECURSIVE descendants")
						require.Equal(t, []any{"parent-1"}, args)
						return &mockRow{scanFn: func(dest ...any) error {
							*(dest[0].(*int)) = tt.nonTerminalCount
							return nil
						}}
					},
				}
				got, err := New(db).AreAllDescendantsTerminal(context.Background(), "parent-1")
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("wraps descendant errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("descendants failed") }}
			},
		}
		_, err := New(db).AreAllDescendantsTerminal(context.Background(), "parent-1")
		require.ErrorContains(t, err, "check descendants terminal: descendants failed")
	})

	t.Run("sums run and project daily costs", func(t *testing.T) {
		t.Parallel()

		var runArgs []any
		runDB := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "strait:cost_recorded:")
				runArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int64)) = 99
					return nil
				}}
			},
		}
		total, err := New(runDB).SumRunCostMicrousd(context.Background(), "run-1")
		require.NoError(t, err)
		require.Equal(t, int64(99), total)
		require.Equal(t, []any{"run-1"}, runArgs)

		var projectArgs []any
		projectDB := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "AT TIME ZONE $2")
				projectArgs = append([]any(nil), args...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int64)) = 123
					return nil
				}}
			},
		}
		total, err = New(projectDB).SumProjectDailyCostMicrousd(context.Background(), "project-1", "")
		require.NoError(t, err)
		require.Equal(t, int64(123), total)
		require.Equal(t, []any{"project-1", "UTC"}, projectArgs)

		projectArgs = nil
		total, err = New(projectDB).SumProjectDailyCostMicrousd(context.Background(), "project-1", "America/New_York")
		require.NoError(t, err)
		require.Equal(t, int64(123), total)
		require.Equal(t, []any{"project-1", "America/New_York"}, projectArgs)
	})

	t.Run("wraps cost query errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("cost failed") }}
			},
		}
		_, err := New(db).SumRunCostMicrousd(context.Background(), "run-1")
		require.ErrorContains(t, err, "sum run cost: cost failed")

		_, err = New(db).SumProjectDailyCostMicrousd(context.Background(), "project-1", "")
		require.ErrorContains(t, err, "sum project daily cost: cost failed")
	})
}

func TestListRunsByJobUnit(t *testing.T) {
	t.Parallel()

	t.Run("scans rows and passes pagination arguments", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC()
		var args []any
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, gotArgs ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "WHERE jr.job_id = $1")
				args = append([]any(nil), gotArgs...)
				return &mockRows{scanFns: []func(dest ...any) error{
					runScanFn(now, false),
				}}, nil
			},
		}

		runs, err := New(db).ListRunsByJob(context.Background(), "job-1", 10, 20)

		require.NoError(t, err)
		require.Len(t, runs, 1)
		require.Equal(t, "run-1", runs[0].ID)
		require.Equal(t, []any{"job-1", 10, 20}, args)
	})

	t.Run("wraps query scan and rows errors", func(t *testing.T) {
		t.Parallel()

		queryErr := errors.New("query failed")
		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) { return nil, queryErr },
		}
		_, err := New(db).ListRunsByJob(context.Background(), "job-1", 10, 0)
		require.ErrorContains(t, err, "list runs by job: query failed")

		scanDB := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &mockRows{scanFns: []func(dest ...any) error{
					func(...any) error { return errors.New("scan failed") },
				}}, nil
			},
		}
		_, err = New(scanDB).ListRunsByJob(context.Background(), "job-1", 10, 0)
		require.ErrorContains(t, err, "list runs by job scan: scan failed")

		rowsDB := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &mockRows{err: errors.New("rows failed")}, nil
			},
		}
		_, err = New(rowsDB).ListRunsByJob(context.Background(), "job-1", 10, 0)
		require.ErrorContains(t, err, "list runs by job rows: rows failed")
	})
}

func TestListRunsByProjectQueryBuilderUnit(t *testing.T) {
	t.Parallel()

	t.Run("applies every optional filter with value arguments", func(t *testing.T) {
		t.Parallel()

		status := domain.StatusFailed
		metadataKey := "tenant"
		metadataValue := "acme"
		triggeredBy := "workflow"
		batchID := "batch-1"
		executionMode := domain.ExecutionModeWorker
		errorClass := "timeout"
		cursor := time.Now().UTC()
		payload := json.RawMessage(`{"tier":"gold"}`)
		var query string
		var args []any
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, gotArgs ...any) (pgx.Rows, error) {
				query = sql
				args = append([]any(nil), gotArgs...)
				return &mockRows{}, nil
			},
		}

		runs, err := New(db).ListRunsByProject(
			context.Background(),
			"project-1",
			&status,
			&metadataKey,
			&metadataValue,
			&triggeredBy,
			&batchID,
			payload,
			&executionMode,
			&errorClass,
			25,
			&cursor,
		)

		require.NoError(t, err)
		require.Empty(t, runs)
		require.Contains(t, query, "COALESCE(s.status, jr.status) = $2")
		require.Contains(t, query, "metadata_delta.metadata, '{}'::jsonb)) ->> $3 = $4")
		require.Contains(t, query, "jr.triggered_by = $5")
		require.Contains(t, query, "jr.batch_id = $6")
		require.Contains(t, query, "jr.payload @> $7::jsonb")
		require.Contains(t, query, "COALESCE(NULLIF(s.execution_mode, ''), jr.execution_mode) = $8")
		require.Contains(t, query, "jr.error_class END = $9")
		require.Contains(t, query, "jr.created_at < $10")
		require.Contains(t, query, "LIMIT $11")
		require.Equal(t, []any{
			"project-1",
			status,
			metadataKey,
			metadataValue,
			triggeredBy,
			batchID,
			payload,
			string(executionMode),
			errorClass,
			cursor,
			25,
		}, args)
	})

	t.Run("applies metadata key existence filter", func(t *testing.T) {
		t.Parallel()

		metadataKey := "tenant"
		var query string
		var args []any
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, gotArgs ...any) (pgx.Rows, error) {
				query = sql
				args = append([]any(nil), gotArgs...)
				return &mockRows{}, nil
			},
		}

		runs, err := New(db).ListRunsByProject(
			context.Background(), "project-1", nil, &metadataKey, nil, nil, nil, nil, nil, nil, 10, nil,
		)

		require.NoError(t, err)
		require.Empty(t, runs)
		require.Contains(t, query, "metadata_delta.metadata, '{}'::jsonb)) ? $2")
		require.Contains(t, query, "LIMIT $3")
		require.Equal(t, []any{"project-1", metadataKey, 10}, args)
	})

	t.Run("wraps query scan and rows errors", func(t *testing.T) {
		t.Parallel()

		queryErr := errors.New("query failed")
		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) { return nil, queryErr },
		}
		_, err := New(db).ListRunsByProject(context.Background(), "project-1", nil, nil, nil, nil, nil, nil, nil, nil, 10, nil)
		require.ErrorContains(t, err, "list runs by project: query failed")

		scanDB := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &mockRows{scanFns: []func(dest ...any) error{
					func(...any) error { return errors.New("scan failed") },
				}}, nil
			},
		}
		_, err = New(scanDB).ListRunsByProject(context.Background(), "project-1", nil, nil, nil, nil, nil, nil, nil, nil, 10, nil)
		require.ErrorContains(t, err, "list runs by project scan")
		require.ErrorContains(t, err, "scan failed")

		rowsDB := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &mockRows{err: errors.New("rows failed")}, nil
			},
		}
		_, err = New(rowsDB).ListRunsByProject(context.Background(), "project-1", nil, nil, nil, nil, nil, nil, nil, nil, 10, nil)
		require.ErrorContains(t, err, "list runs by project rows: rows failed")
	})
}

func TestListRunsByProjectFilteredQueryBuilderUnit(t *testing.T) {
	t.Parallel()

	t.Run("applies status tag environment and value filters", func(t *testing.T) {
		t.Parallel()

		status := domain.StatusFailed
		environmentID := "env-prod"
		metadataKey := "tenant"
		metadataValue := "acme"
		triggeredBy := "workflow"
		batchID := "batch-1"
		executionMode := domain.ExecutionModeWorker
		errorClass := "timeout"
		cursor := time.Now().UTC()
		payload := json.RawMessage(`{"tier":"gold"}`)
		var query string
		var args []any
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, gotArgs ...any) (pgx.Rows, error) {
				query = sql
				args = append([]any(nil), gotArgs...)
				return &mockRows{}, nil
			},
		}

		runs, err := New(db).ListRunsByProjectFiltered(
			context.Background(),
			"project-1",
			&status,
			nil,
			"team",
			"infra",
			&environmentID,
			&metadataKey,
			&metadataValue,
			&triggeredBy,
			&batchID,
			payload,
			&executionMode,
			&errorClass,
			25,
			&cursor,
		)

		require.NoError(t, err)
		require.Empty(t, runs)
		require.Contains(t, query, "JOIN jobs j ON j.id = jr.job_id")
		require.Contains(t, query, "COALESCE(s.status, jr.status) = $2")
		require.Contains(t, query, "jr.tags ->> $3 = $4")
		require.Contains(t, query, "j.environment_id = $5")
		require.Contains(t, query, "metadata_delta.metadata, '{}'::jsonb)) ->> $6 = $7")
		require.Contains(t, query, "jr.triggered_by = $8")
		require.Contains(t, query, "jr.batch_id = $9")
		require.Contains(t, query, "jr.payload @> $10::jsonb")
		require.Contains(t, query, "COALESCE(NULLIF(s.execution_mode, ''), jr.execution_mode) = $11")
		require.Contains(t, query, "jr.error_class END = $12")
		require.Contains(t, query, "jr.created_at < $13")
		require.Contains(t, query, "LIMIT $14")
		require.Equal(t, []any{
			"project-1",
			string(status),
			"team",
			"infra",
			environmentID,
			metadataKey,
			metadataValue,
			triggeredBy,
			batchID,
			payload,
			string(executionMode),
			errorClass,
			cursor,
			25,
		}, args)
	})

	t.Run("applies status list tag existence and metadata existence filters", func(t *testing.T) {
		t.Parallel()

		emptyEnvironment := ""
		metadataKey := "tenant"
		statuses := []domain.RunStatus{domain.StatusQueued, domain.StatusExecuting}
		var query string
		var args []any
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, gotArgs ...any) (pgx.Rows, error) {
				query = sql
				args = append([]any(nil), gotArgs...)
				return &mockRows{}, nil
			},
		}

		runs, err := New(db).ListRunsByProjectFiltered(
			context.Background(),
			"project-1",
			nil,
			statuses,
			"team",
			"",
			&emptyEnvironment,
			&metadataKey,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			10,
			nil,
		)

		require.NoError(t, err)
		require.Empty(t, runs)
		require.NotContains(t, query, "JOIN jobs j ON j.id = jr.job_id")
		require.Contains(t, query, "COALESCE(s.status, jr.status) = ANY($2)")
		require.Contains(t, query, "jr.tags ? $3")
		require.Contains(t, query, "metadata_delta.metadata, '{}'::jsonb)) ? $4")
		require.Contains(t, query, "LIMIT $5")
		require.Equal(t, []any{
			"project-1",
			[]string{string(domain.StatusQueued), string(domain.StatusExecuting)},
			"team",
			metadataKey,
			10,
		}, args)
	})

	t.Run("wraps query scan and rows errors", func(t *testing.T) {
		t.Parallel()

		queryErr := errors.New("query failed")
		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) { return nil, queryErr },
		}
		_, err := New(db).ListRunsByProjectFiltered(context.Background(), "project-1", nil, nil, "", "", nil, nil, nil, nil, nil, nil, nil, nil, 10, nil)
		require.ErrorContains(t, err, "list runs by project filtered: query failed")

		scanDB := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &mockRows{scanFns: []func(dest ...any) error{
					func(...any) error { return errors.New("scan failed") },
				}}, nil
			},
		}
		_, err = New(scanDB).ListRunsByProjectFiltered(context.Background(), "project-1", nil, nil, "", "", nil, nil, nil, nil, nil, nil, nil, nil, 10, nil)
		require.ErrorContains(t, err, "list runs by project filtered scan: scan failed")

		rowsDB := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &mockRows{err: errors.New("rows failed")}, nil
			},
		}
		_, err = New(rowsDB).ListRunsByProjectFiltered(context.Background(), "project-1", nil, nil, "", "", nil, nil, nil, nil, nil, nil, nil, nil, 10, nil)
		require.ErrorContains(t, err, "list runs by project filtered rows: rows failed")
	})
}

func TestListFinishedRunsSinceUnit(t *testing.T) {
	t.Parallel()

	t.Run("defaults nonpositive limit and returns empty rows", func(t *testing.T) {
		t.Parallel()

		var args []any
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, gotArgs ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "FROM job_runs jr")
				args = append([]any(nil), gotArgs...)
				return &mockRows{}, nil
			},
		}
		since := time.Now().UTC()

		runs, err := New(db).ListFinishedRunsSince(context.Background(), "project-1", since, "run-0", 0)

		require.NoError(t, err)
		require.Empty(t, runs)
		require.Equal(t, []any{"project-1", since, "run-0", 100}, args)
	})

	t.Run("wraps query and scan errors", func(t *testing.T) {
		t.Parallel()

		queryErr := errors.New("query failed")
		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) { return nil, queryErr },
		}
		_, err := New(db).ListFinishedRunsSince(context.Background(), "project-1", time.Now(), "", 10)
		require.ErrorContains(t, err, "list finished runs since: query failed")

		scanDB := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &mockRows{scanFns: []func(dest ...any) error{
					func(...any) error { return errors.New("scan failed") },
				}}, nil
			},
		}
		_, err = New(scanDB).ListFinishedRunsSince(context.Background(), "project-1", time.Now(), "", 10)
		require.ErrorContains(t, err, "list finished runs since scan: scan failed")
	})

	t.Run("returns rows error directly", func(t *testing.T) {
		t.Parallel()

		rowsErr := errors.New("rows failed")
		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &mockRows{err: rowsErr}, nil
			},
		}
		_, err := New(db).ListFinishedRunsSince(context.Background(), "project-1", time.Now(), "", 10)
		require.ErrorIs(t, err, rowsErr)
	})
}

func TestRunStateTransitionHelpersUnit(t *testing.T) {
	t.Parallel()

	require.True(t, activeClaimRunStateShouldRequeue(domain.StatusExecuting, domain.StatusQueued))
	require.True(t, activeClaimRunStateShouldRequeue(domain.StatusDequeued, domain.StatusQueued))
	require.False(t, activeClaimRunStateShouldRequeue(domain.StatusWaiting, domain.StatusQueued))
	require.False(t, activeClaimRunStateShouldRequeue(domain.StatusExecuting, domain.StatusCompleted))

	require.True(t, isActiveClaimRunStateStatus(domain.StatusExecuting))
	require.True(t, isActiveClaimRunStateStatus(domain.StatusDequeued))
	require.False(t, isActiveClaimRunStateStatus(domain.StatusQueued))

	require.True(t, terminalRunStateShouldMove(domain.StatusCompleted))
	require.False(t, terminalRunStateShouldMove(domain.StatusExecuting))
	require.True(t, terminalRunStateShouldReactivate(domain.StatusDeadLetter, domain.StatusQueued))
	require.False(t, terminalRunStateShouldReactivate(domain.StatusFailed, domain.StatusQueued))
	require.False(t, terminalRunStateShouldReactivate(domain.StatusDeadLetter, domain.StatusCompleted))

	fields := runLedgerFields(map[string]any{
		"payload":         json.RawMessage(`{"ok":true}`),
		"triggered_by":    "workflow",
		"lineage_depth":   2,
		"next_retry_at":   time.Now().UTC(),
		"concurrency_key": "tenant-1",
	})
	require.Len(t, fields, 3)
	require.Contains(t, fields, "payload")
	require.Contains(t, fields, "triggered_by")
	require.Contains(t, fields, "lineage_depth")
	require.NotContains(t, fields, "next_retry_at")
	require.NotContains(t, fields, "concurrency_key")
}

func TestAppendRunTerminalStateUnit(t *testing.T) {
	t.Parallel()

	t.Run("uses attempt query and normalizes optional fields", func(t *testing.T) {
		t.Parallel()

		attempt := 3
		var query string
		var args []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
				query = sql
				args = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 7
					return nil
				}}
			},
		}
		fields := map[string]any{
			"priority":        9,
			"concurrency_key": "",
			"execution_mode":  string(domain.ExecutionModeWorker),
		}

		moved, eventAttempt, err := New(db).appendRunTerminalState(
			context.Background(), "run-1", domain.StatusExecuting, domain.StatusCompleted, fields, &attempt,
		)

		require.NoError(t, err)
		require.True(t, moved)
		require.Equal(t, 7, eventAttempt)
		require.Contains(t, query, "COALESCE(c.attempt, ready.attempt, s.attempt) = $13")
		require.Len(t, args, 13)
		require.Equal(t, "run-1", args[0])
		require.Equal(t, domain.StatusExecuting, args[1])
		require.Equal(t, domain.StatusCompleted, args[2])
		require.Equal(t, 9, args[3])
		require.Nil(t, args[10])
		require.Equal(t, string(domain.ExecutionModeWorker), args[11])
		require.Equal(t, attempt, args[12])
	})

	t.Run("maps no rows to unmoved and wraps errors", func(t *testing.T) {
		t.Parallel()

		noRowsDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}
		moved, eventAttempt, err := New(noRowsDB).appendRunTerminalState(
			context.Background(), "run-1", domain.StatusExecuting, domain.StatusCompleted, nil, nil,
		)
		require.NoError(t, err)
		require.False(t, moved)
		require.Zero(t, eventAttempt)

		errDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("terminal insert failed") }}
			},
		}
		_, _, err = New(errDB).appendRunTerminalState(
			context.Background(), "run-1", domain.StatusExecuting, domain.StatusCompleted, nil, nil,
		)
		require.ErrorContains(t, err, "append run terminal state: terminal insert failed")
	})
}

func TestReactivateRunTerminalStateUnit(t *testing.T) {
	t.Parallel()

	t.Run("deletes terminal state and normalizes state update fields", func(t *testing.T) {
		t.Parallel()

		attempt := 2
		var deleteQuery string
		var deleteArgs []any
		var updateQuery string
		var updateArgs []any
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, gotArgs ...any) pgx.Row {
				deleteQuery = sql
				deleteArgs = append([]any(nil), gotArgs...)
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 4
					return nil
				}}
			},
			execFn: func(_ context.Context, sql string, gotArgs ...any) (pgconn.CommandTag, error) {
				updateQuery = sql
				updateArgs = append([]any(nil), gotArgs...)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}
		fields := map[string]any{
			"payload":         json.RawMessage(`{"ignored":"ledger"}`),
			"priority":        5,
			"concurrency_key": "",
			"execution_mode":  string(domain.ExecutionModeWorker),
		}

		moved, eventAttempt, err := New(db).reactivateRunTerminalState(
			context.Background(), "run-1", domain.StatusDeadLetter, domain.StatusQueued, fields, &attempt,
		)

		require.NoError(t, err)
		require.True(t, moved)
		require.Equal(t, 4, eventAttempt)
		require.Contains(t, deleteQuery, "AND attempt = $3")
		require.Equal(t, []any{"run-1", domain.StatusDeadLetter, attempt}, deleteArgs)
		require.Contains(t, updateQuery, "ready_generation = ready_generation + 1")
		require.Contains(t, updateQuery, "concurrency_key = $3")
		require.Contains(t, updateQuery, "execution_mode = $4")
		require.Contains(t, updateQuery, "priority = $5")
		require.Equal(t, []any{domain.StatusQueued, "run-1", nil, string(domain.ExecutionModeWorker), 5}, updateArgs)
	})

	t.Run("maps no terminal row to unmoved", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return pgx.ErrNoRows }}
			},
		}
		moved, eventAttempt, err := New(db).reactivateRunTerminalState(
			context.Background(), "run-1", domain.StatusDeadLetter, domain.StatusQueued, nil, nil,
		)
		require.NoError(t, err)
		require.False(t, moved)
		require.Zero(t, eventAttempt)
	})

	t.Run("wraps delete and update errors", func(t *testing.T) {
		t.Parallel()

		deleteDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error { return errors.New("delete failed") }}
			},
		}
		_, _, err := New(deleteDB).reactivateRunTerminalState(
			context.Background(), "run-1", domain.StatusDeadLetter, domain.StatusQueued, nil, nil,
		)
		require.ErrorContains(t, err, "reactivate terminal run state: delete failed")

		updateDB := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 1
					return nil
				}}
			},
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, errors.New("update failed")
			},
		}
		_, _, err = New(updateDB).reactivateRunTerminalState(
			context.Background(), "run-1", domain.StatusDeadLetter, domain.StatusQueued, nil, nil,
		)
		require.ErrorContains(t, err, "reactivate run state: update failed")
	})

	t.Run("maps missing state update to run not found", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 1
					return nil
				}}
			},
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("UPDATE 0"), nil
			},
		}
		_, _, err := New(db).reactivateRunTerminalState(
			context.Background(), "run-1", domain.StatusDeadLetter, domain.StatusQueued, nil, nil,
		)
		require.ErrorIs(t, err, ErrRunNotFound)
	})
}

func TestRunLifecycleEventUnit(t *testing.T) {
	t.Parallel()

	t.Run("falls back to current attempt and normalizes result bytes", func(t *testing.T) {
		t.Parallel()

		var execArgs []any
		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 0
					return nil
				}}
			},
			execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "INSERT INTO job_run_lifecycle_events")
				execArgs = append([]any(nil), args...)
				return pgconn.NewCommandTag("INSERT 1"), nil
			},
		}

		err := New(db).appendRunLifecycleEvent(
			context.Background(),
			"run-1",
			domain.StatusExecuting,
			domain.StatusCompleted,
			map[string]any{"result": []byte(`{"ok":true}`)},
			nil,
		)

		require.NoError(t, err)
		require.Len(t, execArgs, 5)
		require.Equal(t, "run-1", execArgs[0])
		require.Equal(t, domain.StatusExecuting, execArgs[1])
		require.Equal(t, domain.StatusCompleted, execArgs[2])
		require.Equal(t, 1, execArgs[3])
		require.JSONEq(t, `{"result":{"ok":true}}`, string(execArgs[4].([]byte)))
	})

	t.Run("uses explicit attempt and wraps marshal and exec errors", func(t *testing.T) {
		t.Parallel()

		attempt := 5
		var execArgs []any
		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				execArgs = append([]any(nil), attempt)
				return pgconn.NewCommandTag("INSERT 1"), nil
			},
		}
		err := New(db).appendRunLifecycleEvent(
			context.Background(), "run-1", domain.StatusQueued, domain.StatusExecuting, nil, &attempt,
		)
		require.NoError(t, err)
		require.Equal(t, []any{attempt}, execArgs)

		marshalErr := New(&mockDBTX{}).appendRunLifecycleEvent(
			context.Background(),
			"run-1",
			domain.StatusQueued,
			domain.StatusExecuting,
			map[string]any{"bad": make(chan int)},
			&attempt,
		)
		require.ErrorContains(t, marshalErr, "marshal run lifecycle fields")

		execDB := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, errors.New("insert failed")
			},
		}
		err = New(execDB).appendRunLifecycleEvent(
			context.Background(), "run-1", domain.StatusQueued, domain.StatusExecuting, nil, &attempt,
		)
		require.ErrorContains(t, err, "append run lifecycle event: insert failed")
	})

	t.Run("normalizes empty and non-result fields", func(t *testing.T) {
		t.Parallel()

		require.Nil(t, normalizeRunLifecycleFields(nil))
		require.Empty(t, normalizeRunLifecycleFields(map[string]any{}))

		fields := normalizeRunLifecycleFields(map[string]any{
			"result": []byte(`{"ok":true}`),
			"error":  "failed",
		})
		require.IsType(t, json.RawMessage{}, fields["result"])
		require.Equal(t, "failed", fields["error"])
	})
}

func TestUpdateRunLedgerFieldsUnit(t *testing.T) {
	t.Parallel()

	t.Run("returns without exec for empty fields", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				require.Fail(t, "empty ledger update should not execute SQL")
				return pgconn.CommandTag{}, nil
			},
		}
		require.NoError(t, New(db).updateRunLedgerFields(context.Background(), "run-1", nil))
	})

	t.Run("builds deterministic update and normalizes field values", func(t *testing.T) {
		t.Parallel()

		var query string
		var args []any
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, gotArgs ...any) (pgconn.CommandTag, error) {
				query = sql
				args = append([]any(nil), gotArgs...)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}
		trace := &domain.ExecutionTrace{TotalMs: 42}
		fields := map[string]any{
			"metadata":        map[string]string{"tenant": "acme"},
			"execution_trace": trace,
			"error":           "",
			"result":          json.RawMessage{},
		}

		err := New(db).updateRunLedgerFields(context.Background(), "run-1", fields)

		require.NoError(t, err)
		require.Contains(t, query, "error = $2")
		require.Contains(t, query, "execution_trace = $3")
		require.Contains(t, query, "metadata = COALESCE(metadata, '{}'::jsonb) || $4::jsonb")
		require.Contains(t, query, "result = $5")
		require.Contains(t, query, "IS DISTINCT FROM")
		require.Equal(t, "run-1", args[0])
		require.Nil(t, args[1])
		require.JSONEq(t, `{"total_ms":42,"queue_wait_ms":0,"dequeue_ms":0,"connect_ms":0,"ttfb_ms":0,"transfer_ms":0,"dispatch_ms":0}`, string(args[2].(json.RawMessage)))
		require.JSONEq(t, `{"tenant":"acme"}`, string(args[3].([]byte)))
		require.Nil(t, args[4])
	})

	t.Run("accepts value execution trace and nil pointer trace", func(t *testing.T) {
		t.Parallel()

		var args []any
		db := &mockDBTX{
			execFn: func(_ context.Context, _ string, gotArgs ...any) (pgconn.CommandTag, error) {
				args = append([]any(nil), gotArgs...)
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}

		err := New(db).updateRunLedgerFields(context.Background(), "run-1", map[string]any{
			"execution_trace": domain.ExecutionTrace{DispatchMs: 7},
		})
		require.NoError(t, err)
		require.JSONEq(t, `{"dispatch_ms":7,"queue_wait_ms":0,"dequeue_ms":0,"connect_ms":0,"ttfb_ms":0,"transfer_ms":0,"total_ms":0}`, string(args[1].(json.RawMessage)))

		args = nil
		var nilTrace *domain.ExecutionTrace
		err = New(db).updateRunLedgerFields(context.Background(), "run-1", map[string]any{
			"execution_trace": nilTrace,
		})
		require.NoError(t, err)
		require.Nil(t, args[1])
	})

	t.Run("wraps exec errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, errors.New("update failed")
			},
		}
		err := New(db).updateRunLedgerFields(context.Background(), "run-1", map[string]any{"error": "failed"})
		require.ErrorContains(t, err, "update run ledger fields: update failed")
	})
}
