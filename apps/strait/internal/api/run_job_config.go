package api

import "strait/internal/domain"

func stampRunJobConfig(run *domain.JobRun, job *domain.Job) {
	if run == nil || job == nil {
		return
	}
	enabled := job.Enabled
	paused := job.Paused
	maxConcurrency := job.MaxConcurrency
	maxConcurrencyPerKey := job.MaxConcurrencyPerKey
	run.JobEnabled = &enabled
	run.JobPaused = &paused
	run.JobMaxConcurrency = &maxConcurrency
	run.JobMaxConcurrencyPerKey = &maxConcurrencyPerKey
}
