//go:build integration

package scheduler

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCronScheduler_TriggerJob_ProjectQueuedQuotaPreventsInsert(t *testing.T) {
	ctx := context.Background()
	tdb := cleanSchedulerIntegrationDB(t, ctx)

	st := store.New(tdb.Pool)
	pq := queue.NewPgQueQueue(tdb.Pool, queue.NewPostgresRunWriter(tdb.Pool), queue.PgQueConfig{})
	project := &domain.Project{
		ID:    "cron-quota-" + uuid.Must(uuid.NewV7()).String(),
		OrgID: "org-" + uuid.Must(uuid.NewV7()).String(),
		Name:  "cron quota",
	}
	require.NoError(t, st.
		CreateProject(ctx, project))

	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO project_quotas (project_id, max_queued_runs)
		VALUES ($1, 1)
	`, project.ID); err != nil {
		require.Failf(t, "test failure",

			"insert project quota: %v", err)
	}

	job := &domain.Job{
		ID:          uuid.Must(uuid.NewV7()).String(),
		ProjectID:   project.ID,
		Name:        "cron quota job",
		Slug:        "cron-quota-" + uuid.Must(uuid.NewV7()).String()[:8],
		Cron:        "* * * * *",
		EndpointURL: "https://example.com/cron",
		MaxAttempts: 3,
		TimeoutSecs: 60,
		Enabled:     true,
	}
	require.NoError(t, st.
		CreateJob(
			ctx, job))

	existing := &domain.JobRun{
		ID:        uuid.Must(uuid.NewV7()).String(),
		JobID:     job.ID,
		ProjectID: project.ID,
		Status:    domain.StatusQueued,
	}
	require.NoError(t, pq.
		Enqueue(ctx,
			existing,
		))

	cs := NewCronScheduler(ctx, st, pq, nil)
	cs.triggerJob(ctx, *job)

	count, err := st.CountProjectQueuedRuns(ctx, project.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

}

func TestCronScheduler_LoadJobs_SkipsInvalidStoredJobTimezone(t *testing.T) {
	ctx := context.Background()
	tdb := cleanSchedulerIntegrationDB(t, ctx)

	st := store.New(tdb.Pool)
	project := &domain.Project{
		ID:    "cron-tz-" + uuid.Must(uuid.NewV7()).String(),
		OrgID: "org-" + uuid.Must(uuid.NewV7()).String(),
		Name:  "cron timezone",
	}
	require.NoError(t, st.
		CreateProject(ctx, project))

	for _, tt := range []struct {
		name     string
		timezone string
	}{
		{name: "valid", timezone: "UTC"},
		{name: "invalid", timezone: "Mars/Olympus"},
	} {
		job := &domain.Job{
			ID:          uuid.Must(uuid.NewV7()).String(),
			ProjectID:   project.ID,
			Name:        "cron tz " + tt.name,
			Slug:        "cron-tz-" + tt.name + "-" + uuid.Must(uuid.NewV7()).String()[:8],
			Cron:        "* * * * *",
			Timezone:    tt.timezone,
			EndpointURL: "https://example.com/cron",
			MaxAttempts: 3,
			TimeoutSecs: 60,
			Enabled:     true,
		}
		require.NoError(t, st.
			CreateJob(
				ctx, job))

	}

	cs := NewCronScheduler(ctx, st, &mockQueue{}, nil)
	require.NoError(t, cs.
		LoadJobs(ctx))
	require.EqualValues(t, 1, len(cs.cron.
		Entries()))

}
