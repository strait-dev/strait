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

func BenchmarkDequeueKernelQueryAssembly(b *testing.B) {
	q := NewPostgresQueue(nil)

	specs := []struct {
		name string
		spec dequeueSpec
	}{
		{
			name: "standard",
			spec: dequeueSpec{
				spanName:      "bench_standard",
				candidatesSQL: "SELECT jr.id, jr.created_at FROM job_runs jr JOIN jobs j ON j.id = jr.job_id WHERE jr.status = 'queued'",
			},
		},
		{
			name: "skip_ctes",
			spec: dequeueSpec{
				spanName:            "bench_skip_ctes",
				candidatesSQL:       "SELECT jr.id, jr.created_at FROM job_runs jr WHERE jr.status = 'queued'",
				skipConcurrencyCTEs: true,
			},
		},
	}

	for _, s := range specs {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_ = q.buildDequeueQuery(10, s.spec)
			}
		})
	}
}

func (q *PostgresQueue) buildDequeueQuery(n int, spec dequeueSpec) string {
	orderBy := q.dequeueOrderByClause()
	ctes := ""
	joins := ""
	where := ""
	if !spec.skipConcurrencyCTEs {
		ctes = concurrencyCTEs
		joins = concurrencyJoins
		where = concurrencyWhere
	}
	return fmt.Sprintf(`
		WITH %s candidates AS (%s ORDER BY %s FOR UPDATE OF jr SKIP LOCKED LIMIT %d),
		claimed AS (UPDATE job_runs SET status = 'dequeued', started_at = NOW() WHERE id IN (SELECT id FROM candidates) RETURNING *)
		SELECT %s FROM claimed %s %s`,
		ctes, spec.candidatesSQL, orderBy, n, dequeueColumns, joins, where)
}
