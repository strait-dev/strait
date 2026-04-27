-- Increase autovacuum_vacuum_cost_limit on job_runs so vacuum does 5x
-- more work per cycle before napping. The system-wide default of 200
-- is too conservative for a high-churn queue table: under sustained
-- load, dead tuples accumulate faster than the default budget allows
-- vacuum to reclaim.
--
-- 1000 lets vacuum process ~5x more pages per cycle, which is critical
-- when combined with the aggressive cost_delay=2 (set in 000063).

ALTER TABLE job_runs SET (
  autovacuum_vacuum_cost_limit = 1000
);
