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

	if err := acquireTriggerAdmissionLocks(context.Background(), tx, job, quota); err != nil {
		t.Fatalf("acquireTriggerAdmissionLocks() error = %v", err)
	}

	joined := strings.Join(append(tx.execSQL, tx.queryRowSQL...), "\n")
	if strings.Contains(joined, "pg_advisory_xact_lock") {
		t.Fatalf("trigger admission still uses advisory lock SQL:\n%s", joined)
	}
	if !strings.Contains(joined, "SET LOCAL lock_timeout") {
		t.Fatalf("trigger admission did not set bounded lock timeout:\n%s", joined)
	}
	if !strings.Contains(joined, "FROM project_quotas") || !strings.Contains(joined, "FROM jobs") {
		t.Fatalf("trigger admission did not lock project quota and job rows:\n%s", joined)
	}
}

func TestAcquireTriggerAdmissionLocks_NoLimitsSkipsDatabaseWork(t *testing.T) {
	t.Parallel()

	tx := &triggerAdmissionTx{}
	job := &domain.Job{ID: "job-1", ProjectID: "project-1"}

	if err := acquireTriggerAdmissionLocks(context.Background(), tx, job, nil); err != nil {
		t.Fatalf("acquireTriggerAdmissionLocks() error = %v", err)
	}
	if len(tx.execSQL) != 0 || len(tx.queryRowSQL) != 0 {
		t.Fatalf("acquireTriggerAdmissionLocks() issued SQL with no admission limits: exec=%v query=%v", tx.execSQL, tx.queryRowSQL)
	}
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

	if err := srv.checkTriggerLimitsInTx(context.Background(), tx, job, quota); err != nil {
		t.Fatalf("checkTriggerLimitsInTx() error = %v", err)
	}

	joined := strings.Join(tx.queryRowSQL, "\n")
	if strings.Count(joined, "FROM job_runs") != 3 {
		t.Fatalf("checkTriggerLimitsInTx() did not perform all transactional count queries:\n%s", joined)
	}
}

func TestCheckTriggerDispatchPrioritySkipsZeroPriority(t *testing.T) {
	t.Parallel()

	enforcer := &triggerPriorityAdmissionEnforcer{
		checkFunc: func(context.Context, string, int) error {
			t.Fatal("CheckMaxDispatchPriority must not run for zero-priority triggers")
			return nil
		},
	}
	srv := &Server{edition: domain.EditionCloud, billingEnforcer: enforcer}

	if err := srv.checkTriggerDispatchPriority(context.Background(), "project-1", 0); err != nil {
		t.Fatalf("checkTriggerDispatchPriority() error = %v", err)
	}
}

func TestCheckTriggerDispatchPriorityMapsPlanErrorTo402(t *testing.T) {
	t.Parallel()

	enforcer := &triggerPriorityAdmissionEnforcer{
		checkFunc: func(_ context.Context, projectID string, priority int) error {
			if projectID != "project-1" {
				t.Fatalf("projectID = %q, want project-1", projectID)
			}
			if priority != 9 {
				t.Fatalf("priority = %d, want 9", priority)
			}
			return errors.New("dispatch priority exceeds plan limit")
		},
	}
	srv := &Server{edition: domain.EditionCloud, billingEnforcer: enforcer}

	err := srv.checkTriggerDispatchPriority(context.Background(), "project-1", 9)
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("checkTriggerDispatchPriority() = %T, want huma.StatusError", err)
	}
	if statusErr.GetStatus() != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", statusErr.GetStatus())
	}
	if !strings.Contains(err.Error(), "dispatch priority exceeds plan limit") {
		t.Fatalf("error = %v, want plan-limit message", err)
	}
}

func TestCheckTriggerDailyCostBudgetUsesUTCDefault(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		SumProjectDailyCostMicrousdFunc: func(_ context.Context, projectID, timezone string) (int64, error) {
			if projectID != "project-1" {
				t.Fatalf("projectID = %q, want project-1", projectID)
			}
			if timezone != "UTC" {
				t.Fatalf("timezone = %q, want UTC", timezone)
			}
			return 4999, nil
		},
	}}

	err := srv.checkTriggerDailyCostBudget(context.Background(), "project-1", &store.ProjectQuota{
		ProjectID:            "project-1",
		MaxDailyCostMicrousd: 5000,
	})
	if err != nil {
		t.Fatalf("checkTriggerDailyCostBudget() error = %v", err)
	}
}

func TestCheckTriggerDailyCostBudgetRejectsAtLimit(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		SumProjectDailyCostMicrousdFunc: func(_ context.Context, _ string, timezone string) (int64, error) {
			if timezone != "Europe/Madrid" {
				t.Fatalf("timezone = %q, want Europe/Madrid", timezone)
			}
			return 5000, nil
		},
	}}

	err := srv.checkTriggerDailyCostBudget(context.Background(), "project-1", &store.ProjectQuota{
		ProjectID:            "project-1",
		MaxDailyCostMicrousd: 5000,
		Timezone:             "Europe/Madrid",
	})
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("checkTriggerDailyCostBudget() = %T, want huma.StatusError", err)
	}
	if statusErr.GetStatus() != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", statusErr.GetStatus())
	}
}

func TestTriggerAdmissionContentionMapsToRetryable429(t *testing.T) {
	t.Parallel()

	tx := &triggerAdmissionTx{queryRowErr: &pgconn.PgError{Code: "55P03"}}
	job := &domain.Job{ID: "job-1", ProjectID: "project-1"}
	quota := &store.ProjectQuota{ProjectID: job.ProjectID, MaxQueuedRuns: 1}

	err := acquireTriggerAdmissionLocks(context.Background(), tx, job, quota)
	if !errors.Is(err, errTriggerAdmissionContended) {
		t.Fatalf("acquireTriggerAdmissionLocks() error = %v, want errTriggerAdmissionContended", err)
	}

	apiErr := triggerLimitAPIError(err, "failed to trigger job")
	var statusErr huma.StatusError
	if !errors.As(apiErr, &statusErr) {
		t.Fatalf("triggerLimitAPIError() = %T, want huma.StatusError", apiErr)
	}
	if statusErr.GetStatus() != http.StatusTooManyRequests {
		t.Fatalf("triggerLimitAPIError() status = %d, want %d", statusErr.GetStatus(), http.StatusTooManyRequests)
	}
	if !strings.Contains(apiErr.Error(), "trigger admission busy") {
		t.Fatalf("triggerLimitAPIError() = %v, want trigger admission busy", apiErr)
	}
}

func TestClassifyTriggerAdmissionLockError_DeadlockIsContention(t *testing.T) {
	t.Parallel()

	err := classifyTriggerAdmissionLockError(&pgconn.PgError{Code: "40P01"})
	if !errors.Is(err, errTriggerAdmissionContended) {
		t.Fatalf("classifyTriggerAdmissionLockError() = %v, want errTriggerAdmissionContended", err)
	}
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

		if accepted < 0 {
			t.Fatalf("accepted = %d, want non-negative", accepted)
		}
		if limit > 0 && existing < limit && existing+accepted > limit {
			t.Fatalf("existing=%d accepted=%d limit=%d exceeds quota", existing, accepted, limit)
		}
		if limit > 0 && existing >= limit && accepted != 0 {
			t.Fatalf("already-over-quota existing=%d accepted=%d limit=%d, want 0 accepted", existing, accepted, limit)
		}
		if limit == 0 && accepted != batch {
			t.Fatalf("unlimited quota accepted = %d, want batch %d", accepted, batch)
		}
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
