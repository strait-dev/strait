-- Add compensation step reference for Saga pattern
ALTER TABLE workflow_steps ADD COLUMN compensate_step_ref TEXT;

-- Add cancel endpoint URL for HTTP jobs (graceful interruption)
ALTER TABLE jobs ADD COLUMN cancel_endpoint_url TEXT;

-- Add canceling status to track cleanup phase for runs
-- (The FSM transition is enforced in application code, not DB constraints)

-- Index for quick lookup of compensation steps
CREATE INDEX idx_workflow_steps_compensate ON workflow_steps (compensate_step_ref)
    WHERE compensate_step_ref IS NOT NULL;
