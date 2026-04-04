//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

// --------------------------------------------------------------------------.
// DeleteJob (exercises private deleteJobTx indirectly).
// --------------------------------------------------------------------------.

func TestJobs_DeleteJob_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-delete-job")

	if err := q.DeleteJob(ctx, job.ID); err != nil {
		t.Fatalf("DeleteJob() error = %v", err)
	}

	_, err := q.GetJob(ctx, job.ID)
	if !errors.Is(err, store.ErrJobNotFound) {
		t.Fatalf("GetJob() after delete error = %v, want ErrJobNotFound", err)
	}
}

func TestJobs_DeleteJob_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.DeleteJob(ctx, newID())
	if !errors.Is(err, store.ErrJobNotFound) {
		t.Fatalf("DeleteJob() error = %v, want ErrJobNotFound", err)
	}
}

func TestJobs_DeleteJob_BlockedByActiveRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-delete-job-active")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	err := q.DeleteJob(ctx, job.ID)
	if !errors.Is(err, store.ErrJobHasActiveRuns) {
		t.Fatalf("DeleteJob() error = %v, want ErrJobHasActiveRuns", err)
	}
}

// --------------------------------------------------------------------------.
// BulkCountExecutingRunsByOrg.
// --------------------------------------------------------------------------.

func TestJobs_BulkCountExecutingRunsByOrg_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-bulk-exec-" + newID()
	projectID := "project-bulk-exec-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, q, projectID)
	for range 3 {
		run := baseRun(job, newID())
		run.Status = domain.StatusExecuting
		if err := q.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun() error = %v", err)
		}
	}

	counts, err := q.BulkCountExecutingRunsByOrg(ctx, []string{orgID})
	if err != nil {
		t.Fatalf("BulkCountExecutingRunsByOrg() error = %v", err)
	}
	if counts[orgID] != 3 {
		t.Fatalf("count = %d, want 3", counts[orgID])
	}
}

func TestJobs_BulkCountExecutingRunsByOrg_EmptyInput(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	counts, err := q.BulkCountExecutingRunsByOrg(ctx, []string{})
	if err != nil {
		t.Fatalf("BulkCountExecutingRunsByOrg() error = %v", err)
	}
	if len(counts) != 0 {
		t.Fatalf("len = %d, want 0", len(counts))
	}
}

func TestJobs_BulkCountExecutingRunsByOrg_NoExecuting(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-bulk-no-exec-" + newID()
	projectID := "project-bulk-no-exec-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, q, projectID)
	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	counts, err := q.BulkCountExecutingRunsByOrg(ctx, []string{orgID})
	if err != nil {
		t.Fatalf("BulkCountExecutingRunsByOrg() error = %v", err)
	}
	if counts[orgID] != 0 {
		t.Fatalf("count = %d, want 0", counts[orgID])
	}
}

// --------------------------------------------------------------------------.
// ListOrgsWithExecutingRuns.
// --------------------------------------------------------------------------.

func TestJobs_ListOrgsWithExecutingRuns_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-exec-list-" + newID()
	projectID := "project-exec-list-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, q, projectID)
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	orgs, err := q.ListOrgsWithExecutingRuns(ctx)
	if err != nil {
		t.Fatalf("ListOrgsWithExecutingRuns() error = %v", err)
	}

	found := false
	for _, o := range orgs {
		if o == orgID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("org %q not in result %v", orgID, orgs)
	}
}

func TestJobs_ListOrgsWithExecutingRuns_EmptyWhenNone(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgs, err := q.ListOrgsWithExecutingRuns(ctx)
	if err != nil {
		t.Fatalf("ListOrgsWithExecutingRuns() error = %v", err)
	}
	if len(orgs) != 0 {
		t.Fatalf("len = %d, want 0", len(orgs))
	}
}

func TestJobs_ListOrgsWithExecutingRuns_Distinct(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-exec-distinct-" + newID()
	projectID := "project-exec-distinct-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, q, projectID)
	for range 3 {
		run := baseRun(job, newID())
		run.Status = domain.StatusExecuting
		if err := q.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun() error = %v", err)
		}
	}

	orgs, err := q.ListOrgsWithExecutingRuns(ctx)
	if err != nil {
		t.Fatalf("ListOrgsWithExecutingRuns() error = %v", err)
	}

	count := 0
	for _, o := range orgs {
		if o == orgID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("org %q appeared %d times, want 1", orgID, count)
	}
}

// --------------------------------------------------------------------------.
// UpdateProjectDefaultRegion.
// --------------------------------------------------------------------------.

func TestJobs_UpdateProjectDefaultRegion_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-default-region-" + newID()
	project := &domain.Project{ID: projectID, Name: "test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if err := q.UpdateProjectDefaultRegion(ctx, projectID, "us-east-1"); err != nil {
		t.Fatalf("UpdateProjectDefaultRegion() error = %v", err)
	}

	quota, err := q.GetProjectQuota(ctx, projectID)
	if err != nil {
		t.Fatalf("GetProjectQuota() error = %v", err)
	}
	if quota == nil {
		t.Fatal("quota is nil after upsert")
	}
	if quota.DefaultRegion != "us-east-1" {
		t.Fatalf("default_region = %q, want us-east-1", quota.DefaultRegion)
	}
}

func TestJobs_UpdateProjectDefaultRegion_Upsert(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-region-upsert-" + newID()
	project := &domain.Project{ID: projectID, Name: "test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if err := q.UpdateProjectDefaultRegion(ctx, projectID, "us-east-1"); err != nil {
		t.Fatalf("first upsert error = %v", err)
	}
	if err := q.UpdateProjectDefaultRegion(ctx, projectID, "eu-west-1"); err != nil {
		t.Fatalf("second upsert error = %v", err)
	}

	quota, err := q.GetProjectQuota(ctx, projectID)
	if err != nil {
		t.Fatalf("GetProjectQuota() error = %v", err)
	}
	if quota.DefaultRegion != "eu-west-1" {
		t.Fatalf("default_region = %q, want eu-west-1", quota.DefaultRegion)
	}
}

// --------------------------------------------------------------------------.
// UpdateProjectMaxKeyLifetimeDays.
// --------------------------------------------------------------------------.

func TestJobs_UpdateProjectMaxKeyLifetimeDays_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-key-lifetime-" + newID()
	project := &domain.Project{ID: projectID, Name: "test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if err := q.UpdateProjectMaxKeyLifetimeDays(ctx, projectID, 90); err != nil {
		t.Fatalf("UpdateProjectMaxKeyLifetimeDays() error = %v", err)
	}

	quota, err := q.GetProjectQuota(ctx, projectID)
	if err != nil {
		t.Fatalf("GetProjectQuota() error = %v", err)
	}
	if quota == nil {
		t.Fatal("quota is nil after upsert")
	}
	if quota.MaxKeyLifetimeDays != 90 {
		t.Fatalf("max_key_lifetime_days = %d, want 90", quota.MaxKeyLifetimeDays)
	}
}

func TestJobs_UpdateProjectMaxKeyLifetimeDays_Upsert(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-key-lifetime-upsert-" + newID()
	project := &domain.Project{ID: projectID, Name: "test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if err := q.UpdateProjectMaxKeyLifetimeDays(ctx, projectID, 30); err != nil {
		t.Fatalf("first upsert error = %v", err)
	}
	if err := q.UpdateProjectMaxKeyLifetimeDays(ctx, projectID, 60); err != nil {
		t.Fatalf("second upsert error = %v", err)
	}

	quota, err := q.GetProjectQuota(ctx, projectID)
	if err != nil {
		t.Fatalf("GetProjectQuota() error = %v", err)
	}
	if quota.MaxKeyLifetimeDays != 60 {
		t.Fatalf("max_key_lifetime_days = %d, want 60", quota.MaxKeyLifetimeDays)
	}
}

// --------------------------------------------------------------------------.
// InsertBatchBufferItem.
// --------------------------------------------------------------------------.

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

	if err := q.InsertBatchBufferItem(ctx, item); err != nil {
		t.Fatalf("InsertBatchBufferItem() error = %v", err)
	}
	if item.ID == "" {
		t.Fatal("ID should be generated")
	}
	if item.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should be set")
	}
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

	if err := q.InsertBatchBufferItem(ctx, item); err != nil {
		t.Fatalf("InsertBatchBufferItem() error = %v", err)
	}
	if item.ID != customID {
		t.Fatalf("ID = %q, want %q", item.ID, customID)
	}
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
		if err := q.InsertBatchBufferItem(ctx, item); err != nil {
			t.Fatalf("InsertBatchBufferItem(%d) error = %v", i, err)
		}
	}

	count, err := q.CountBatchBufferItems(ctx, job.ID, "key-1")
	if err != nil {
		t.Fatalf("CountBatchBufferItems() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
}

// --------------------------------------------------------------------------.
// CountBatchBufferItems.
// --------------------------------------------------------------------------.

func TestBatch_CountBatchBufferItems_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CountBatchBufferItems(ctx, newID(), "key-1")
	if err != nil {
		t.Fatalf("CountBatchBufferItems() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
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
		if err := q.InsertBatchBufferItem(ctx, item); err != nil {
			t.Fatalf("InsertBatchBufferItem() error = %v", err)
		}
	}

	countAlpha, err := q.CountBatchBufferItems(ctx, job.ID, "alpha")
	if err != nil {
		t.Fatalf("CountBatchBufferItems(alpha) error = %v", err)
	}
	if countAlpha != 2 {
		t.Fatalf("alpha count = %d, want 2", countAlpha)
	}

	countBeta, err := q.CountBatchBufferItems(ctx, job.ID, "beta")
	if err != nil {
		t.Fatalf("CountBatchBufferItems(beta) error = %v", err)
	}
	if countBeta != 1 {
		t.Fatalf("beta count = %d, want 1", countBeta)
	}
}

// --------------------------------------------------------------------------.
// DrainBatchBuffer.
// --------------------------------------------------------------------------.

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
		if err := q.InsertBatchBufferItem(ctx, item); err != nil {
			t.Fatalf("InsertBatchBufferItem() error = %v", err)
		}
	}

	drained, err := q.DrainBatchBuffer(ctx, job.ID, "drain-key", 10)
	if err != nil {
		t.Fatalf("DrainBatchBuffer() error = %v", err)
	}
	if len(drained) != 3 {
		t.Fatalf("drained len = %d, want 3", len(drained))
	}

	// Buffer should be empty after drain.
	count, err := q.CountBatchBufferItems(ctx, job.ID, "drain-key")
	if err != nil {
		t.Fatalf("CountBatchBufferItems() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count after drain = %d, want 0", count)
	}
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
		if err := q.InsertBatchBufferItem(ctx, item); err != nil {
			t.Fatalf("InsertBatchBufferItem() error = %v", err)
		}
	}

	drained, err := q.DrainBatchBuffer(ctx, job.ID, "lim-key", 2)
	if err != nil {
		t.Fatalf("DrainBatchBuffer() error = %v", err)
	}
	if len(drained) != 2 {
		t.Fatalf("drained len = %d, want 2", len(drained))
	}

	remaining, _ := q.CountBatchBufferItems(ctx, job.ID, "lim-key")
	if remaining != 3 {
		t.Fatalf("remaining = %d, want 3", remaining)
	}
}

func TestBatch_DrainBatchBuffer_EmptyBuffer(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	drained, err := q.DrainBatchBuffer(ctx, newID(), "empty-key", 10)
	if err != nil {
		t.Fatalf("DrainBatchBuffer() error = %v", err)
	}
	if len(drained) != 0 {
		t.Fatalf("drained len = %d, want 0", len(drained))
	}
}

// --------------------------------------------------------------------------.
// ListFlushableBatches.
// --------------------------------------------------------------------------.

func TestBatch_ListFlushableBatches_ByMaxSize(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Create a job with batch_max_size = 2.
	job := baseJob(newID(), "project-flushable-max-size")
	job.BatchMaxSize = 2
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	for range 2 {
		item := &domain.BatchBufferItem{
			JobID:       job.ID,
			ProjectID:   job.ProjectID,
			BatchKey:    "flush-key",
			Payload:     json.RawMessage(`{}`),
			Tags:        json.RawMessage(`{}`),
			TriggeredBy: "manual",
		}
		if err := q.InsertBatchBufferItem(ctx, item); err != nil {
			t.Fatalf("InsertBatchBufferItem() error = %v", err)
		}
	}

	batches, err := q.ListFlushableBatches(ctx)
	if err != nil {
		t.Fatalf("ListFlushableBatches() error = %v", err)
	}

	found := false
	for _, b := range batches {
		if b.JobID == job.ID && b.BatchKey == "flush-key" {
			found = true
			if b.ItemCount != 2 {
				t.Fatalf("item_count = %d, want 2", b.ItemCount)
			}
		}
	}
	if !found {
		t.Fatal("expected flushable batch not found")
	}
}

func TestBatch_ListFlushableBatches_EmptyWhenNotReady(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Create a job with batch_max_size = 10 but only insert 1 item.
	job := baseJob(newID(), "project-flushable-not-ready")
	job.BatchMaxSize = 10
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	item := &domain.BatchBufferItem{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		BatchKey:    "not-ready",
		Payload:     json.RawMessage(`{}`),
		Tags:        json.RawMessage(`{}`),
		TriggeredBy: "manual",
	}
	if err := q.InsertBatchBufferItem(ctx, item); err != nil {
		t.Fatalf("InsertBatchBufferItem() error = %v", err)
	}

	batches, err := q.ListFlushableBatches(ctx)
	if err != nil {
		t.Fatalf("ListFlushableBatches() error = %v", err)
	}

	for _, b := range batches {
		if b.JobID == job.ID {
			t.Fatal("batch should not be flushable with only 1 item and max_size=10")
		}
	}
}

// --------------------------------------------------------------------------.
// ListEventsAsc.
// --------------------------------------------------------------------------.

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
		if err := q.InsertEvent(ctx, ev); err != nil {
			t.Fatalf("InsertEvent(%d) error = %v", i, err)
		}
		// Sleep briefly to ensure ordering.
		time.Sleep(2 * time.Millisecond)
	}

	events, err := q.ListEventsAsc(ctx, run.ID, 10, nil, "")
	if err != nil {
		t.Fatalf("ListEventsAsc() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("len = %d, want 3", len(events))
	}
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
		if err := q.InsertEvent(ctx, ev); err != nil {
			t.Fatalf("InsertEvent(%d) error = %v", i, err)
		}
		if i == 0 {
			firstEvent = ev
		}
		time.Sleep(2 * time.Millisecond)
	}

	events, err := q.ListEventsAsc(ctx, run.ID, 10, &firstEvent.CreatedAt, firstEvent.ID)
	if err != nil {
		t.Fatalf("ListEventsAsc() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len = %d, want 2", len(events))
	}
}

func TestEvents_ListEventsAsc_EmptyRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	events, err := q.ListEventsAsc(ctx, newID(), 10, nil, "")
	if err != nil {
		t.Fatalf("ListEventsAsc() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("len = %d, want 0", len(events))
	}
}

// --------------------------------------------------------------------------.
// DeleteProject (exercises private deleteProjectRows indirectly).
// --------------------------------------------------------------------------.

func TestProjects_DeleteProject_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-delete-" + newID()
	project := &domain.Project{ID: projectID, Name: "to-delete", OrgID: "org-delete"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if err := q.DeleteProject(ctx, projectID); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	_, err := q.GetProject(ctx, projectID)
	if !errors.Is(err, store.ErrProjectNotFound) {
		t.Fatalf("GetProject() after delete error = %v, want ErrProjectNotFound", err)
	}
}

func TestProjects_DeleteProject_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.DeleteProject(ctx, "nonexistent-project-"+newID())
	if !errors.Is(err, store.ErrProjectNotFound) {
		t.Fatalf("DeleteProject() error = %v, want ErrProjectNotFound", err)
	}
}

func TestProjects_DeleteProject_DisablesJobs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-delete-disables-" + newID()
	project := &domain.Project{ID: projectID, Name: "to-delete", OrgID: "org-delete"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, q, projectID)

	if err := q.DeleteProject(ctx, projectID); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	// Job should still exist but be disabled.
	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() after project delete error = %v", err)
	}
	if got.Enabled {
		t.Fatal("job should be disabled after project delete")
	}
}

// --------------------------------------------------------------------------.
// ListJobsByOrg.
// --------------------------------------------------------------------------.

func TestJobs_ListJobsByOrg_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-list-jobs-" + newID()
	projectID := "project-list-jobs-org-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	for range 3 {
		_ = mustCreateJob(t, ctx, q, projectID)
	}

	jobs, err := q.ListJobsByOrg(ctx, orgID, 100, nil)
	if err != nil {
		t.Fatalf("ListJobsByOrg() error = %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len = %d, want 3", len(jobs))
	}
}

func TestJobs_ListJobsByOrg_EmptyOrg(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	jobs, err := q.ListJobsByOrg(ctx, "nonexistent-org-"+newID(), 100, nil)
	if err != nil {
		t.Fatalf("ListJobsByOrg() error = %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("len = %d, want 0", len(jobs))
	}
}

func TestJobs_ListJobsByOrg_WithCursor(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-list-jobs-cursor-" + newID()
	projectID := "project-list-jobs-cursor-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	for range 3 {
		_ = mustCreateJob(t, ctx, q, projectID)
		time.Sleep(2 * time.Millisecond)
	}

	allJobs, _ := q.ListJobsByOrg(ctx, orgID, 100, nil)
	if len(allJobs) < 2 {
		t.Fatal("need at least 2 jobs for cursor test")
	}

	cursor := allJobs[0].CreatedAt
	paged, err := q.ListJobsByOrg(ctx, orgID, 100, &cursor)
	if err != nil {
		t.Fatalf("ListJobsByOrg(cursor) error = %v", err)
	}
	if len(paged) != len(allJobs)-1 {
		t.Fatalf("paged len = %d, want %d", len(paged), len(allJobs)-1)
	}
}

// --------------------------------------------------------------------------.
// ListRunsByOrg.
// --------------------------------------------------------------------------.

func TestJobs_ListRunsByOrg_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-list-runs-" + newID()
	projectID := "project-list-runs-org-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, q, projectID)
	for range 2 {
		_ = mustCreateRun(t, ctx, q, job)
	}

	runs, err := q.ListRunsByOrg(ctx, orgID, 100, nil)
	if err != nil {
		t.Fatalf("ListRunsByOrg() error = %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len = %d, want 2", len(runs))
	}
}

func TestJobs_ListRunsByOrg_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runs, err := q.ListRunsByOrg(ctx, "nonexistent-org-"+newID(), 100, nil)
	if err != nil {
		t.Fatalf("ListRunsByOrg() error = %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("len = %d, want 0", len(runs))
	}
}

func TestJobs_ListRunsByOrg_ExcludesDeletedProjects(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-list-runs-deleted-" + newID()
	projectID := "project-list-runs-deleted-" + newID()

	project := &domain.Project{ID: projectID, OrgID: orgID, Name: "test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, q, projectID)
	_ = mustCreateRun(t, ctx, q, job)

	if err := q.DeleteProject(ctx, projectID); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	runs, err := q.ListRunsByOrg(ctx, orgID, 100, nil)
	if err != nil {
		t.Fatalf("ListRunsByOrg() error = %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("len = %d, want 0 (project deleted)", len(runs))
	}
}

// --------------------------------------------------------------------------.
// ListEnabledLogDrains.
// --------------------------------------------------------------------------.

func TestLogDrains_ListEnabledLogDrains_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-log-drains-" + newID()
	project := &domain.Project{ID: projectID, Name: "test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	enabled := &domain.LogDrain{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "enabled-drain",
		DrainType:   "http",
		EndpointURL: "https://example.com/logs",
		AuthType:    "bearer",
		Enabled:     true,
	}
	if err := q.CreateLogDrain(ctx, enabled); err != nil {
		t.Fatalf("CreateLogDrain(enabled) error = %v", err)
	}

	disabled := &domain.LogDrain{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "disabled-drain",
		DrainType:   "http",
		EndpointURL: "https://example.com/logs-disabled",
		AuthType:    "bearer",
		Enabled:     false,
	}
	if err := q.CreateLogDrain(ctx, disabled); err != nil {
		t.Fatalf("CreateLogDrain(disabled) error = %v", err)
	}

	drains, err := q.ListEnabledLogDrains(ctx)
	if err != nil {
		t.Fatalf("ListEnabledLogDrains() error = %v", err)
	}

	found := false
	for _, d := range drains {
		if d.ID == disabled.ID {
			t.Fatal("disabled drain should not be in enabled list")
		}
		if d.ID == enabled.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("enabled drain not found in results")
	}
}

func TestLogDrains_ListEnabledLogDrains_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	drains, err := q.ListEnabledLogDrains(ctx)
	if err != nil {
		t.Fatalf("ListEnabledLogDrains() error = %v", err)
	}
	if len(drains) != 0 {
		t.Fatalf("len = %d, want 0", len(drains))
	}
}

func TestLogDrains_ListEnabledLogDrains_AllDisabled(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-log-drains-all-disabled-" + newID()
	project := &domain.Project{ID: projectID, Name: "test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	drain := &domain.LogDrain{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "disabled-only",
		DrainType:   "http",
		EndpointURL: "https://example.com/logs-off",
		AuthType:    "none",
		Enabled:     false,
	}
	if err := q.CreateLogDrain(ctx, drain); err != nil {
		t.Fatalf("CreateLogDrain() error = %v", err)
	}

	drains, err := q.ListEnabledLogDrains(ctx)
	if err != nil {
		t.Fatalf("ListEnabledLogDrains() error = %v", err)
	}
	for _, d := range drains {
		if d.ID == drain.ID {
			t.Fatal("disabled drain should not appear")
		}
	}
}

// Suppress unused import warning.
var _ = testutil.Ptr[string]
