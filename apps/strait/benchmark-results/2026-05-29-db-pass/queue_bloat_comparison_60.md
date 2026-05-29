# queue_bloat_comparison_60

- Baseline: `legacy`
- Candidate: `batchlog`
- P99 latency delta: `-7.278667ms`
- Throughput delta: `6.25 runs/s`
- WAL bytes delta: `0`

## Counters

| Metric | Baseline | Candidate | Delta |
|---|---:|---:|---:|
| `enqueued` | 60 | 60 | +0 |
| `dequeued` | 60 | 60 | +0 |
| `completed` | 60 | 60 | +0 |
| `retry_redelivery` | 1 | 1 | +0 |
| `duplicate_claims` | 0 | 0 | +0 |
| `lost_claims` | 0 | 0 | +0 |
| `notify_count` | 33 | 35 | +2 |
| `wal_bytes` | 0 | 0 | +0 |

## Relation Deltas

| Relation | Dead tuples | Dead / 1k completed | Table bytes | Index bytes |
|---|---:|---:|---:|---:|
| `enqueue_outbox` | +0 | 0.00 | +0 | +0 |
| `enqueue_outbox_history` | +0 | 0.00 | +0 | +0 |
| `enqueue_outbox_history_default` | +0 | 0.00 | +0 | +0 |
| `enqueue_outbox_history_p2026_05` | +0 | 0.00 | +0 | +0 |
| `enqueue_outbox_history_p2026_06` | +0 | 0.00 | +0 | +0 |
| `enqueue_outbox_history_p2026_07` | +0 | 0.00 | +0 | +0 |
| `enqueue_outbox_history_p2026_08` | +0 | 0.00 | +0 | +0 |
| `event_triggers` | +0 | 0.00 | +0 | +0 |
| `job_active_counts` | +0 | 0.00 | +0 | +0 |
| `job_retries` | +0 | 0.00 | +0 | +0 |
| `job_runs` | +0 | 0.00 | +0 | +0 |
| `job_runs_default` | +0 | 0.00 | +0 | +0 |
| `job_runs_history` | +0 | 0.00 | +0 | +0 |
| `job_runs_p2026_05` | +0 | 0.00 | -16384 | -8192 |
| `job_runs_p2026_06` | +0 | 0.00 | +0 | +0 |
| `job_runs_p2026_07` | +0 | 0.00 | +0 | +0 |
| `job_runs_p2026_08` | +0 | 0.00 | +0 | +0 |
| `outbox_batches` | +0 | 0.00 | +0 | +0 |
| `outbox_claims` | +0 | 0.00 | +0 | +0 |
| `queue_batch_seal_state` | +0 | 0.00 | +0 | +0 |
| `queue_batch_ticks` | +0 | 0.00 | +0 | +0 |
| `queue_batches` | +0 | 0.00 | +0 | +0 |
| `queue_entries` | +0 | 0.00 | +114688 | +98304 |
| `workflow_progression_events` | +0 | 0.00 | +0 | +0 |
| `workflow_step_runs` | +0 | 0.00 | +0 | +0 |

## Improvement Hints

- `queue_entries` / `relation_bloat_delta`: dead tuples +0, index bytes +98304

## SQL Plans

### Baseline: legacy candidate selection

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

### Candidate: batchlog candidate selection

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

