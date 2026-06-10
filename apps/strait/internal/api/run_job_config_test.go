package api

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestStampRunJobConfigCopiesJobValues(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{}
	job := &domain.Job{
		Enabled:              true,
		Paused:               false,
		MaxConcurrency:       12,
		MaxConcurrencyPerKey: 3,
	}

	stampRunJobConfig(run, job)

	require.NotNil(t, run.JobEnabled)
	require.NotNil(t, run.JobPaused)
	require.NotNil(t, run.JobMaxConcurrency)
	require.NotNil(t, run.JobMaxConcurrencyPerKey)
	require.True(t, *run.JobEnabled)
	require.False(t, *run.JobPaused)
	require.Equal(t, 12, *run.JobMaxConcurrency)
	require.Equal(t, 3, *run.JobMaxConcurrencyPerKey)

	job.Enabled = false
	job.Paused = true
	job.MaxConcurrency = 1
	job.MaxConcurrencyPerKey = 1

	require.True(t, *run.JobEnabled)
	require.False(t, *run.JobPaused)
	require.Equal(t, 12, *run.JobMaxConcurrency)
	require.Equal(t, 3, *run.JobMaxConcurrencyPerKey)
}

func TestStampRunJobConfigLeavesUnlimitedConcurrencyNil(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{}
	job := &domain.Job{
		Enabled: true,
	}

	stampRunJobConfig(run, job)

	require.NotNil(t, run.JobEnabled)
	require.NotNil(t, run.JobPaused)
	require.True(t, *run.JobEnabled)
	require.False(t, *run.JobPaused)
	require.Nil(t, run.JobMaxConcurrency)
	require.Nil(t, run.JobMaxConcurrencyPerKey)
}
