ALTER TABLE jobs DROP COLUMN IF EXISTS singleton_preempt_higher_priority;
ALTER TABLE job_versions DROP COLUMN IF EXISTS singleton_preempt_higher_priority;
ALTER TABLE workflows DROP COLUMN IF EXISTS singleton_preempt_higher_priority;
ALTER TABLE workflow_versions DROP COLUMN IF EXISTS singleton_preempt_higher_priority;
