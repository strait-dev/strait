-- Add execution mode to jobs: "http" (default) or "sandbox"
ALTER TABLE jobs ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'http';

-- Add sandbox code field for sandbox-type jobs
ALTER TABLE jobs ADD COLUMN sandbox_code TEXT;
ALTER TABLE jobs ADD COLUMN sandbox_language TEXT;

-- Add constraints
ALTER TABLE jobs ADD CONSTRAINT chk_execution_mode
    CHECK (execution_mode IN ('http', 'sandbox'));

ALTER TABLE jobs ADD CONSTRAINT chk_sandbox_fields
    CHECK (
        (execution_mode = 'http') OR
        (execution_mode = 'sandbox' AND sandbox_code IS NOT NULL AND sandbox_language IS NOT NULL)
    );

-- Index for filtering by execution mode
CREATE INDEX idx_jobs_execution_mode ON jobs (execution_mode);
