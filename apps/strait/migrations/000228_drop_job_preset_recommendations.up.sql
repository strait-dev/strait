-- Remove OOM-based preset recommendation table now that managed-container
-- execution has been replaced by orchestration-only mode.
DROP TABLE IF EXISTS job_preset_recommendations;
