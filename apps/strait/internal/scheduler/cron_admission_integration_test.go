//go:build integration

package scheduler

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

func TestCronScheduler_TriggerJob_ProjectQueuedQuotaPreventsInsert(t *testing.T) {
	ctx := context.Background()
	tdb, err := testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup db: %v", err)
	}
	t.Cleanup(func() { tdb.Cleanup(ctx) })

	st := store.New(tdb.Pool)
	pq := queue.NewPostgresQueue(tdb.Pool)
	project := &domain.Project{
		ID:    "cron-quota-" + uuid.Must(uuid.NewV7()).String(),
		OrgID: "org-" + uuid.Must(uuid.NewV7()).String(),
		Name:  "cron quota",
	}
	if err := st.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO project_quotas (project_id, max_queued_runs)
		VALUES ($1, 1)
	`, project.ID); err != nil {
		t.Fatalf("insert project quota: %v", err)
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
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	existing := &domain.JobRun{
		ID:        uuid.Must(uuid.NewV7()).String(),
		JobID:     job.ID,
		ProjectID: project.ID,
		Status:    domain.StatusQueued,
	}
	if err := pq.Enqueue(ctx, existing); err != nil {
		t.Fatalf("Enqueue existing run: %v", err)
	}

	cs := NewCronScheduler(ctx, st, pq, nil)
	cs.triggerJob(ctx, *job)

	count, err := st.CountProjectQueuedRuns(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountProjectQueuedRuns() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("queued run count = %d, want unchanged count 1", count)
	}
}

func TestCronScheduler_LoadJobs_SkipsInvalidStoredJobTimezone(t *testing.T) {
	ctx := context.Background()
	tdb, err := testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup db: %v", err)
	}
	t.Cleanup(func() { tdb.Cleanup(ctx) })

	st := store.New(tdb.Pool)
	project := &domain.Project{
		ID:    "cron-tz-" + uuid.Must(uuid.NewV7()).String(),
		OrgID: "org-" + uuid.Must(uuid.NewV7()).String(),
		Name:  "cron timezone",
	}
	if err := st.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

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
		if err := st.CreateJob(ctx, job); err != nil {
			t.Fatalf("CreateJob(%s) error = %v", tt.name, err)
		}
	}

	cs := NewCronScheduler(ctx, st, &mockQueue{}, nil)
	if err := cs.LoadJobs(ctx); err != nil {
		t.Fatalf("LoadJobs() error = %v", err)
	}
	if got := len(cs.cron.Entries()); got != 1 {
		t.Fatalf("loaded cron entries = %d, want only the valid timezone job", got)
	}
}
