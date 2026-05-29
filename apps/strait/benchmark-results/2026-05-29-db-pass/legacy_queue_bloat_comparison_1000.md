# legacy_queue_bloat_comparison_1000

- Engine: `legacy`
- Duration: `4.594575625s`
- Enqueued: `1000`
- Dequeued: `1000`
- Completed: `1000`
- Duplicate claims: `0`
- Lost claims: `0`
- Notifications observed: `503`
- WAL bytes: `11673268`

## Dequeue Latency

| Count | Min | P50 | P95 | P99 | Max |
|---:|---:|---:|---:|---:|---:|
| 20 | 10.428458ms | 12.094166ms | 17.047084ms | 23.777375ms | 23.777375ms |

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
| `job_active_counts` | 1 | 24 | 96.00 | 3253 | 100.00 | 32768 | 16384 |
| `job_retries` | 0 | 0 | 0.00 | 0 | 0.00 | 24576 | 16384 |
| `job_runs` | 0 | 0 | 0.00 | 0 | 0.00 | 0 | 0 |
| `job_runs_default` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `job_runs_history` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `job_runs_p2026_05` | 1000 | 2177 | 68.52 | 2481 | 0.00 | 2555904 | 2088960 |
| `job_runs_p2026_06` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `job_runs_p2026_07` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `job_runs_p2026_08` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `outbox_batches` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `outbox_claims` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `queue_batch_seal_state` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `queue_batch_ticks` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `queue_batches` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `queue_entries` | 1000 | 2891 | 74.30 | 3438 | 0.00 | 1179648 | 811008 |
| `workflow_progression_events` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `workflow_step_runs` | 0 | 0 | 0.00 | 0 | 0.00 | 49152 | 40960 |

## SQL Plans

### legacy candidate selection

```text
Limit  (cost=67.82..67.82 rows=1 width=44) (actual time=1.783..1.793 rows=50.00 loops=1)
  Buffers: shared hit=2052
  ->  Sort  (cost=67.82..67.82 rows=1 width=44) (actual time=1.782..1.787 rows=50.00 loops=1)
        Sort Key: jr.priority DESC, jr.created_at
        Sort Method: top-N heapsort  Memory: 36kB
        Buffers: shared hit=2052
        ->  Nested Loop Left Join  (cost=12.63..67.81 rows=1 width=44) (actual time=0.085..1.611 rows=1000.00 loops=1)
              Filter: ((jr.job_max_concurrency_per_key IS NULL) OR (jr.concurrency_key IS NULL) OR (jr.concurrency_key = ''::text) OR (COALESCE(jac_key.count, 0) < jr.job_max_concurrency_per_key))
              Buffers: shared hit=2052
              ->  Nested Loop Left Join  (cost=12.48..59.44 rows=2 width=112) (actual time=0.066..0.841 rows=1000.00 loops=1)
                    Join Filter: (jac_job.job_id = jr.job_id)
                    Filter: ((jr.job_max_concurrency IS NULL) OR (COALESCE(jac_job.count, 0) < jr.job_max_concurrency))
                    Buffers: shared hit=52
                    ->  Append  (cost=4.28..43.86 rows=5 width=116) (actual time=0.052..0.598 rows=1000.00 loops=1)
                          Buffers: shared hit=51
                          ->  Bitmap Heap Scan on job_runs_p2026_05 jr_1  (cost=4.28..11.16 rows=1 width=116) (actual time=0.051..0.409 rows=1000.00 loops=1)
                                Recheck Cond: (status = 'queued'::text)
                                Filter: (COALESCE(job_enabled, true) AND (NOT COALESCE(job_paused, false)) AND ((scheduled_at IS NULL) OR (scheduled_at <= now())) AND ((next_retry_at IS NULL) OR (next_retry_at <= now())))
                                Heap Blocks: exact=42
                                Buffers: shared hit=47
                                ->  Bitmap Index Scan on job_runs_p2026_05_priority_created_at_idx  (cost=0.00..4.28 rows=2 width=0) (actual time=0.030..0.030 rows=1000.00 loops=1)
                                      Index Searches: 1
                                      Buffers: shared hit=5
                          ->  Index Scan using job_runs_p2026_06_priority_created_at_idx on job_runs_p2026_06 jr_2  (cost=0.14..8.17 rows=1 width=116) (actual time=0.036..0.036 rows=0.00 loops=1)
                                Filter: (COALESCE(job_enabled, true) AND (NOT COALESCE(job_paused, false)) AND ((scheduled_at IS NULL) OR (scheduled_at <= now())) AND ((next_retry_at IS NULL) OR (next_retry_at <= now())))
                                Index Searches: 1
                                Buffers: shared hit=1
                          ->  Index Scan using job_runs_p2026_07_priority_created_at_idx on job_runs_p2026_07 jr_3  (cost=0.14..8.17 rows=1 width=116) (actual time=0.007..0.007 rows=0.00 loops=1)
                                Filter: (COALESCE(job_enabled, true) AND (NOT COALESCE(job_paused, false)) AND ((scheduled_at IS NULL) OR (scheduled_at <= now())) AND ((next_retry_at IS NULL) OR (next_retry_at <= now())))
                                Index Searches: 1
                                Buffers: shared hit=1
                          ->  Index Scan using job_runs_p2026_08_priority_created_at_idx on job_runs_p2026_08 jr_4  (cost=0.14..8.17 rows=1 width=116) (actual time=0.007..0.007 rows=0.00 loops=1)
                                Filter: (COALESCE(job_enabled, true) AND (NOT COALESCE(job_paused, false)) AND ((scheduled_at IS NULL) OR (scheduled_at <= now())) AND ((next_retry_at IS NULL) OR (next_retry_at <= now())))
                                Index Searches: 1
                                Buffers: shared hit=1
                          ->  Index Scan using job_runs_default_priority_created_at_idx on job_runs_default jr_5  (cost=0.14..8.17 rows=1 width=116) (actual time=0.012..0.012 rows=0.00 loops=1)
                                Filter: (COALESCE(job_enabled, true) AND (NOT COALESCE(job_paused, false)) AND ((scheduled_at IS NULL) OR (scheduled_at <= now())) AND ((next_retry_at IS NULL) OR (next_retry_at <= now())))
                                Index Searches: 1
                                Buffers: shared hit=1
                    ->  Materialize  (cost=8.20..15.32 rows=3 width=36) (actual time=0.000..0.000 rows=0.00 loops=1000)
                          Storage: Memory  Maximum Storage: 17kB
                          Buffers: shared hit=1
                          ->  Bitmap Heap Scan on job_active_counts jac_job  (cost=8.20..15.31 rows=3 width=36) (actual time=0.008..0.009 rows=0.00 loops=1)
                                Recheck Cond: (concurrency_key = ''::text)
                                Buffers: shared hit=1
                                ->  Bitmap Index Scan on job_active_counts_pkey  (cost=0.00..8.20 rows=3 width=0) (actual time=0.005..0.005 rows=0.00 loops=1)
                                      Index Cond: (concurrency_key = ''::text)
                                      Index Searches: 1
                                      Buffers: shared hit=1
              ->  Index Scan using job_active_counts_pkey on job_active_counts jac_key  (cost=0.15..4.17 rows=1 width=68) (actual time=0.000..0.000 rows=0.00 loops=1000)
                    Index Cond: ((job_id = jr.job_id) AND (concurrency_key = COALESCE(jr.concurrency_key, ''::text)))
                    Index Searches: 1000
                    Buffers: shared hit=2000
Planning:
  Buffers: shared hit=119 read=1
Planning Time: 2.270 ms
Execution Time: 1.936 ms
```

