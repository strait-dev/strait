# queue_bloat_comparison_1000

- Baseline: `legacy`
- Candidate: `batchlog`
- P99 latency delta: `-13.628875ms`
- Throughput delta: `-22.90 runs/s`
- WAL bytes delta: `6182205`

## Counters

| Metric | Baseline | Candidate | Delta |
|---|---:|---:|---:|
| `enqueued` | 1000 | 1000 | +0 |
| `dequeued` | 1000 | 1000 | +0 |
| `completed` | 1000 | 1000 | +0 |
| `retry_redelivery` | 1 | 1 | +0 |
| `duplicate_claims` | 0 | 0 | +0 |
| `lost_claims` | 0 | 0 | +0 |
| `notify_count` | 503 | 505 | +2 |
| `wal_bytes` | 11673268 | 17855473 | +6182205 |

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
| `job_active_counts` | +33 | 33.00 | +0 | +0 |
| `job_retries` | +0 | 0.00 | +0 | +0 |
| `job_runs` | +0 | 0.00 | +0 | +0 |
| `job_runs_default` | +0 | 0.00 | +0 | +0 |
| `job_runs_history` | +0 | 0.00 | +0 | +0 |
| `job_runs_p2026_05` | +2292 | 2292.00 | -335872 | -335872 |
| `job_runs_p2026_06` | +0 | 0.00 | +0 | +0 |
| `job_runs_p2026_07` | +0 | 0.00 | +0 | +0 |
| `job_runs_p2026_08` | +0 | 0.00 | +0 | +0 |
| `outbox_batches` | +0 | 0.00 | +0 | +0 |
| `outbox_claims` | +0 | 0.00 | +0 | +0 |
| `queue_batch_seal_state` | +0 | 0.00 | +0 | +0 |
| `queue_batch_ticks` | +0 | 0.00 | +0 | +0 |
| `queue_batches` | +0 | 0.00 | +0 | +0 |
| `queue_entries` | +5060 | 5060.00 | +712704 | +409600 |
| `workflow_progression_events` | +0 | 0.00 | +0 | +0 |
| `workflow_step_runs` | +0 | 0.00 | +0 | +0 |

## Improvement Hints

- `throughput` / `runs_per_second_delta`: candidate throughput is 22.90 runs/s lower than baseline
- `wal` / `wal_bytes_delta`: candidate wrote 6182205 more WAL bytes than baseline
- `job_active_counts` / `relation_bloat_delta`: dead tuples +33, index bytes +0
- `job_runs_p2026_05` / `relation_bloat_delta`: dead tuples +2292, index bytes -335872
- `queue_entries` / `relation_bloat_delta`: dead tuples +5060, index bytes +409600

## SQL Plans

### Baseline: legacy candidate selection

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

### Candidate: batchlog candidate selection

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

