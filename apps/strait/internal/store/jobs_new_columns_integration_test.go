//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestCreateJob_WithNewColumns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-job-new-cols")
	job.MaxConcurrencyPerKey = 5
	job.RateLimitKeys = []domain.RateLimitKey{{Name: "api", Max: 10, WindowSecs: 60}}
	job.DefaultRunMetadata = map[string]string{"env": "prod"}
	require.NoError(t, q.CreateJob(ctx,
		job))

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)

	assertJobEqual(t, job, got)
	require.EqualValues(t, 5, got.
		MaxConcurrencyPerKey,
	)
	require.Len(t, got.RateLimitKeys,

		1)
	require.Equal(t, "api",

		got.RateLimitKeys[0].
			Name)
	require.EqualValues(t, 10, got.
		RateLimitKeys[0].Max,
	)
	require.EqualValues(t, 60, got.
		RateLimitKeys[0].WindowSecs,
	)
	require.NotNil(t, got.DefaultRunMetadata)
	require.Equal(t, "prod",

		got.DefaultRunMetadata["env"])

}

func TestUpdateJob_NewColumns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-update-new-cols")
	require.NoError(t, q.CreateJob(ctx,
		job))

	// Verify zero-valued defaults after initial create.
	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, got.
		MaxConcurrencyPerKey,
	)
	require.Len(t, got.RateLimitKeys,

		0)
	require.Len(t, got.DefaultRunMetadata,

		0)

	// Set the new columns and update.
	job.MaxConcurrencyPerKey = 3
	job.RateLimitKeys = []domain.RateLimitKey{
		{Name: "user", Max: 100, WindowSecs: 3600},
		{Name: "ip", Max: 20, WindowSecs: 60},
	}
	job.DefaultRunMetadata = map[string]string{"region": "us-east-1", "tier": "premium"}
	require.NoError(t, q.UpdateJob(ctx,
		job))

	got, err = q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 3, got.
		MaxConcurrencyPerKey,
	)
	require.Len(t, got.RateLimitKeys,

		2)
	require.False(t, got.RateLimitKeys[0].Name !=
		"user" ||

		got.RateLimitKeys[0].Max !=

			100 ||
		got.RateLimitKeys[0].WindowSecs != 3600)
	require.False(t, got.RateLimitKeys[1].Name !=
		"ip" || got.
		RateLimitKeys[1].Max !=
		20 ||
		got.RateLimitKeys[1].WindowSecs != 60)
	require.Len(t, got.DefaultRunMetadata,

		2)
	require.Equal(t, "us-east-1",

		got.
			DefaultRunMetadata["region"])
	require.Equal(t, "premium",

		got.DefaultRunMetadata["tier"])

}

func TestUpdateJob_ClearNewColumns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-clear-new-cols")
	job.MaxConcurrencyPerKey = 10
	job.RateLimitKeys = []domain.RateLimitKey{{Name: "global", Max: 50, WindowSecs: 300}}
	job.DefaultRunMetadata = map[string]string{"source": "test"}
	require.NoError(t, q.CreateJob(ctx,
		job))

	// Clear all new columns.
	job.MaxConcurrencyPerKey = 0
	job.RateLimitKeys = nil
	job.DefaultRunMetadata = nil
	require.NoError(t, q.UpdateJob(ctx,
		job))

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, got.
		MaxConcurrencyPerKey,
	)
	require.Len(t, got.RateLimitKeys,

		0)
	require.Len(t, got.DefaultRunMetadata,

		0)

}
