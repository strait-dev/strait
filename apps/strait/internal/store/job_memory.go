package store

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

const (
	jobMemoryQuotaKindPerKey = "per_key"
	jobMemoryQuotaKindPerJob = "per_job"
)

type JobMemoryQuotaError struct {
	Kind string
	Max  int
}

func (e *JobMemoryQuotaError) Error() string {
	switch e.Kind {
	case jobMemoryQuotaKindPerKey:
		return fmt.Sprintf("%s: %d", ErrJobMemoryPerKeyLimitExceeded, e.Max)
	case jobMemoryQuotaKindPerJob:
		return fmt.Sprintf("%s: %d", ErrJobMemoryPerJobLimitExceeded, e.Max)
	default:
		return "job memory quota exceeded"
	}
}

func (e *JobMemoryQuotaError) Is(target error) bool {
	switch target {
	case ErrJobMemoryPerKeyLimitExceeded:
		return e.Kind == jobMemoryQuotaKindPerKey
	case ErrJobMemoryPerJobLimitExceeded:
		return e.Kind == jobMemoryQuotaKindPerJob
	default:
		return false
	}
}

func (q *Queries) UpsertJobMemory(ctx context.Context, mem *domain.JobMemory) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertJobMemory")
	defer span.End()

	query := `
		INSERT INTO job_memory (job_id, project_id, memory_key, value, size_bytes, ttl_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (job_id, memory_key)
		DO UPDATE SET value = EXCLUDED.value, size_bytes = EXCLUDED.size_bytes, ttl_expires_at = EXCLUDED.ttl_expires_at, updated_at = NOW()
		RETURNING id, created_at, updated_at`

	err := q.db.QueryRow(ctx, query,
		mem.JobID, mem.ProjectID, mem.MemoryKey, mem.Value, mem.SizeBytes, mem.TTLExpiresAt,
	).Scan(&mem.ID, &mem.CreatedAt, &mem.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert job memory: %w", err)
	}
	return nil
}

func (q *Queries) UpsertJobMemoryWithQuota(ctx context.Context, mem *domain.JobMemory, maxPerKey, maxPerJob int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertJobMemoryWithQuota")
	defer span.End()

	if maxPerKey > 0 && mem.SizeBytes > maxPerKey {
		return &JobMemoryQuotaError{Kind: jobMemoryQuotaKindPerKey, Max: maxPerKey}
	}

	_, ok := q.db.(TxBeginner)
	if !ok {
		return fmt.Errorf("upsert job memory with quota: db does not support transactions")
	}

	return q.withTx(ctx, func(txQ *Queries) error {
		if err := txQ.AdvisoryXactLock(ctx, hashString(mem.JobID)); err != nil {
			return fmt.Errorf("advisory lock: %w", err)
		}

		existing, err := txQ.GetJobMemory(ctx, mem.JobID, mem.MemoryKey)
		if err != nil {
			return fmt.Errorf("get existing job memory: %w", err)
		}

		currentTotal, err := txQ.SumJobMemorySizeBytes(ctx, mem.JobID)
		if err != nil {
			return fmt.Errorf("sum job memory size: %w", err)
		}

		existingSize := 0
		if existing != nil {
			existingSize = existing.SizeBytes
		}
		if maxPerJob > 0 && currentTotal-existingSize+mem.SizeBytes > maxPerJob {
			return &JobMemoryQuotaError{Kind: jobMemoryQuotaKindPerJob, Max: maxPerJob}
		}

		if err := txQ.UpsertJobMemory(ctx, mem); err != nil {
			return fmt.Errorf("upsert job memory: %w", err)
		}
		return nil
	})
}

func (q *Queries) UpsertJobMemoryWithQuotaForActiveRun(ctx context.Context, runID string, mem *domain.JobMemory, maxPerKey, maxPerJob, attempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertJobMemoryWithQuotaForActiveRun")
	defer span.End()

	if maxPerKey > 0 && mem.SizeBytes > maxPerKey {
		return &JobMemoryQuotaError{Kind: jobMemoryQuotaKindPerKey, Max: maxPerKey}
	}

	_, ok := q.db.(TxBeginner)
	if !ok {
		return fmt.Errorf("upsert active job memory with quota: db does not support transactions")
	}

	return q.withTx(ctx, func(txQ *Queries) error {
		var active bool
		if err := txQ.db.QueryRow(ctx, `
			SELECT TRUE
			FROM job_runs
			WHERE id = $1
			  AND job_id = $2
			  AND attempt = $3
			  AND status IN ('executing', 'waiting')
			FOR UPDATE`, runID, mem.JobID, attempt).Scan(&active); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, runID, attempt)
			}
			return fmt.Errorf("verify active run for job memory: %w", err)
		}

		if err := txQ.AdvisoryXactLock(ctx, hashString(mem.JobID)); err != nil {
			return fmt.Errorf("advisory lock: %w", err)
		}

		existing, err := txQ.GetJobMemory(ctx, mem.JobID, mem.MemoryKey)
		if err != nil {
			return fmt.Errorf("get existing job memory: %w", err)
		}

		currentTotal, err := txQ.SumJobMemorySizeBytes(ctx, mem.JobID)
		if err != nil {
			return fmt.Errorf("sum job memory size: %w", err)
		}

		existingSize := 0
		if existing != nil {
			existingSize = existing.SizeBytes
		}
		if maxPerJob > 0 && currentTotal-existingSize+mem.SizeBytes > maxPerJob {
			return &JobMemoryQuotaError{Kind: jobMemoryQuotaKindPerJob, Max: maxPerJob}
		}

		if err := txQ.UpsertJobMemory(ctx, mem); err != nil {
			return fmt.Errorf("upsert job memory: %w", err)
		}
		return nil
	})
}

func (q *Queries) GetJobMemory(ctx context.Context, jobID, key string) (*domain.JobMemory, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobMemory")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, memory_key, value, size_bytes, ttl_expires_at, created_at, updated_at
		FROM job_memory
		WHERE job_id = $1 AND memory_key = $2
		  AND (ttl_expires_at IS NULL OR ttl_expires_at > NOW())`

	var mem domain.JobMemory
	err := q.db.QueryRow(ctx, query, jobID, key).Scan(
		&mem.ID, &mem.JobID, &mem.ProjectID, &mem.MemoryKey, &mem.Value,
		&mem.SizeBytes, &mem.TTLExpiresAt, &mem.CreatedAt, &mem.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get job memory: %w", err)
	}
	return &mem, nil
}

// GetJobMemoryForActiveRun returns a memory row only if the supplied run is
// active for the given attempt. The job-memory table is keyed on job_id, so
// we additionally cross-check that the run identifies the same job.
func (q *Queries) GetJobMemoryForActiveRun(ctx context.Context, runID, jobID, key string, attempt int) (*domain.JobMemory, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobMemoryForActiveRun")
	defer span.End()

	var active bool
	if err := q.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM job_runs WHERE id = $1 AND attempt = $2 AND job_id = $3 AND status IN ('executing', 'waiting'))`, runID, attempt, jobID).Scan(&active); err != nil {
		return nil, fmt.Errorf("check run active for attempt: %w", err)
	}
	if !active {
		return nil, fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, runID, attempt)
	}
	return q.GetJobMemory(ctx, jobID, key)
}

// ListJobMemoryForActiveRun mirrors ListJobMemory but rejects callers whose
// run/attempt is no longer active.
func (q *Queries) ListJobMemoryForActiveRun(ctx context.Context, runID, jobID string, attempt int) ([]domain.JobMemory, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobMemoryForActiveRun")
	defer span.End()

	var active bool
	if err := q.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM job_runs WHERE id = $1 AND attempt = $2 AND job_id = $3 AND status IN ('executing', 'waiting'))`, runID, attempt, jobID).Scan(&active); err != nil {
		return nil, fmt.Errorf("check run active for attempt: %w", err)
	}
	if !active {
		return nil, fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, runID, attempt)
	}
	return q.ListJobMemory(ctx, jobID)
}

func (q *Queries) ListJobMemory(ctx context.Context, jobID string) ([]domain.JobMemory, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobMemory")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, memory_key, value, size_bytes, ttl_expires_at, created_at, updated_at
		FROM job_memory
		WHERE job_id = $1
		  AND (ttl_expires_at IS NULL OR ttl_expires_at > NOW())
		ORDER BY memory_key ASC
		LIMIT 10000`

	rows, err := q.db.Query(ctx, query, jobID)
	if err != nil {
		return nil, fmt.Errorf("list job memory: %w", err)
	}
	defer rows.Close()

	items := make([]domain.JobMemory, 0, 16)
	for rows.Next() {
		var mem domain.JobMemory
		if err := rows.Scan(
			&mem.ID, &mem.JobID, &mem.ProjectID, &mem.MemoryKey, &mem.Value,
			&mem.SizeBytes, &mem.TTLExpiresAt, &mem.CreatedAt, &mem.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("list job memory scan: %w", err)
		}
		items = append(items, mem)
	}
	return items, rows.Err()
}

func (q *Queries) DeleteJobMemory(ctx context.Context, jobID, key string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteJobMemory")
	defer span.End()

	_, err := q.db.Exec(ctx, `DELETE FROM job_memory WHERE job_id = $1 AND memory_key = $2`, jobID, key)
	if err != nil {
		return fmt.Errorf("delete job memory: %w", err)
	}
	return nil
}

func (q *Queries) DeleteJobMemoryForActiveRun(ctx context.Context, runID, jobID, key string, attempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteJobMemoryForActiveRun")
	defer span.End()

	var active bool
	query := `
		WITH active_run AS (
			SELECT id
			FROM job_runs
			WHERE id = $1
			  AND job_id = $2
			  AND attempt = $4
			  AND status IN ('executing', 'waiting')
			FOR UPDATE
		),
		deleted AS (
			DELETE FROM job_memory
			WHERE job_id = $2
			  AND memory_key = $3
			  AND EXISTS (SELECT 1 FROM active_run)
			RETURNING 1
		)
		SELECT EXISTS (SELECT 1 FROM active_run)`
	if err := q.db.QueryRow(ctx, query, runID, jobID, key, attempt).Scan(&active); err != nil {
		return fmt.Errorf("delete active job memory: %w", err)
	}
	if !active {
		return fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, runID, attempt)
	}
	return nil
}

func (q *Queries) SumJobMemorySizeBytes(ctx context.Context, jobID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SumJobMemorySizeBytes")
	defer span.End()

	var total int
	err := q.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(size_bytes), 0) FROM job_memory WHERE job_id = $1 AND (ttl_expires_at IS NULL OR ttl_expires_at > NOW())`,
		jobID,
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("sum job memory size: %w", err)
	}
	return total, nil
}

func (q *Queries) DeleteExpiredJobMemory(ctx context.Context) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteExpiredJobMemory")
	defer span.End()

	// Delete in batches of 10000 to avoid unbounded deletes that could lock
	// the table for extended periods under heavy write load.
	var totalDeleted int64
	for {
		tag, err := q.db.Exec(ctx, `
			DELETE FROM job_memory
			WHERE ctid IN (
				SELECT ctid FROM job_memory
				WHERE ttl_expires_at IS NOT NULL AND ttl_expires_at <= NOW()
				LIMIT 10000
			)`)
		if err != nil {
			return totalDeleted, fmt.Errorf("delete expired job memory: %w", err)
		}
		n := tag.RowsAffected()
		totalDeleted += n
		if n < 10000 {
			break
		}
	}
	return totalDeleted, nil
}
