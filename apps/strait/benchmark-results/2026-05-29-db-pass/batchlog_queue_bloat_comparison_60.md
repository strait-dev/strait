# batchlog_queue_bloat_comparison_60

- Engine: `batchlog`
- Duration: `848.286917ms`
- Enqueued: `60`
- Dequeued: `60`
- Completed: `60`
- Duplicate claims: `0`
- Lost claims: `0`
- Notifications observed: `35`
- WAL bytes: `0`

## Dequeue Latency

| Count | Min | P50 | P95 | P99 | Max |
|---:|---:|---:|---:|---:|---:|
| 6 | 1.944334ms | 2.351375ms | 3.973875ms | 3.973875ms | 3.973875ms |

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
| `job_active_counts` | 0 | 0 | 0.00 | 244 | 100.00 | 32768 | 16384 |
| `job_retries` | 0 | 0 | 0.00 | 0 | 0.00 | 24576 | 16384 |
| `job_runs` | 0 | 0 | 0.00 | 0 | 0.00 | 0 | 0 |
| `job_runs_default` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `job_runs_history` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `job_runs_p2026_05` | 0 | 0 | 0.00 | 184 | 0.00 | 450560 | 393216 |
| `job_runs_p2026_06` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `job_runs_p2026_07` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `job_runs_p2026_08` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `outbox_batches` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `outbox_claims` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `queue_batch_seal_state` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `queue_batch_ticks` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `queue_batches` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `queue_entries` | 0 | 0 | 0.00 | 243 | 0.00 | 335872 | 262144 |
| `workflow_progression_events` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `workflow_step_runs` | 0 | 0 | 0.00 | 0 | 0.00 | 49152 | 40960 |

## SQL Plans

### batchlog candidate selection

```text
Limit  (cost=0.73..40.95 rows=1 width=52) (actual time=0.047..0.213 rows=50.00 loops=1)
  Buffers: shared hit=331
  ->  Nested Loop Left Join  (cost=0.73..40.95 rows=1 width=52) (actual time=0.047..0.207 rows=50.00 loops=1)
        Filter: ((qe.job_max_concurrency_per_key IS NULL) OR (qe.concurrency_key = ''::text) OR ((COALESCE(jac_key.count, 0) + COALESCE(((count(*))::integer), 0)) < qe.job_max_concurrency_per_key))
        Buffers: shared hit=331
        ->  Nested Loop Left Join  (cost=0.58..32.77 rows=1 width=124) (actual time=0.035..0.145 rows=50.00 loops=1)
              Join Filter: ((leased_1.job_id = qe.job_id) AND (leased_1.concurrency_key = qe.concurrency_key))
              Buffers: shared hit=231
              ->  Nested Loop Left Join  (cost=0.44..24.56 rows=1 width=120) (actual time=0.027..0.113 rows=50.00 loops=1)
                    Filter: ((qe.job_max_concurrency IS NULL) OR ((COALESCE(jac_job.count, 0) + COALESCE(((count(*))::integer), 0)) < qe.job_max_concurrency))
                    Buffers: shared hit=181
                    ->  Nested Loop Left Join  (cost=0.29..16.38 rows=1 width=128) (actual time=0.019..0.075 rows=50.00 loops=1)
                          Join Filter: (leased.job_id = qe.job_id)
                          Buffers: shared hit=81
                          ->  Index Scan using idx_queue_entries_claimable on queue_entries qe  (cost=0.14..8.18 rows=1 width=124) (actual time=0.012..0.044 rows=50.00 loops=1)
                                Index Cond: (batch_id IS NOT NULL)
                                Filter: (COALESCE(job_enabled, true) AND (NOT COALESCE(job_paused, false)) AND (run_status = 'queued'::text) AND (available_at <= now()) AND ((scheduled_at IS NULL) OR (scheduled_at <= now())) AND ((next_retry_at IS NULL) OR (next_retry_at <= now())))
                                Index Searches: 1
                                Buffers: shared hit=31
                          ->  GroupAggregate  (cost=0.14..8.17 rows=1 width=36) (actual time=0.000..0.000 rows=0.00 loops=50)
                                Group Key: leased.job_id
                                Buffers: shared hit=50
                                ->  Index Only Scan using idx_queue_entries_leased_key_denorm on queue_entries leased  (cost=0.14..8.16 rows=1 width=32) (actual time=0.000..0.000 rows=0.00 loops=50)
                                      Heap Fetches: 0
                                      Index Searches: 50
                                      Buffers: shared hit=50
                    ->  Index Scan using job_active_counts_pkey on job_active_counts jac_job  (cost=0.15..8.17 rows=1 width=36) (actual time=0.000..0.000 rows=0.00 loops=50)
                          Index Cond: ((job_id = qe.job_id) AND (concurrency_key = ''::text))
                          Index Searches: 50
                          Buffers: shared hit=100
              ->  GroupAggregate  (cost=0.14..8.18 rows=1 width=68) (actual time=0.000..0.000 rows=0.00 loops=50)
                    Group Key: leased_1.job_id, leased_1.concurrency_key
                    Buffers: shared hit=50
                    ->  Index Only Scan using idx_queue_entries_leased_key_denorm on queue_entries leased_1  (cost=0.14..8.16 rows=1 width=64) (actual time=0.000..0.000 rows=0.00 loops=50)
                          Filter: (concurrency_key <> ''::text)
                          Heap Fetches: 0
                          Index Searches: 50
                          Buffers: shared hit=50
        ->  Index Scan using job_active_counts_pkey on job_active_counts jac_key  (cost=0.15..8.17 rows=1 width=68) (actual time=0.001..0.001 rows=0.00 loops=50)
              Index Cond: ((job_id = qe.job_id) AND (concurrency_key = qe.concurrency_key))
              Index Searches: 50
              Buffers: shared hit=100
Planning:
  Buffers: shared hit=46 read=1
Planning Time: 0.776 ms
Execution Time: 0.305 ms
```

