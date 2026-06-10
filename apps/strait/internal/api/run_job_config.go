package api

import "strait/internal/domain"

func stampRunJobConfig(run *domain.JobRun, job *domain.Job) {
	if run == nil || job == nil {
		return
	}
	enabled := job.Enabled
	paused := job.Paused
	run.JobEnabled = &enabled
	run.JobPaused = &paused
	if job.MaxConcurrency > 0 {
		maxConcurrency := job.MaxConcurrency
		run.JobMaxConcurrency = &maxConcurrency
	}
	if job.MaxConcurrencyPerKey > 0 {
		maxConcurrencyPerKey := job.MaxConcurrencyPerKey
		run.JobMaxConcurrencyPerKey = &maxConcurrencyPerKey
	}
}
