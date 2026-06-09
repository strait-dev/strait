package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	errTriggerProjectQueuedQuotaExceeded    = errors.New("project queued quota exceeded")
	errTriggerProjectExecutingQuotaExceeded = errors.New("project executing quota exceeded")
	errTriggerJobRateLimitExceeded          = errors.New("job rate limit exceeded")
	errTriggerAdmissionContended            = errors.New("trigger admission contended")
)

type triggerLimitTransactioner interface {
	WithTx(ctx context.Context, fn func(context.Context, store.DBTX) error) error
}

const triggerAdmissionLockTimeout = "2500ms"
const setTriggerAdmissionLockTimeoutSQL = "SET LOCAL lock_timeout = '" + triggerAdmissionLockTimeout + "'"

func (s *Server) withTriggerLimitGuard(ctx context.Context, job *domain.Job, quota *store.ProjectQuota, fn func(context.Context, store.DBTX) error) error {
	if txer, ok := s.store.(triggerLimitTransactioner); ok {
		return txer.WithTx(ctx, func(txCtx context.Context, tx store.DBTX) error {
			if err := acquireTriggerAdmissionLocks(txCtx, tx, job, quota); err != nil {
				return err
			}
			if err := s.checkTriggerLimitsInTx(txCtx, tx, job, quota); err != nil {
				return err
			}
			return fn(txCtx, tx)
		})
	}
	if err := s.checkTriggerLimits(ctx, job, quota); err != nil {
		return err
	}
	return fn(ctx, nil)
}

func (s *Server) checkTriggerDispatchPriority(ctx context.Context, projectID string, priority int) error {
	if priority <= 0 {
		return nil
	}
	if !s.edition.RequiresHTTPModeGating() {
		return nil
	}
	if s.billingEnforcer == nil {
		return planGateUnavailable("dispatch_priority_enforcer", errors.New("billing enforcer not configured"))
	}
	if err := s.billingEnforcer.CheckMaxDispatchPriority(ctx, projectID, priority); err != nil {
		var rse *rawStatusError
		if converted := limitErrorTo402(err, ""); converted != nil && errors.As(converted, &rse) {
			return converted
		}
		return huma.Error402PaymentRequired(err.Error())
	}
	return nil
}

func (s *Server) checkTriggerDailyCostBudget(ctx context.Context, projectID string, projectQuota *store.ProjectQuota) error {
	if projectQuota == nil || projectQuota.MaxDailyCostMicrousd <= 0 {
		return nil
	}
	tz := projectQuota.Timezone
	if tz == "" {
		tz = "UTC"
	}
	dailyCost, err := s.store.SumProjectDailyCostMicrousd(ctx, projectID, tz)
	if err != nil {
		return huma.Error500InternalServerError(fmt.Sprintf("failed to evaluate daily cost budget (timezone: %s)", tz))
	}
	if dailyCost >= projectQuota.MaxDailyCostMicrousd {
		return huma.Error429TooManyRequests("project daily cost budget exceeded")
	}
	return nil
}

func acquireTriggerAdmissionLocks(ctx context.Context, tx store.DBTX, job *domain.Job, quota *store.ProjectQuota) error {
	if tx == nil || job == nil {
		return nil
	}
	needsProjectLock := quota != nil && (quota.MaxQueuedRuns > 0 || quota.MaxExecutingRuns > 0)
	needsJobLock := jobHasRateLimit(job)
	if !needsProjectLock && !needsJobLock {
		return nil
	}

	if _, err := tx.Exec(ctx, setTriggerAdmissionLockTimeoutSQL); err != nil {
		return fmt.Errorf("set trigger admission lock timeout: %w", err)
	}
	if needsProjectLock {
		var projectID string
		if err := tx.QueryRow(ctx, `
			SELECT project_id
			FROM project_quotas
			WHERE project_id = $1
			FOR UPDATE`, job.ProjectID).Scan(&projectID); err != nil {
			return classifyTriggerAdmissionLockError(err)
		}
	}
	if needsJobLock {
		var jobID string
		if err := tx.QueryRow(ctx, `
			SELECT id
			FROM jobs
			WHERE id = $1
			FOR UPDATE`, job.ID).Scan(&jobID); err != nil {
			return classifyTriggerAdmissionLockError(err)
		}
	}
	return nil
}

func classifyTriggerAdmissionLockError(err error) error {
	if err == nil {
		return nil
	}
	if isTriggerAdmissionContention(err) {
		return errTriggerAdmissionContended
	}
	return fmt.Errorf("acquire trigger admission lock: %w", err)
}

func isTriggerAdmissionContention(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case "40P01", "55P03":
		return true
	default:
		return false
	}
}

func (s *Server) checkTriggerLimits(ctx context.Context, job *domain.Job, quota *store.ProjectQuota) error {
	if quota == nil {
		return s.checkJobRateLimit(ctx, job)
	}
	if quota.MaxQueuedRuns > 0 {
		queuedRuns, countErr := s.store.CountProjectQueuedRuns(ctx, job.ProjectID)
		if countErr != nil {
			return fmt.Errorf("evaluate project queued quota: %w", countErr)
		}
		if queuedRuns >= quota.MaxQueuedRuns {
			return errTriggerProjectQueuedQuotaExceeded
		}
	}
	if quota.MaxExecutingRuns > 0 {
		activeRuns, countErr := s.store.CountProjectActiveRuns(ctx, job.ProjectID)
		if countErr != nil {
			return fmt.Errorf("evaluate project active quota: %w", countErr)
		}
		if activeRuns >= quota.MaxExecutingRuns {
			return errTriggerProjectExecutingQuotaExceeded
		}
	}
	return s.checkJobRateLimit(ctx, job)
}

func jobHasRateLimit(job *domain.Job) bool {
	return job.RateLimitMax > 0 && job.RateLimitWindowSecs > 0
}

func (s *Server) checkJobRateLimit(ctx context.Context, job *domain.Job) error {
	if jobHasRateLimit(job) {
		since := time.Now().Add(-time.Duration(job.RateLimitWindowSecs) * time.Second)
		runCount, countErr := s.store.CountRunsForJobSince(ctx, job.ID, since)
		if countErr != nil {
			return fmt.Errorf("evaluate job rate limit: %w", countErr)
		}
		if runCount >= job.RateLimitMax {
			return errTriggerJobRateLimitExceeded
		}
	}

	return nil
}

func (s *Server) checkTriggerLimitsInTx(ctx context.Context, tx store.DBTX, job *domain.Job, quota *store.ProjectQuota) error {
	if tx == nil {
		return s.checkTriggerLimits(ctx, job, quota)
	}
	if err := checkProjectQuotaInTx(ctx, tx, job, quota); err != nil {
		return err
	}
	return checkJobRateLimitInTx(ctx, tx, job)
}

func checkProjectQuotaInTx(ctx context.Context, tx store.DBTX, job *domain.Job, quota *store.ProjectQuota) error {
	if quota == nil {
		return nil
	}
	if quota.MaxQueuedRuns > 0 {
		queuedRuns, countErr := countProjectQueuedRuns(ctx, tx, job.ProjectID)
		if countErr != nil {
			return fmt.Errorf("evaluate project queued quota: %w", countErr)
		}
		if queuedRuns >= quota.MaxQueuedRuns {
			return errTriggerProjectQueuedQuotaExceeded
		}
	}
	if quota.MaxExecutingRuns > 0 {
		activeRuns, countErr := countProjectActiveRuns(ctx, tx, job.ProjectID)
		if countErr != nil {
			return fmt.Errorf("evaluate project active quota: %w", countErr)
		}
		if activeRuns >= quota.MaxExecutingRuns {
			return errTriggerProjectExecutingQuotaExceeded
		}
	}
	return nil
}

func checkJobRateLimitInTx(ctx context.Context, tx store.DBTX, job *domain.Job) error {
	if !jobHasRateLimit(job) {
		return nil
	}
	since := time.Now().Add(-time.Duration(job.RateLimitWindowSecs) * time.Second)
	runCount, countErr := countRunsForJobSince(ctx, tx, job.ID, since)
	if countErr != nil {
		return fmt.Errorf("evaluate job rate limit: %w", countErr)
	}
	if runCount >= job.RateLimitMax {
		return errTriggerJobRateLimitExceeded
	}
	return nil
}

func countProjectQueuedRuns(ctx context.Context, tx store.DBTX, projectID string) (int, error) {
	const query = `
		SELECT COUNT(*)
		FROM job_runs
		WHERE project_id = $1 AND status IN ('queued', 'delayed')`

	var count int
	if err := tx.QueryRow(ctx, query, projectID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count project queued runs: %w", err)
	}
	return count, nil
}

func countProjectActiveRuns(ctx context.Context, tx store.DBTX, projectID string) (int, error) {
	const query = `
		SELECT COUNT(*)
		FROM job_runs
		WHERE project_id = $1 AND status IN ('dequeued', 'executing')`

	var count int
	if err := tx.QueryRow(ctx, query, projectID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count project active runs: %w", err)
	}
	return count, nil
}

func countRunsForJobSince(ctx context.Context, tx store.DBTX, jobID string, since time.Time) (int, error) {
	const query = `
		SELECT COUNT(*)
		FROM job_runs
		WHERE job_id = $1
		  AND created_at >= $2`

	var count int
	if err := tx.QueryRow(ctx, query, jobID, since).Scan(&count); err != nil {
		return 0, fmt.Errorf("count runs for job since: %w", err)
	}
	return count, nil
}

// triggerLimitFallbackRetryAfterSeconds is the Retry-After hint surfaced
// on the sentinel-error code path (errTriggerProjectQueuedQuotaExceeded,
// errTriggerProjectExecutingQuotaExceeded, errTriggerJobRateLimitExceeded).
// It is a static fallback — callers that want a precise back-off should
// inspect the structured rate-limit metadata on the response detail
// string ("retry_after_seconds=<n>"), which is set by per-job and
// per-project limiters at the call site.
//
// 5s is long enough for callers to back off without piling on, short
// enough that legitimately throttled traffic recovers quickly when
// capacity frees up. Pre-existing huma.StatusError values (e.g. the
// daily-cost-budget 429 that resets at midnight) intentionally bypass
// this constant — see triggerLimitAPIError.
const triggerLimitFallbackRetryAfterSeconds = 5

func triggerLimitAPIError(err error, fallback string) error {
	var statusErr huma.StatusError
	if errors.As(err, &statusErr) {
		return err
	}
	switch {
	case errors.Is(err, errTriggerProjectQueuedQuotaExceeded):
		return newTriggerLimit429("project queued quota exceeded")
	case errors.Is(err, errTriggerProjectExecutingQuotaExceeded):
		return newTriggerLimit429("project executing quota exceeded")
	case errors.Is(err, errTriggerJobRateLimitExceeded):
		return newTriggerLimit429("job rate limit exceeded")
	case errors.Is(err, errTriggerAdmissionContended):
		return newTriggerLimit429("trigger admission busy")
	default:
		return huma.Error500InternalServerError(fallback)
	}
}

func newTriggerLimit429(msg string) error {
	retryAfter := strconv.Itoa(triggerLimitFallbackRetryAfterSeconds)
	return &typedAPIError{
		status: http.StatusTooManyRequests,
		apiError: APIError{
			Code:    ErrorCodeRateLimited,
			Message: msg,
			Details: []string{"retry_after_seconds=" + retryAfter},
		},
		headers: map[string]string{
			"Retry-After": retryAfter,
		},
	}
}
