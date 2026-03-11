package queue

import (
	"fmt"
	"testing"

	"strait/internal/domain"
)

const dequeueQueryTemplate = `
		UPDATE job_runs
		SET status = '%s', started_at = NOW()
		WHERE id = (
			SELECT jr.id
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
			WHERE jr.status = '%s'
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  AND (
				j.max_concurrency IS NULL OR (
					SELECT COUNT(*)
					FROM job_runs active
					WHERE active.job_id = jr.job_id
					  AND active.status IN ('dequeued', 'executing')
				) < j.max_concurrency
			  )
			ORDER BY jr.priority DESC, jr.created_at ASC
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, job_id, project_id, status, attempt, payload, result, metadata, error,
		          triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		          next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by`

func BenchmarkBuildDequeueQuery(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = fmt.Sprintf(dequeueQueryTemplate, domain.StatusDequeued, domain.StatusQueued)
	}
}

func BenchmarkDequeueOrderByClause(b *testing.B) {
	b.Run("without_priority_aging", func(b *testing.B) {
		q := NewPostgresQueue(nil, WithPriorityAging(false))

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_ = q.dequeueOrderByClause()
		}
	})

	b.Run("with_priority_aging", func(b *testing.B) {
		q := NewPostgresQueue(nil, WithPriorityAging(true))

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_ = q.dequeueOrderByClause()
		}
	})
}
