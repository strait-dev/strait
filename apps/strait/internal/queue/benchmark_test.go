package queue

import (
	"fmt"
	"testing"

	"strait/internal/domain"
)

func BenchmarkBuildDequeueQuery(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = fmt.Sprintf(`
		WITH %s
		UPDATE job_runs
		SET status = '%s', started_at = NOW()
		WHERE id = (
			SELECT jr.id
			FROM job_runs jr
			JOIN jobs j ON j.id = jr.job_id
			%s
			WHERE jr.status = '%s'
			  AND j.enabled = true
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  %s
			ORDER BY jr.priority DESC, jr.created_at ASC
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT 1
		)
		RETURNING %s`, concurrencyCTEs, domain.StatusDequeued, concurrencyJoins, domain.StatusQueued, concurrencyWhere, dequeueColumns)
	}
}

func BenchmarkDequeueOrderByClause(b *testing.B) {
	b.Run("without_priority_aging", func(b *testing.B) {
		q := NewPostgresQueue(nil, WithPriorityAging(false))

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = q.dequeueOrderByClause()
		}
	})

	b.Run("with_priority_aging", func(b *testing.B) {
		q := NewPostgresQueue(nil, WithPriorityAging(true))

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = q.dequeueOrderByClause()
		}
	})
}
