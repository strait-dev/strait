package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestAcquireTriggerAdmissionLocks_UsesRowLocksWithoutAdvisoryLock(t *testing.T) {
	t.Parallel()

	tx := &triggerAdmissionTx{}
	job := &domain.Job{
		ID:                  "job-1",
		ProjectID:           "project-1",
		RateLimitMax:        10,
		RateLimitWindowSecs: int(time.Minute.Seconds()),
	}
	quota := &store.ProjectQuota{ProjectID: job.ProjectID, MaxQueuedRuns: 100}
	require.NoError(t, acquireTriggerAdmissionLocks(context.Background(), tx, job, quota))

	joined := strings.Join(append(tx.execSQL, tx.queryRowSQL...), "\n")
	require.NotContains(t, joined, "pg_advisory_xact_lock")
	require.Contains(
		t, joined, "SET LOCAL lock_timeout",
	)
	require.False(t, !strings.Contains(
		joined, "FROM project_quotas",
	) ||
		!strings.Contains(joined,
			"FROM jobs"))
}

func TestAcquireTriggerAdmissionLocks_NoLimitsSkipsDatabaseWork(t *testing.T) {
	t.Parallel()

	tx := &triggerAdmissionTx{}
	job := &domain.Job{ID: "job-1", ProjectID: "project-1"}
	require.NoError(t, acquireTriggerAdmissionLocks(context.Background(), tx, job, nil))
	require.False(t, len(tx.
		execSQL) !=
		0 || len(tx.
		queryRowSQL,
	) != 0,
	)
}

func TestCheckTriggerLimitsInTx_UsesTransactionalCounts(t *testing.T) {
	t.Parallel()

	tx := &triggerAdmissionTx{
		counts: map[string]int{
			"queued": 0,
			"active": 0,
			"job":    0,
		},
	}
	srv := &Server{}
	job := &domain.Job{
		ID:                  "job-1",
		ProjectID:           "project-1",
		RateLimitMax:        10,
		RateLimitWindowSecs: int(time.Minute.Seconds()),
	}
	quota := &store.ProjectQuota{ProjectID: job.ProjectID, MaxQueuedRuns: 100, MaxExecutingRuns: 100}
	require.NoError(t, srv.
		checkTriggerLimitsInTx(
			context.Background(),
			tx, job, quota))

	joined := strings.Join(tx.queryRowSQL, "\n")
	require.Equal(t, 3, strings.Count(joined,
		"FROM job_runs",
	))
}

func TestCheckTriggerDispatchPrioritySkipsZeroPriority(t *testing.T) {
	t.Parallel()

	enforcer := &triggerPriorityAdmissionEnforcer{
		checkFunc: func(context.Context, string, int) error {
			require.Fail(t,

				"CheckMaxDispatchPriority must not run for zero-priority triggers")
			return nil
		},
	}
	srv := &Server{edition: domain.EditionCloud, billingEnforcer: enforcer}
	require.NoError(t, srv.
		checkTriggerDispatchPriority(context.
			Background(), "project-1", 0))
}

func TestCheckTriggerDispatchPriorityMapsPlanErrorTo402(t *testing.T) {
	t.Parallel()

	enforcer := &triggerPriorityAdmissionEnforcer{
		checkFunc: func(_ context.Context, projectID string, priority int) error {
			require.Equal(t, "project-1",
				projectID,
			)
			require.Equal(t, 9, priority)

			return errors.New("dispatch priority exceeds plan limit")
		},
	}
	srv := &Server{edition: domain.EditionCloud, billingEnforcer: enforcer}

	err := srv.checkTriggerDispatchPriority(context.Background(), "project-1", 9)
	var statusErr huma.StatusError
	require.ErrorAs(
		t, err, &statusErr)
	require.Equal(t, http.StatusPaymentRequired,

		statusErr.
			GetStatus(),
	)
	require.Contains(
		t, err.
			Error(), "dispatch priority exceeds plan limit")
}

func TestCheckTriggerDailyCostBudgetUsesUTCDefault(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		SumProjectDailyCostMicrousdFunc: func(_ context.Context, projectID, timezone string) (int64, error) {
			require.Equal(t, "project-1",
				projectID,
			)
			require.Equal(t, "UTC",
				timezone)

			return 4999, nil
		},
	}}

	err := srv.checkTriggerDailyCostBudget(context.Background(), "project-1", &store.ProjectQuota{
		ProjectID:            "project-1",
		MaxDailyCostMicrousd: 5000,
	})
	require.NoError(t, err)
}

func TestCheckTriggerDailyCostBudgetRejectsAtLimit(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		SumProjectDailyCostMicrousdFunc: func(_ context.Context, _ string, timezone string) (int64, error) {
			require.Equal(t, "Europe/Madrid",
				timezone,
			)

			return 5000, nil
		},
	}}

	err := srv.checkTriggerDailyCostBudget(context.Background(), "project-1", &store.ProjectQuota{
		ProjectID:            "project-1",
		MaxDailyCostMicrousd: 5000,
		Timezone:             "Europe/Madrid",
	})
	var statusErr huma.StatusError
	require.ErrorAs(
		t, err, &statusErr)
	require.Equal(t, http.StatusTooManyRequests,

		statusErr.
			GetStatus(),
	)
}

func TestTriggerAdmissionContentionMapsToRetryable429(t *testing.T) {
	t.Parallel()

	tx := &triggerAdmissionTx{queryRowErr: &pgconn.PgError{Code: "55P03"}}
	job := &domain.Job{ID: "job-1", ProjectID: "project-1"}
	quota := &store.ProjectQuota{ProjectID: job.ProjectID, MaxQueuedRuns: 1}

	err := acquireTriggerAdmissionLocks(context.Background(), tx, job, quota)
	require.ErrorIs(
		t, err, errTriggerAdmissionContended)

	apiErr := triggerLimitAPIError(err, "failed to trigger job")
	var statusErr huma.StatusError
	require.ErrorAs(
		t, apiErr, &statusErr)
	require.Equal(t, http.StatusTooManyRequests,

		statusErr.
			GetStatus(),
	)
	require.Contains(
		t, apiErr.
			Error(), "trigger admission busy")
}

func TestClassifyTriggerAdmissionLockError_DeadlockIsContention(t *testing.T) {
	t.Parallel()

	err := classifyTriggerAdmissionLockError(&pgconn.PgError{Code: "40P01"})
	require.ErrorIs(
		t, err, errTriggerAdmissionContended)
}

func BenchmarkTriggerAdmissionRowLocks(b *testing.B) {
	job := &domain.Job{
		ID:                  "job-1",
		ProjectID:           "project-1",
		RateLimitMax:        10,
		RateLimitWindowSecs: int(time.Minute.Seconds()),
	}
	quota := &store.ProjectQuota{ProjectID: job.ProjectID, MaxQueuedRuns: 100, MaxExecutingRuns: 50}

	b.ReportAllocs()
	for b.Loop() {
		tx := &triggerAdmissionTx{}
		if err := acquireTriggerAdmissionLocks(context.Background(), tx, job, quota); err != nil {
			b.Fatal(err)
		}
	}
}

func FuzzTriggerAdmissionQuotaModel(f *testing.F) {
	f.Add(0, 1, 1)
	f.Add(1, 1, 1)
	f.Add(10, 20, 5)

	f.Fuzz(func(t *testing.T, existing, limit, batch int) {
		existing = absInt(existing % 1000)
		limit = absInt(limit % 1000)
		batch = absInt(batch % 1000)

		accepted := 0
		for range batch {
			if limit > 0 && existing+accepted >= limit {
				break
			}
			accepted++
		}
		require.GreaterOrEqual(
			t, accepted,
			0)
		require.False(t, limit >
			0 && existing <
			limit &&
			existing+
				accepted >
				limit)
		require.False(t, limit >
			0 && existing >=
			limit &&
			accepted !=
				0)
		require.False(t, limit ==
			0 && accepted !=
			batch,
		)
	})
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

type triggerPriorityAdmissionEnforcer struct {
	tunableLimitsEnforcer
	checkFunc func(context.Context, string, int) error
}

func (e *triggerPriorityAdmissionEnforcer) CheckMaxDispatchPriority(ctx context.Context, projectID string, priority int) error {
	return e.checkFunc(ctx, projectID, priority)
}

type triggerAdmissionTx struct {
	execSQL     []string
	queryRowSQL []string
	queryRowErr error
	counts      map[string]int
}

func (tx *triggerAdmissionTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	tx.execSQL = append(tx.execSQL, sql)
	return pgconn.CommandTag{}, nil
}

func (tx *triggerAdmissionTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("triggerAdmissionTx does not support Query")
}

func (tx *triggerAdmissionTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	tx.queryRowSQL = append(tx.queryRowSQL, sql)
	if tx.queryRowErr != nil {
		return triggerAdmissionRow{err: tx.queryRowErr}
	}
	normalized := strings.Join(strings.Fields(sql), " ")
	switch {
	case strings.Contains(normalized, "COUNT(*)") && strings.Contains(normalized, "status IN ('queued', 'delayed')"):
		return triggerAdmissionRow{value: tx.counts["queued"]}
	case strings.Contains(normalized, "COUNT(*)") && strings.Contains(normalized, "status IN ('dequeued', 'executing')"):
		return triggerAdmissionRow{value: tx.counts["active"]}
	case strings.Contains(normalized, "COUNT(*)") && strings.Contains(normalized, "created_at >= $2"):
		return triggerAdmissionRow{value: tx.counts["job"]}
	case strings.Contains(normalized, "FROM project_quotas"):
		return triggerAdmissionRow{value: "project-1"}
	case strings.Contains(normalized, "FROM jobs"):
		return triggerAdmissionRow{value: "job-1"}
	default:
		return triggerAdmissionRow{err: errors.New("unexpected QueryRow SQL: " + normalized)}
	}
}

type triggerAdmissionRow struct {
	value any
	err   error
}

func (r triggerAdmissionRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return errors.New("triggerAdmissionRow expects exactly one scan destination")
	}
	switch d := dest[0].(type) {
	case *int:
		v, ok := r.value.(int)
		if !ok {
			return errors.New("triggerAdmissionRow value is not int")
		}
		*d = v
	case *string:
		v, ok := r.value.(string)
		if !ok {
			return errors.New("triggerAdmissionRow value is not string")
		}
		*d = v
	default:
		return errors.New("triggerAdmissionRow unsupported scan destination")
	}
	return nil
}
