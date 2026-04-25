-- Phase 4: index to support the scheduler.PriorityPromoter goroutine. The
-- promoter scans queued rows older than a threshold and bumps their
-- priority so starvation is handled outside of the dequeue hot path.
--
-- The index is partial (status='queued' AND priority < 1000) so it stays
-- small even under backlog: rows that are already at the ceiling are not
-- included, and rows in other statuses are not candidates. Ordering by
-- created_at lets the promoter walk the oldest rows first.

-- safety-ok: initial deploy on empty partitions, no concurrent readers
CREATE INDEX IF NOT EXISTS idx_runs_promoter
    ON job_runs (created_at)
    WHERE status = 'queued' AND priority < 1000;
