//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestCreateLogDrainWithOrgLimit_SerializesConcurrentCreates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-log-drain-limit-" + newID()
	projectID := "proj-log-drain-limit-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID,

		Name: "P"}))

	errs := make(chan error, 20)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			drain := &domain.LogDrain{
				ID:          "drain-limit-" + newID(),
				ProjectID:   projectID,
				Name:        fmt.Sprintf("drain-%d", i),
				DrainType:   "http",
				EndpointURL: "https://example.com/logs",
				AuthType:    "none",
				Enabled:     true,
			}
			errs <- q.CreateLogDrainWithOrgLimit(ctx, drain, orgID, 2)
		}(i)
	}
	wg.Wait()
	close(errs)

	assertConcurrentLimitResults(t, errs, 2, store.ErrLogDrainLimitExceeded)

	count, err := q.CountLogDrainsByOrg(ctx, orgID)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

}

func TestCreateNotificationChannelWithProjectLimit_SerializesConcurrentCreates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-channel-limit-" + newID()
	errs := make(chan error, 20)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			channel := &domain.NotificationChannel{
				ID:          "channel-limit-" + newID(),
				ProjectID:   projectID,
				ChannelType: domain.ChannelTypeWebhook,
				Name:        fmt.Sprintf("ops-%d", i),
				Config:      []byte(`{"url":"https://example.com/hooks/ops"}`),
				Enabled:     true,
			}
			errs <- q.CreateNotificationChannelWithProjectLimit(ctx, channel, 3)
		}(i)
	}
	wg.Wait()
	close(errs)

	assertConcurrentLimitResults(t, errs, 3, store.ErrNotificationChannelLimitExceeded)

	count, err := q.CountNotificationChannelsByProject(ctx, projectID)
	require.NoError(t, err)
	require.EqualValues(t, 3, count)

}

func TestCreateEnvironmentWithOrgLimit_SerializesConcurrentCreates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-env-limit-" + newID()
	projectID := "proj-env-limit-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID,

		Name: "P"}))

	errs := make(chan error, 20)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			env := &domain.Environment{
				ID:        "env-limit-" + newID(),
				ProjectID: projectID,
				Name:      fmt.Sprintf("env-%d", i),
				Slug:      fmt.Sprintf("env-%d-%s", i, newID()),
			}
			errs <- q.CreateEnvironmentWithOrgLimit(ctx, env, orgID, 2)
		}(i)
	}
	wg.Wait()
	close(errs)

	assertConcurrentLimitResults(t, errs, 2, store.ErrEnvironmentLimitExceeded)

	count, err := q.CountEnvironmentsByOrg(ctx, orgID)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

}

func TestCreateJobWithCronScheduleLimit_SerializesConcurrentCreates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-job-cron-limit-" + newID()
	projectID := "proj-job-cron-limit-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID,

		Name: "P"}))

	errs := make(chan error, 20)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			job := baseJob("job-cron-limit-"+newID(), projectID)
			job.Slug = fmt.Sprintf("job-%d-%s", i, newID())
			job.Cron = "*/5 * * * *"
			errs <- q.CreateJobWithCronScheduleLimit(ctx, job, orgID, 2)
		}(i)
	}
	wg.Wait()
	close(errs)

	assertConcurrentLimitResults(t, errs, 2, store.ErrCronScheduleLimitExceeded)

	count, err := q.CountCronJobsByOrg(ctx, orgID)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

}

func TestEnforceCronScheduleLimit_SerializesJobsAndWorkflows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-mixed-cron-limit-" + newID()
	projectID := "proj-mixed-cron-limit-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID,

		Name: "P"}))

	errs := make(chan error, 20)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- q.WithTxQueries(ctx, func(txq *store.Queries) error {
				if err := txq.EnforceCronScheduleLimit(ctx, orgID, 3); err != nil {
					return err
				}
				if i%2 == 0 {
					job := baseJob("mixed-cron-job-"+newID(), projectID)
					job.Slug = fmt.Sprintf("mixed-job-%d-%s", i, newID())
					job.Cron = "*/5 * * * *"
					return txq.CreateJob(ctx, job)
				}
				workflow := &domain.Workflow{
					ID:        "mixed-cron-wf-" + newID(),
					ProjectID: projectID,
					Name:      fmt.Sprintf("workflow-%d", i),
					Slug:      fmt.Sprintf("workflow-%d-%s", i, newID()),
					Enabled:   true,
					Cron:      "*/5 * * * *",
				}
				return txq.CreateWorkflow(ctx, workflow)
			})
		}(i)
	}
	wg.Wait()
	close(errs)

	assertConcurrentLimitResults(t, errs, 3, store.ErrCronScheduleLimitExceeded)

	count, err := q.CountCronJobsByOrg(ctx, orgID)
	require.NoError(t, err)
	require.EqualValues(t, 3, count)

}

func assertConcurrentLimitResults(t *testing.T, errs <-chan error, wantSuccesses int, limitErr error) {
	t.Helper()

	successes := 0
	rejections := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, limitErr):
			rejections++
		default:
			require.Failf(t, "test failure", "unexpected error: %v", err)
		}
	}
	require.Equal(t, wantSuccesses,

		successes,
	)
	require.NotEqual(t, 0,
		rejections,
	)

}
