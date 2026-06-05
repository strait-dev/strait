//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/stretchr/testify/require"
)

// DeleteJob (exercises private deleteJobTx indirectly).

func TestJobs_DeleteJob_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-delete-job")
	require.NoError(t, q.DeleteJob(ctx,
		job.ID))

	_, err := q.GetJob(ctx, job.ID)
	require.True(t, errors.Is(err, store.
		ErrJobNotFound,
	))

}

func TestJobs_DeleteJob_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.DeleteJob(ctx, newID())
	require.True(t, errors.Is(err, store.
		ErrJobNotFound,
	))

}

func TestJobs_DeleteJob_BlockedByActiveRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-delete-job-active")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))

	err := q.DeleteJob(ctx, job.ID)
	require.True(t, errors.Is(err, store.
		ErrJobHasActiveRuns,
	))

}

// BulkCountExecutingRunsByOrg.

func TestJobs_BulkCountExecutingRunsByOrg_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-bulk-exec-" + newID()
	projectID := "project-bulk-exec-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))

	job := mustCreateJob(t, ctx, q, projectID)
	for range 3 {
		run := baseRun(job, newID())
		run.Status = domain.StatusExecuting
		require.NoError(t, q.CreateRun(ctx,
			run))

	}

	counts, err := q.BulkCountExecutingRunsByOrg(ctx, []string{orgID})
	require.NoError(t, err)
	require.EqualValues(t, 3, counts[orgID])

}

func TestJobs_BulkCountExecutingRunsByOrg_EmptyInput(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	counts, err := q.BulkCountExecutingRunsByOrg(ctx, []string{})
	require.NoError(t, err)
	require.Len(t, counts,
		0,
	)

}

func TestJobs_BulkCountExecutingRunsByOrg_NoExecuting(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-bulk-no-exec-" + newID()
	projectID := "project-bulk-no-exec-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))

	job := mustCreateJob(t, ctx, q, projectID)
	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	require.NoError(t, q.CreateRun(ctx,
		run))

	counts, err := q.BulkCountExecutingRunsByOrg(ctx, []string{orgID})
	require.NoError(t, err)
	require.EqualValues(t, 0, counts[orgID])

}

// ListOrgsWithExecutingRuns.

func TestJobs_ListOrgsWithExecutingRuns_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-exec-list-" + newID()
	projectID := "project-exec-list-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))

	job := mustCreateJob(t, ctx, q, projectID)
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))

	orgs, err := q.ListOrgsWithExecutingRuns(ctx)
	require.NoError(t, err)

	found := slices.Contains(orgs, orgID)
	require.True(t, found)

}

func TestJobs_ListOrgsWithExecutingRuns_EmptyWhenNone(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgs, err := q.ListOrgsWithExecutingRuns(ctx)
	require.NoError(t, err)
	require.Len(t, orgs, 0)

}

func TestJobs_ListOrgsWithExecutingRuns_Distinct(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-exec-distinct-" + newID()
	projectID := "project-exec-distinct-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))

	job := mustCreateJob(t, ctx, q, projectID)
	for range 3 {
		run := baseRun(job, newID())
		run.Status = domain.StatusExecuting
		require.NoError(t, q.CreateRun(ctx,
			run))

	}

	orgs, err := q.ListOrgsWithExecutingRuns(ctx)
	require.NoError(t, err)

	count := 0
	for _, o := range orgs {
		if o == orgID {
			count++
		}
	}
	require.EqualValues(t, 1, count)

}

// UpdateProjectDefaultRegion.

func TestJobs_UpdateProjectDefaultRegion_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-default-region-" + newID()
	project := &domain.Project{ID: projectID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))
	require.NoError(t, q.UpdateProjectDefaultRegion(ctx, projectID,
		"us-east-1",
	))

	quota, err := q.GetProjectQuota(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, quota)
	require.Equal(t, "us-east-1",

		quota.
			DefaultRegion,
	)

}

func TestJobs_UpdateProjectDefaultRegion_Upsert(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-region-upsert-" + newID()
	project := &domain.Project{ID: projectID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))
	require.NoError(t, q.UpdateProjectDefaultRegion(ctx, projectID,
		"us-east-1",
	))
	require.NoError(t, q.UpdateProjectDefaultRegion(ctx, projectID,
		"eu-west-1",
	))

	quota, err := q.GetProjectQuota(ctx, projectID)
	require.NoError(t, err)
	require.Equal(t, "eu-west-1",

		quota.
			DefaultRegion,
	)

}

// UpdateProjectMaxKeyLifetimeDays.

func TestJobs_UpdateProjectMaxKeyLifetimeDays_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-key-lifetime-" + newID()
	project := &domain.Project{ID: projectID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))
	require.NoError(t, q.UpdateProjectMaxKeyLifetimeDays(ctx, projectID,
		90,
	))

	quota, err := q.GetProjectQuota(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, quota)
	require.EqualValues(t, 90, quota.
		MaxKeyLifetimeDays,
	)

}

func TestJobs_UpdateProjectMaxKeyLifetimeDays_Upsert(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-key-lifetime-upsert-" + newID()
	project := &domain.Project{ID: projectID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))
	require.NoError(t, q.UpdateProjectMaxKeyLifetimeDays(ctx, projectID,
		30,
	))
	require.NoError(t, q.UpdateProjectMaxKeyLifetimeDays(ctx, projectID,
		60,
	))

	quota, err := q.GetProjectQuota(ctx, projectID)
	require.NoError(t, err)
	require.EqualValues(t, 60, quota.
		MaxKeyLifetimeDays,
	)

}

func TestJobs_UpdateProjectQuotaSettings_SameValueNoOpDoesNotRewrite(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-quota-settings-no-op-" + newID()
	project := &domain.Project{ID: projectID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))

	quotaState := func() (string, int64, string, int) {
		t.Helper()
		var xmin string
		var cacheVersion int64
		var defaultRegion string
		var maxKeyLifetimeDays int
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			`
			SELECT xmin::text, cache_version, COALESCE(default_region, ''), max_key_lifetime_days
			FROM project_quotas
			WHERE project_id = $1`,

			projectID).Scan(&xmin, &cacheVersion, &defaultRegion,
			&maxKeyLifetimeDays))

		return xmin, cacheVersion, defaultRegion, maxKeyLifetimeDays
	}
	require.NoError(t, q.UpdateProjectDefaultRegion(ctx, projectID,
		"us-east-1",
	))

	regionXminBefore, regionVersionBefore, _, _ := quotaState()
	require.NoError(t, q.UpdateProjectDefaultRegion(ctx, projectID,
		"us-east-1",
	))

	regionXminAfterNoOp, regionVersionAfterNoOp, regionAfterNoOp, _ := quotaState()
	require.Equal(t, "us-east-1",

		regionAfterNoOp,
	)
	require.Equal(t, regionXminBefore,

		regionXminAfterNoOp,
	)
	require.Equal(t, regionVersionBefore,

		regionVersionAfterNoOp,
	)
	require.NoError(t, q.UpdateProjectDefaultRegion(ctx, projectID,
		"eu-west-1",
	))

	regionXminAfterUpdate, regionVersionAfterUpdate, regionAfterUpdate, _ := quotaState()
	require.Equal(t, "eu-west-1",

		regionAfterUpdate,
	)
	require.NotEqual(t, regionXminBefore,

		regionXminAfterUpdate,
	)
	require.False(t, regionVersionAfterUpdate <=
		regionVersionBefore,
	)
	require.NoError(t, q.UpdateProjectMaxKeyLifetimeDays(ctx, projectID,
		90,
	))

	lifetimeXminBefore, lifetimeVersionBefore, _, _ := quotaState()
	require.NoError(t, q.UpdateProjectMaxKeyLifetimeDays(ctx, projectID,
		90,
	))

	lifetimeXminAfterNoOp, lifetimeVersionAfterNoOp, _, lifetimeAfterNoOp := quotaState()
	require.EqualValues(t, 90, lifetimeAfterNoOp)
	require.Equal(t, lifetimeXminBefore,

		lifetimeXminAfterNoOp,
	)
	require.Equal(t, lifetimeVersionBefore,

		lifetimeVersionAfterNoOp,
	)
	require.NoError(t, q.UpdateProjectMaxKeyLifetimeDays(ctx, projectID,
		60,
	))

	lifetimeXminAfterUpdate, lifetimeVersionAfterUpdate, _, lifetimeAfterUpdate := quotaState()
	require.EqualValues(t, 60, lifetimeAfterUpdate)
	require.NotEqual(t, lifetimeXminBefore,

		lifetimeXminAfterUpdate,
	)
	require.False(t, lifetimeVersionAfterUpdate <=
		lifetimeVersionBefore,
	)

}

// InsertBatchBufferItem.

func TestBatch_InsertBatchBufferItem_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-batch-insert")
	item := &domain.BatchBufferItem{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		BatchKey:    "key-1",
		Payload:     json.RawMessage(`{"a":1}`),
		Tags:        json.RawMessage(`{}`),
		Priority:    0,
		TriggeredBy: "manual",
	}
	require.NoError(t, q.InsertBatchBufferItem(ctx,
		item))
	require.NotEqual(t, "",

		item.ID)
	require.False(t, item.CreatedAt.
		IsZero())

}

func TestBatch_InsertBatchBufferItem_CustomID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-batch-custom-id")
	customID := newID()
	item := &domain.BatchBufferItem{
		ID:          customID,
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		BatchKey:    "key-1",
		Payload:     json.RawMessage(`{"a":1}`),
		Tags:        json.RawMessage(`{}`),
		Priority:    0,
		TriggeredBy: "manual",
	}
	require.NoError(t, q.InsertBatchBufferItem(ctx,
		item))
	require.Equal(t, customID,

		item.ID,
	)

}

func TestBatch_InsertBatchBufferItem_MultipleItems(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-batch-multi")
	for i := range 3 {
		item := &domain.BatchBufferItem{
			JobID:       job.ID,
			ProjectID:   job.ProjectID,
			BatchKey:    "key-1",
			Payload:     json.RawMessage(`{"i":` + string(rune('0'+i)) + `}`),
			Tags:        json.RawMessage(`{}`),
			Priority:    i,
			TriggeredBy: "manual",
		}
		require.NoError(t, q.InsertBatchBufferItem(ctx,
			item))

	}

	count, err := q.CountBatchBufferItems(ctx, job.ID, "key-1")
	require.NoError(t, err)
	require.EqualValues(t, 3, count)

}

// CountBatchBufferItems.

func TestBatch_CountBatchBufferItems_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CountBatchBufferItems(ctx, newID(), "key-1")
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

func TestBatch_CountBatchBufferItems_FiltersByBatchKey(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-batch-count-filter")
	for _, key := range []string{"alpha", "alpha", "beta"} {
		item := &domain.BatchBufferItem{
			JobID:       job.ID,
			ProjectID:   job.ProjectID,
			BatchKey:    key,
			Payload:     json.RawMessage(`{}`),
			Tags:        json.RawMessage(`{}`),
			TriggeredBy: "manual",
		}
		require.NoError(t, q.InsertBatchBufferItem(ctx,
			item))

	}

	countAlpha, err := q.CountBatchBufferItems(ctx, job.ID, "alpha")
	require.NoError(t, err)
	require.EqualValues(t, 2, countAlpha)

	countBeta, err := q.CountBatchBufferItems(ctx, job.ID, "beta")
	require.NoError(t, err)
	require.EqualValues(t, 1, countBeta)

}

// DrainBatchBuffer.

func TestBatch_DrainBatchBuffer_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-drain-batch")
	for range 3 {
		item := &domain.BatchBufferItem{
			JobID:       job.ID,
			ProjectID:   job.ProjectID,
			BatchKey:    "drain-key",
			Payload:     json.RawMessage(`{"ok":true}`),
			Tags:        json.RawMessage(`{}`),
			TriggeredBy: "manual",
		}
		require.NoError(t, q.InsertBatchBufferItem(ctx,
			item))

	}

	drained, err := q.DrainBatchBuffer(ctx, job.ID, "drain-key", 10)
	require.NoError(t, err)
	require.Len(t, drained,

		3)

	// Buffer should be empty after drain.
	count, err := q.CountBatchBufferItems(ctx, job.ID, "drain-key")
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

func TestBatch_DrainBatchBuffer_LimitRespected(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-drain-limit")
	for range 5 {
		item := &domain.BatchBufferItem{
			JobID:       job.ID,
			ProjectID:   job.ProjectID,
			BatchKey:    "lim-key",
			Payload:     json.RawMessage(`{}`),
			Tags:        json.RawMessage(`{}`),
			TriggeredBy: "manual",
		}
		require.NoError(t, q.InsertBatchBufferItem(ctx,
			item))

	}

	drained, err := q.DrainBatchBuffer(ctx, job.ID, "lim-key", 2)
	require.NoError(t, err)
	require.Len(t, drained,

		2)

	remaining, _ := q.CountBatchBufferItems(ctx, job.ID, "lim-key")
	require.EqualValues(t, 3, remaining)

}

func TestBatch_DrainBatchBuffer_SkipsRowsLockedByConcurrentDrain(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-drain-locked")
	for i := range 4 {
		item := &domain.BatchBufferItem{
			ID:          "locked-batch-item-" + newID(),
			JobID:       job.ID,
			ProjectID:   job.ProjectID,
			BatchKey:    "locked-key",
			Payload:     json.RawMessage(`{"i":` + string(rune('0'+i)) + `}`),
			Tags:        json.RawMessage(`{}`),
			TriggeredBy: "manual",
		}
		require.NoError(t, q.InsertBatchBufferItem(ctx,
			item))

	}

	tx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	defer func() { _ = tx.Rollback(ctx) }()

	firstDrain, err := q.DrainBatchBufferInTx(ctx, tx, job.ID, "locked-key", 2)
	require.NoError(t, err)
	require.Len(t, firstDrain,

		2)

	secondDrain, err := q.DrainBatchBuffer(ctx, job.ID, "locked-key", 4)
	require.NoError(t, err)
	require.Len(t, secondDrain,

		2)

	seen := make(map[string]bool, 4)
	for _, item := range firstDrain {
		seen[item.ID] = true
	}
	for _, item := range secondDrain {
		require.False(t, seen[item.
			ID])

		seen[item.ID] = true
	}
	require.Len(t, seen, 4)
	require.NoError(t, tx.Commit(ctx))

	remaining, err := q.CountBatchBufferItems(ctx, job.ID, "locked-key")
	require.NoError(t, err)
	require.EqualValues(t, 0, remaining)

}

func TestBatch_DrainBatchBuffer_EmptyBuffer(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	drained, err := q.DrainBatchBuffer(ctx, newID(), "empty-key", 10)
	require.NoError(t, err)
	require.Len(t, drained,

		0)

}

// ListFlushableBatches.

func TestBatch_ListFlushableBatches_ByMaxSize(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Create a job with batch_max_size = 2.
	job := baseJob(newID(), "project-flushable-max-size")
	job.BatchMaxSize = 2
	require.NoError(t, q.CreateJob(ctx,
		job))

	for range 2 {
		item := &domain.BatchBufferItem{
			JobID:       job.ID,
			ProjectID:   job.ProjectID,
			BatchKey:    "flush-key",
			Payload:     json.RawMessage(`{}`),
			Tags:        json.RawMessage(`{}`),
			TriggeredBy: "manual",
		}
		require.NoError(t, q.InsertBatchBufferItem(ctx,
			item))

	}

	batches, err := q.ListFlushableBatches(ctx)
	require.NoError(t, err)

	found := false
	for _, b := range batches {
		if b.JobID == job.ID && b.BatchKey == "flush-key" {
			found = true
			require.EqualValues(t, 2, b.ItemCount)

		}
	}
	require.True(t, found)

}

func TestBatch_ListFlushableBatches_EmptyWhenNotReady(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Create a job with batch_max_size = 10 but only insert 1 item.
	job := baseJob(newID(), "project-flushable-not-ready")
	job.BatchMaxSize = 10
	require.NoError(t, q.CreateJob(ctx,
		job))

	item := &domain.BatchBufferItem{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		BatchKey:    "not-ready",
		Payload:     json.RawMessage(`{}`),
		Tags:        json.RawMessage(`{}`),
		TriggeredBy: "manual",
	}
	require.NoError(t, q.InsertBatchBufferItem(ctx,
		item))

	batches, err := q.ListFlushableBatches(ctx)
	require.NoError(t, err)

	for _, b := range batches {
		require.NotEqual(t, job.
			ID, b.JobID,
		)

	}
}

// ListEventsAsc.

func TestEvents_ListEventsAsc_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-events-asc")
	run := mustCreateRun(t, ctx, q, job)

	for i := range 3 {
		ev := &domain.RunEvent{
			RunID:   run.ID,
			Type:    domain.EventType("log"),
			Message: "event-" + string(rune('A'+i)),
			Data:    json.RawMessage(`{}`),
		}
		require.NoError(t, q.InsertEvent(
			ctx, ev))

		// Sleep briefly to ensure ordering.
		time.Sleep(2 * time.Millisecond)
	}

	events, err := q.ListEventsAsc(ctx, run.ID, 10, nil, "")
	require.NoError(t, err)
	require.Len(t, events,
		3,
	)

	assertEventTimesAsc(t, events)
}

func TestEvents_ListEventsAsc_WithCursor(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-events-asc-cursor")
	run := mustCreateRun(t, ctx, q, job)

	var firstEvent *domain.RunEvent
	for i := range 3 {
		ev := &domain.RunEvent{
			RunID:   run.ID,
			Type:    domain.EventType("log"),
			Message: "event-" + string(rune('A'+i)),
			Data:    json.RawMessage(`{}`),
		}
		require.NoError(t, q.InsertEvent(
			ctx, ev))

		if i == 0 {
			firstEvent = ev
		}
		time.Sleep(2 * time.Millisecond)
	}

	events, err := q.ListEventsAsc(ctx, run.ID, 10, &firstEvent.CreatedAt, firstEvent.ID)
	require.NoError(t, err)
	require.Len(t, events,
		2,
	)

}

func TestEvents_ListEventsAsc_EmptyRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	events, err := q.ListEventsAsc(ctx, newID(), 10, nil, "")
	require.NoError(t, err)
	require.Len(t, events,
		0,
	)

}

// DeleteProject (exercises private deleteProjectRows indirectly).

func TestProjects_DeleteProject_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-delete-" + newID()
	project := &domain.Project{ID: projectID, Name: "to-delete", OrgID: "org-delete"}
	require.NoError(t, q.CreateProject(ctx, project))
	require.NoError(t, q.DeleteProject(ctx, projectID))

	_, err := q.GetProject(ctx, projectID)
	require.True(t, errors.Is(err, store.
		ErrProjectNotFound,
	))

}

func TestProjects_DeleteProject_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.DeleteProject(ctx, "nonexistent-project-"+newID())
	require.True(t, errors.Is(err, store.
		ErrProjectNotFound,
	))

}

func TestProjects_DeleteProject_DisablesJobs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-delete-disables-" + newID()
	project := &domain.Project{ID: projectID, Name: "to-delete", OrgID: "org-delete"}
	require.NoError(t, q.CreateProject(ctx, project))

	job := mustCreateJob(t, ctx, q, projectID)
	require.NoError(t, q.DeleteProject(ctx, projectID))

	// Job should still exist but be disabled.
	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.False(t, got.Enabled)

}

// ListJobsByOrg.

func TestJobs_ListJobsByOrg_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-list-jobs-" + newID()
	projectID := "project-list-jobs-org-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))

	for range 3 {
		_ = mustCreateJob(t, ctx, q, projectID)
	}

	jobs, err := q.ListJobsByOrg(ctx, orgID, 100, nil)
	require.NoError(t, err)
	require.Len(t, jobs, 3)

}

func TestJobs_ListJobsByOrg_EmptyOrg(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	jobs, err := q.ListJobsByOrg(ctx, "nonexistent-org-"+newID(), 100, nil)
	require.NoError(t, err)
	require.Len(t, jobs, 0)

}

func TestJobs_ListJobsByOrg_WithCursor(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-list-jobs-cursor-" + newID()
	projectID := "project-list-jobs-cursor-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))

	for range 3 {
		_ = mustCreateJob(t, ctx, q, projectID)
		time.Sleep(2 * time.Millisecond)
	}

	allJobs, _ := q.ListJobsByOrg(ctx, orgID, 100, nil)
	require.GreaterOrEqual(
		t,
		len(allJobs), 2)

	cursor := allJobs[0].CreatedAt
	paged, err := q.ListJobsByOrg(ctx, orgID, 100, &cursor)
	require.NoError(t, err)
	require.Len(t, paged, len(allJobs)-1)

}

// ListRunsByOrg.

func TestJobs_ListRunsByOrg_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-list-runs-" + newID()
	projectID := "project-list-runs-org-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))

	job := mustCreateJob(t, ctx, q, projectID)
	for range 2 {
		_ = mustCreateRun(t, ctx, q, job)
	}

	runs, err := q.ListRunsByOrg(ctx, orgID, 100, nil)
	require.NoError(t, err)
	require.Len(t, runs, 2)

}

func TestJobs_ListRunsByOrg_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runs, err := q.ListRunsByOrg(ctx, "nonexistent-org-"+newID(), 100, nil)
	require.NoError(t, err)
	require.Len(t, runs, 0)

}

func TestJobs_ListRunsByOrg_ExcludesDeletedProjects(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-list-runs-deleted-" + newID()
	projectID := "project-list-runs-deleted-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))

	job := mustCreateJob(t, ctx, q, projectID)
	_ = mustCreateRun(t, ctx, q, job)
	require.NoError(t, q.DeleteProject(ctx, projectID))

	runs, err := q.ListRunsByOrg(ctx, orgID, 100, nil)
	require.NoError(t, err)
	require.Len(t, runs, 0)

}

func TestJobs_ListRunsByOrg_ExcludesRetentionMaskedRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-list-runs-masked-" + newID()
	projectID := "project-list-runs-masked-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))

	job := mustCreateJob(t, ctx, q, projectID)
	visibleRun := mustCreateRun(t, ctx, q, job)
	maskedRun := mustCreateRun(t, ctx, q, job)
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET visible_until = NOW() WHERE id = $1`, maskedRun.ID); err != nil {
		require.Failf(t, "test failure",

			"mask run: %v", err)
	}

	runs, err := q.ListRunsByOrg(ctx, orgID, 100, nil)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, visibleRun.
		ID, runs[0].ID)

}

// ListEnabledLogDrains.

func TestLogDrains_ListEnabledLogDrains_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-log-drains-" + newID()
	project := &domain.Project{ID: projectID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))

	enabled := &domain.LogDrain{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "enabled-drain",
		DrainType:   "http",
		EndpointURL: "https://example.com/logs",
		AuthType:    "bearer",
		Enabled:     true,
	}
	require.NoError(t, q.CreateLogDrain(ctx, enabled))

	disabled := &domain.LogDrain{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "disabled-drain",
		DrainType:   "http",
		EndpointURL: "https://example.com/logs-disabled",
		AuthType:    "bearer",
		Enabled:     false,
	}
	require.NoError(t, q.CreateLogDrain(ctx, disabled))

	drains, err := q.ListEnabledLogDrains(ctx)
	require.NoError(t, err)

	found := false
	for _, d := range drains {
		require.NotEqual(t, disabled.
			ID,
			d.ID)

		if d.ID == enabled.ID {
			found = true
		}
	}
	require.True(t, found)

}

func TestLogDrains_ListEnabledLogDrains_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	drains, err := q.ListEnabledLogDrains(ctx)
	require.NoError(t, err)
	require.Len(t, drains,
		0,
	)

}

func TestLogDrains_ListEnabledLogDrains_AllDisabled(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-log-drains-all-disabled-" + newID()
	project := &domain.Project{ID: projectID, Name: "test"}
	require.NoError(t, q.CreateProject(ctx, project))

	drain := &domain.LogDrain{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "disabled-only",
		DrainType:   "http",
		EndpointURL: "https://example.com/logs-off",
		AuthType:    "none",
		Enabled:     false,
	}
	require.NoError(t, q.CreateLogDrain(ctx, drain))

	drains, err := q.ListEnabledLogDrains(ctx)
	require.NoError(t, err)

	for _, d := range drains {
		require.NotEqual(t, drain.
			ID, d.ID,
		)

	}
}

// Suppress unused import warning.
var _ = testutil.Ptr[string]
