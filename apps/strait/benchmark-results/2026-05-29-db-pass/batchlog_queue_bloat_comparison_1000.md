# batchlog_queue_bloat_comparison_1000

- Engine: `batchlog`
- Duration: `5.134909709s`
- Enqueued: `1000`
- Dequeued: `1000`
- Completed: `1000`
- Duplicate claims: `0`
- Lost claims: `0`
- Notifications observed: `505`
- WAL bytes: `17855473`

## Dequeue Latency

| Count | Min | P50 | P95 | P99 | Max |
|---:|---:|---:|---:|---:|---:|
| 20 | 2.494583ms | 3.492709ms | 9.144209ms | 10.1485ms | 10.1485ms |

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
| `job_active_counts` | 1 | 57 | 98.28 | 5831 | 100.00 | 32768 | 16384 |
| `job_retries` | 0 | 0 | 0.00 | 0 | 0.00 | 24576 | 16384 |
| `job_runs` | 0 | 0 | 0.00 | 0 | 0.00 | 0 | 0 |
| `job_runs_default` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `job_runs_history` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `job_runs_p2026_05` | 2001 | 4469 | 69.07 | 4773 | 0.00 | 2220032 | 1753088 |
| `job_runs_p2026_06` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `job_runs_p2026_07` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `job_runs_p2026_08` | 0 | 0 | 0.00 | 0 | 0.00 | 237568 | 229376 |
| `outbox_batches` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `outbox_claims` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `queue_batch_seal_state` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `queue_batch_ticks` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `queue_batches` | 0 | 0 | 0.00 | 0 | 0.00 | 8192 | 8192 |
| `queue_entries` | 2001 | 7951 | 79.89 | 8498 | 0.00 | 1892352 | 1220608 |
| `workflow_progression_events` | 0 | 0 | 0.00 | 0 | 0.00 | 40960 | 32768 |
| `workflow_step_runs` | 0 | 0 | 0.00 | 0 | 0.00 | 49152 | 40960 |

## SQL Plans

### batchlog candidate selection

```text
Limit  (cost=0.86..41.08 rows=1 width=52) (actual time=0.057..0.215 rows=50.00 loops=1)
  Buffers: shared hit=351
  ->  Nested Loop Left Join  (cost=0.86..41.08 rows=1 width=52) (actual time=0.056..0.209 rows=50.00 loops=1)
        Filter: ((qe.job_max_concurrency_per_key IS NULL) OR (qe.concurrency_key = ''::text) OR ((COALESCE(jac_key.count, 0) + COALESCE(((count(*))::integer), 0)) < qe.job_max_concurrency_per_key))
        Buffers: shared hit=351
        ->  Nested Loop Left Join  (cost=0.71..32.90 rows=1 width=124) (actual time=0.049..0.172 rows=50.00 loops=1)
              Join Filter: ((leased_1.job_id = qe.job_id) AND (leased_1.concurrency_key = qe.concurrency_key))
              Buffers: shared hit=251
              ->  Nested Loop Left Join  (cost=0.57..24.69 rows=1 width=120) (actual time=0.043..0.141 rows=50.00 loops=1)
                    Filter: ((qe.job_max_concurrency IS NULL) OR ((COALESCE(jac_job.count, 0) + COALESCE(((count(*))::integer), 0)) < qe.job_max_concurrency))
                    Buffers: shared hit=201
                    ->  Nested Loop Left Join  (cost=0.42..16.51 rows=1 width=128) (actual time=0.033..0.100 rows=50.00 loops=1)
                          Join Filter: (leased.job_id = qe.job_id)
                          Buffers: shared hit=101
                          ->  Index Scan using idx_queue_entries_claimable_denorm on queue_entries qe  (cost=0.28..8.31 rows=1 width=124) (actual time=0.018..0.061 rows=50.00 loops=1)
                                Index Cond: (batch_id IS NOT NULL)
                                Filter: (COALESCE(job_enabled, true) AND (NOT COALESCE(job_paused, false)) AND (available_at <= now()) AND ((scheduled_at IS NULL) OR (scheduled_at <= now())) AND ((next_retry_at IS NULL) OR (next_retry_at <= now())))
                                Index Searches: 1
                                Buffers: shared hit=51
                          ->  GroupAggregate  (cost=0.14..8.17 rows=1 width=36) (actual time=0.001..0.001 rows=0.00 loops=50)
                                Group Key: leased.job_id
                                Buffers: shared hit=50
                                ->  Index Only Scan using idx_queue_entries_leased_key_denorm on queue_entries leased  (cost=0.14..8.16 rows=1 width=32) (actual time=0.000..0.000 rows=0.00 loops=50)
                                      Heap Fetches: 0
                                      Index Searches: 50
                                      Buffers: shared hit=50
                    ->  Index Scan using job_active_counts_pkey on job_active_counts jac_job  (cost=0.15..8.17 rows=1 width=36) (actual time=0.001..0.001 rows=0.00 loops=50)
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
        ->  Index Scan using job_active_counts_pkey on job_active_counts jac_key  (cost=0.15..8.17 rows=1 width=68) (actual time=0.000..0.000 rows=0.00 loops=50)
              Index Cond: ((job_id = qe.job_id) AND (concurrency_key = qe.concurrency_key))
              Index Searches: 50
              Buffers: shared hit=100
Planning:
  Buffers: shared hit=32 read=1
Planning Time: 1.300 ms
Execution Time: 0.302 ms
```

