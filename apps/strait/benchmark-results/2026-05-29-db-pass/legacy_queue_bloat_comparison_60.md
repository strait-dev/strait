# legacy_queue_bloat_comparison_60

- Engine: `legacy`
- Duration: `930.454667ms`
- Enqueued: `60`
- Dequeued: `60`
- Completed: `60`
- Duplicate claims: `0`
- Lost claims: `0`
- Notifications observed: `33`
- WAL bytes: `0`

## Dequeue Latency

| Count | Min | P50 | P95 | P99 | Max |
|---:|---:|---:|---:|---:|---:|
| 6 | 6.085708ms | 9.465583ms | 11.252542ms | 11.252542ms | 11.252542ms |

## Relations

| Relation | Live | Dead | Dead % | Updates | HOT % | Table bytes | Index bytes |
|---|---:|---:|---:|---:|---:|---:|---:|
| `enqueue_outbox` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `enqueue_outbox_history` | 0 | 0 | 0.00 | 0 | 0.00 | 0 | 0 |
| `enqueue_outbox_history_default` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `enqueue_outbox_history_p2026_05` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `enqueue_outbox_history_p2026_06` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `enqueue_outbox_history_p2026_07` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `enqueue_outbox_history_p2026_08` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `event_triggers` | 0 | 0 | 0.00 | 0 | 0.00 | 90112 | 81920 |
| `job_active_counts` | 0 | 0 | 0.00 | 0 | 0.00 | 32768 | 16384 |
| `job_retries` | 0 | 0 | 0.00 | 0 | 0.00 | 24576 | 16384 |
| `job_runs` | 0 | 0 | 0.00 | 0 | 0.00 | 0 | 0 |
| `job_runs_default` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `job_runs_history` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `job_runs_p2026_05` | 0 | 0 | 0.00 | 0 | 0.00 | 466944 | 401408 |
| `job_runs_p2026_06` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `job_runs_p2026_07` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `job_runs_p2026_08` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `outbox_batches` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `outbox_claims` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `queue_batch_seal_state` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `queue_batch_ticks` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `queue_batches` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `queue_entries` | 0 | 0 | 0.00 | 0 | 0.00 | 221184 | 163840 |
| `workflow_progression_events` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `workflow_step_runs` | 0 | 0 | 0.00 | 0 | 0.00 | 49152 | 40960 |

## SQL Plans

### legacy candidate selection

```text
Limit  (cost=9.12..64.93 rows=1 width=44) (actual time=0.068..0.142 rows=50.00 loops=1)
  Buffers: shared hit=133
  ->  Nested Loop Left Join  (cost=9.12..64.93 rows=1 width=44) (actual time=0.067..0.136 rows=50.00 loops=1)
        Filter: ((jr.job_max_concurrency_per_key IS NULL) OR (jr.concurrency_key IS NULL) OR (jr.concurrency_key = ''::text) OR (COALESCE(jac_key.count, 0) < jr.job_max_concurrency_per_key))
        Buffers: shared hit=133
        ->  Nested Loop Left Join  (cost=8.97..56.56 rows=2 width=112) (actual time=0.060..0.098 rows=50.00 loops=1)
              Join Filter: (jac_job.job_id = jr.job_id)
              Filter: ((jr.job_max_concurrency IS NULL) OR (COALESCE(jac_job.count, 0) < jr.job_max_concurrency))
              Buffers: shared hit=33
              ->  Merge Append  (cost=0.77..40.98 rows=5 width=116) (actual time=0.034..0.059 rows=50.00 loops=1)
                    Sort Key: jr.priority DESC, jr.created_at
                    Buffers: shared hit=30
                    ->  Index Scan using job_runs_p2026_05_priority_created_at_idx on job_runs_p2026_05 jr_1  (cost=0.14..8.17 rows=1 width=116) (actual time=0.012..0.032 rows=50.00 loops=1)
                          Filter: (COALESCE(job_enabled, true) AND (NOT COALESCE(job_paused, false)) AND ((scheduled_at IS NULL) OR (scheduled_at <= now())) AND ((next_retry_at IS NULL) OR (next_retry_at <= now())))
                          Index Searches: 1
                          Buffers: shared hit=26
                    ->  Index Scan using job_runs_p2026_06_priority_created_at_idx on job_runs_p2026_06 jr_2  (cost=0.14..8.17 rows=1 width=116) (actual time=0.005..0.005 rows=0.00 loops=1)
                          Filter: (COALESCE(job_enabled, true) AND (NOT COALESCE(job_paused, false)) AND ((scheduled_at IS NULL) OR (scheduled_at <= now())) AND ((next_retry_at IS NULL) OR (next_retry_at <= now())))
                          Index Searches: 1
                          Buffers: shared hit=1
                    ->  Index Scan using job_runs_p2026_07_priority_created_at_idx on job_runs_p2026_07 jr_3  (cost=0.14..8.17 rows=1 width=116) (actual time=0.005..0.005 rows=0.00 loops=1)
                          Filter: (COALESCE(job_enabled, true) AND (NOT COALESCE(job_paused, false)) AND ((scheduled_at IS NULL) OR (scheduled_at <= now())) AND ((next_retry_at IS NULL) OR (next_retry_at <= now())))
                          Index Searches: 1
                          Buffers: shared hit=1
                    ->  Index Scan using job_runs_p2026_08_priority_created_at_idx on job_runs_p2026_08 jr_4  (cost=0.14..8.17 rows=1 width=116) (actual time=0.005..0.005 rows=0.00 loops=1)
                          Filter: (COALESCE(job_enabled, true) AND (NOT COALESCE(job_paused, false)) AND ((scheduled_at IS NULL) OR (scheduled_at <= now())) AND ((next_retry_at IS NULL) OR (next_retry_at <= now())))
                          Index Searches: 1
                          Buffers: shared hit=1
                    ->  Index Scan using job_runs_default_priority_created_at_idx on job_runs_default jr_5  (cost=0.14..8.17 rows=1 width=116) (actual time=0.006..0.006 rows=0.00 loops=1)
                          Filter: (COALESCE(job_enabled, true) AND (NOT COALESCE(job_paused, false)) AND ((scheduled_at IS NULL) OR (scheduled_at <= now())) AND ((next_retry_at IS NULL) OR (next_retry_at <= now())))
                          Index Searches: 1
                          Buffers: shared hit=1
              ->  Materialize  (cost=8.20..15.32 rows=3 width=36) (actual time=0.001..0.001 rows=0.00 loops=50)
                    Storage: Memory  Maximum Storage: 17kB
                    Buffers: shared hit=3
                    ->  Bitmap Heap Scan on job_active_counts jac_job  (cost=8.20..15.31 rows=3 width=36) (actual time=0.024..0.024 rows=0.00 loops=1)
                          Recheck Cond: (concurrency_key = ''::text)
                          Buffers: shared hit=3
                          ->  Bitmap Index Scan on job_active_counts_pkey  (cost=0.00..8.20 rows=3 width=0) (actual time=0.019..0.020 rows=0.00 loops=1)
                                Index Cond: (concurrency_key = ''::text)
                                Index Searches: 1
                                Buffers: shared hit=3
        ->  Index Scan using job_active_counts_pkey on job_active_counts jac_key  (cost=0.15..4.17 rows=1 width=68) (actual time=0.000..0.000 rows=0.00 loops=50)
              Index Cond: ((job_id = jr.job_id) AND (concurrency_key = COALESCE(jr.concurrency_key, ''::text)))
              Index Searches: 50
              Buffers: shared hit=100
Planning:
  Buffers: shared hit=151 read=1
Planning Time: 1.196 ms
Execution Time: 0.246 ms
```

