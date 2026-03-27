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

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Helpers local to this file
// ---------------------------------------------------------------------------

func covStore(t *testing.T) *store.Queries {
	t.Helper()
	if testDB == nil || testDB.Pool == nil {
		t.Fatal("testDB is not initialized")
	}
	return store.New(testDB.Pool)
}

func covClean(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := testDB.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	// Additional tables not covered by CleanTables.
	for _, tbl := range []string{
		"workflow_step_decisions",
		"workflow_snapshots",
		"endpoint_health_scores",
		"job_slo_evaluations",
		"job_slos",
		"run_state",
		"job_memory",
	} {
		if _, err := testDB.Pool.Exec(ctx, "DELETE FROM "+tbl); err != nil {
			t.Fatalf("clean %s: %v", tbl, err)
		}
	}
}

func covID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func ptr[T any](v T) *T {
	return &v
}

// ---------------------------------------------------------------------------
// Workflow operations
// ---------------------------------------------------------------------------

func TestBulkCancelWorkflowRuns(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-bulk-cancel-" + covID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: ptr(projectID),
		Name:      ptr("wf-bulk-cancel"),
		Slug:      ptr("wf-bulk-cancel-" + covID()),
	})

	// Create three runs: two pending (cancellable) and one already completed.
	run1 := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: ptr(projectID)})
	run2 := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: ptr(projectID)})
	run3 := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: ptr(projectID),
		Status:    ptr(domain.WfStatusCompleted),
	})

	now := time.Now().UTC()
	canceled, err := q.BulkCancelWorkflowRuns(ctx, projectID, []string{run1.ID, run2.ID, run3.ID}, now)
	if err != nil {
		t.Fatalf("BulkCancelWorkflowRuns() error = %v", err)
	}

	// Only the two pending runs should be canceled.
	if len(canceled) != 2 {
		t.Fatalf("BulkCancelWorkflowRuns() canceled %d, want 2", len(canceled))
	}

	// Verify the completed run was not affected.
	got, err := q.GetWorkflowRun(ctx, run3.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}
	if got.Status != domain.WfStatusCompleted {
		t.Fatalf("completed run status = %q, want %q", got.Status, domain.WfStatusCompleted)
	}

	// Verify a canceled run has the right fields.
	got1, err := q.GetWorkflowRun(ctx, run1.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}
	if got1.Status != domain.WfStatusFailed {
		t.Fatalf("canceled run status = %q, want %q", got1.Status, domain.WfStatusFailed)
	}
	if got1.Error != "canceled by user (bulk)" {
		t.Fatalf("canceled run error = %q, want 'canceled by user (bulk)'", got1.Error)
	}
}

func TestBulkCancelWorkflowRuns_EmptyIDs(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	canceled, err := q.BulkCancelWorkflowRuns(ctx, "nonexistent-project", []string{}, time.Now().UTC())
	if err != nil {
		t.Fatalf("BulkCancelWorkflowRuns() error = %v", err)
	}
	if len(canceled) != 0 {
		t.Fatalf("expected 0 canceled, got %d", len(canceled))
	}
}

func TestListWorkflowRunLabels(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-wf-labels-" + covID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: ptr(projectID),
		Name:      ptr("wf-labels"),
		Slug:      ptr("wf-labels-" + covID()),
	})
	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: ptr(projectID)})

	// Empty labels initially.
	labels, err := q.ListWorkflowRunLabels(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListWorkflowRunLabels() error = %v", err)
	}
	if len(labels) != 0 {
		t.Fatalf("expected 0 labels, got %d", len(labels))
	}

	// Create labels.
	want := map[string]string{"env": "staging", "team": "infra"}
	if err := q.CreateWorkflowRunLabels(ctx, run.ID, want); err != nil {
		t.Fatalf("CreateWorkflowRunLabels() error = %v", err)
	}

	labels, err = q.ListWorkflowRunLabels(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListWorkflowRunLabels() error = %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}
	if labels["env"] != "staging" || labels["team"] != "infra" {
		t.Fatalf("labels mismatch: got %v", labels)
	}

	// Upsert one label.
	if err := q.CreateWorkflowRunLabels(ctx, run.ID, map[string]string{"env": "production"}); err != nil {
		t.Fatalf("CreateWorkflowRunLabels(upsert) error = %v", err)
	}
	labels, err = q.ListWorkflowRunLabels(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListWorkflowRunLabels() after upsert error = %v", err)
	}
	if labels["env"] != "production" {
		t.Fatalf("expected env=production, got %q", labels["env"])
	}
}

func TestListWorkflowSnapshotsByWorkflow(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-wf-snap-" + covID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: ptr(projectID),
		Name:      ptr("wf-snap"),
		Slug:      ptr("wf-snap-" + covID()),
	})

	// Create two snapshots with distinct version IDs.
	steps := []domain.WorkflowStep{
		{ID: covID(), WorkflowID: wf.ID, StepRef: "step-a", JobID: covID()},
	}
	wfCopy := *wf
	wfCopy.VersionID = domain.NewVersionID()
	snap1, err := q.GetOrCreateWorkflowSnapshot(ctx, &wfCopy, steps)
	if err != nil {
		t.Fatalf("GetOrCreateWorkflowSnapshot(1) error = %v", err)
	}

	wfCopy2 := *wf
	wfCopy2.VersionID = domain.NewVersionID()
	snap2, err := q.GetOrCreateWorkflowSnapshot(ctx, &wfCopy2, steps)
	if err != nil {
		t.Fatalf("GetOrCreateWorkflowSnapshot(2) error = %v", err)
	}

	snapshots, err := q.ListWorkflowSnapshotsByWorkflow(ctx, wf.ID, 10)
	if err != nil {
		t.Fatalf("ListWorkflowSnapshotsByWorkflow() error = %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}

	// Newest first.
	if snapshots[0].ID != snap2.ID {
		t.Fatalf("expected first snapshot ID = %q, got %q", snap2.ID, snapshots[0].ID)
	}
	if snapshots[1].ID != snap1.ID {
		t.Fatalf("expected second snapshot ID = %q, got %q", snap1.ID, snapshots[1].ID)
	}

	// Limit works.
	limited, err := q.ListWorkflowSnapshotsByWorkflow(ctx, wf.ID, 1)
	if err != nil {
		t.Fatalf("ListWorkflowSnapshotsByWorkflow(limit=1) error = %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected 1 snapshot with limit=1, got %d", len(limited))
	}
}

func TestListWorkflowStepDecisions(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-step-decisions-" + covID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: ptr(projectID),
		Name:      ptr("wf-decisions"),
		Slug:      ptr("wf-decisions-" + covID()),
	})
	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: ptr(projectID)})

	d1 := &domain.WorkflowStepDecision{
		WorkflowRunID: run.ID,
		StepRunID:     covID(),
		StepRef:       "step-a",
		DecisionType:  "retry",
		Decision:      "retry",
		Explanation:   "transient failure",
	}
	if err := q.CreateWorkflowStepDecision(ctx, d1); err != nil {
		t.Fatalf("CreateWorkflowStepDecision(1) error = %v", err)
	}

	d2 := &domain.WorkflowStepDecision{
		WorkflowRunID: run.ID,
		StepRunID:     covID(),
		StepRef:       "step-b",
		DecisionType:  "skip",
		Decision:      "skip",
		Explanation:   "condition not met",
		Details:       json.RawMessage(`{"reason":"missing_input"}`),
	}
	if err := q.CreateWorkflowStepDecision(ctx, d2); err != nil {
		t.Fatalf("CreateWorkflowStepDecision(2) error = %v", err)
	}

	// List all decisions for the run.
	decisions, err := q.ListWorkflowStepDecisions(ctx, run.ID, "", "", 100, nil)
	if err != nil {
		t.Fatalf("ListWorkflowStepDecisions() error = %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}

	// Filter by step_ref.
	filtered, err := q.ListWorkflowStepDecisions(ctx, run.ID, "step-a", "", 100, nil)
	if err != nil {
		t.Fatalf("ListWorkflowStepDecisions(step_ref) error = %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 decision for step-a, got %d", len(filtered))
	}
	if filtered[0].StepRef != "step-a" {
		t.Fatalf("expected step_ref=step-a, got %q", filtered[0].StepRef)
	}

	// Filter by decision_type.
	byType, err := q.ListWorkflowStepDecisions(ctx, run.ID, "", "skip", 100, nil)
	if err != nil {
		t.Fatalf("ListWorkflowStepDecisions(decision_type) error = %v", err)
	}
	if len(byType) != 1 {
		t.Fatalf("expected 1 skip decision, got %d", len(byType))
	}
	if byType[0].DecisionType != "skip" {
		t.Fatalf("expected decision_type=skip, got %q", byType[0].DecisionType)
	}
	if string(byType[0].Details) != `{"reason":"missing_input"}` {
		t.Fatalf("unexpected details: %s", string(byType[0].Details))
	}
}

func TestListOrphanedStepRuns(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-orphaned-" + covID()

	// Create a workflow with a step, a run, and a step run.
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: ptr(projectID),
		Name:      ptr("wf-orphaned"),
		Slug:      ptr("wf-orphaned-" + covID()),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: ptr(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   ptr(stepJob.ID),
		StepRef: ptr("step-orphan-" + covID()),
	})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: ptr(projectID)})

	// Create a job run that is finished (completed) and its finished_at is old.
	finishedAt := time.Now().UTC().Add(-5 * time.Minute)
	jobRun := testutil.BuildRun(stepJob, &testutil.RunOpts{
		Status: ptr(domain.StatusCompleted),
	})
	jobRun.FinishedAt = &finishedAt
	jobRun.WorkflowStepRunID = "" // will be set after step run created
	if err := q.CreateRun(ctx, jobRun); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	// Create a step run that is still "running" -- this is orphaned because the
	// job run is already completed.
	stepRun := testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		Status:  ptr(domain.StepRunning),
		StepRef: ptr(step.StepRef),
	})

	// Link job run to step run.
	_, err := testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET workflow_step_run_id = $1 WHERE id = $2`,
		stepRun.ID, jobRun.ID,
	)
	if err != nil {
		t.Fatalf("link job_run to step_run: %v", err)
	}

	orphaned, err := q.ListOrphanedStepRuns(ctx)
	if err != nil {
		t.Fatalf("ListOrphanedStepRuns() error = %v", err)
	}

	if len(orphaned) != 1 {
		t.Fatalf("expected 1 orphaned step run, got %d", len(orphaned))
	}
	if orphaned[0].StepRunID != stepRun.ID {
		t.Fatalf("orphaned step run ID = %q, want %q", orphaned[0].StepRunID, stepRun.ID)
	}
	if orphaned[0].JobStatus != domain.StatusCompleted {
		t.Fatalf("orphaned job status = %q, want %q", orphaned[0].JobStatus, domain.StatusCompleted)
	}
}

// ---------------------------------------------------------------------------
// Job operations
// ---------------------------------------------------------------------------

func TestDeleteExpiredJobMemory(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-expire-mem-" + covID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: ptr(projectID)})

	// Insert a memory row with an already-expired TTL.
	pastExpiry := time.Now().UTC().Add(-1 * time.Hour)
	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_memory (id, job_id, project_id, memory_key, value, size_bytes, ttl_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, covID(), job.ID, projectID, "expired-key", `"old"`, 3, pastExpiry)
	if err != nil {
		t.Fatalf("insert expired memory: %v", err)
	}

	// Insert a memory row with no TTL (should not be deleted).
	_, err = testDB.Pool.Exec(ctx, `
		INSERT INTO job_memory (id, job_id, project_id, memory_key, value, size_bytes)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, covID(), job.ID, projectID, "no-ttl-key", `"fresh"`, 5)
	if err != nil {
		t.Fatalf("insert non-expired memory: %v", err)
	}

	deleted, err := q.DeleteExpiredJobMemory(ctx)
	if err != nil {
		t.Fatalf("DeleteExpiredJobMemory() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	// Verify the non-expired row still exists.
	remaining, err := q.ListJobMemory(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListJobMemory() error = %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(remaining))
	}
	if remaining[0].MemoryKey != "no-ttl-key" {
		t.Fatalf("remaining key = %q, want no-ttl-key", remaining[0].MemoryKey)
	}
}

// ---------------------------------------------------------------------------
// Event operations
// ---------------------------------------------------------------------------

func TestGetEventTriggerStats(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-trigger-stats-" + covID()

	// Empty stats should return zero counts.
	stats, err := q.GetEventTriggerStats(ctx, projectID)
	if err != nil {
		t.Fatalf("GetEventTriggerStats() error = %v", err)
	}
	if stats.TotalCount != 0 {
		t.Fatalf("total = %d, want 0", stats.TotalCount)
	}

	// Create a job + run to link triggers to.
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: ptr(projectID)})
	run := testutil.BuildRun(job, nil)
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	now := time.Now().UTC()
	expiresAt := now.Add(5 * time.Minute)

	// Create two triggers: one waiting and one received.
	waitTrigger := &domain.EventTrigger{
		ID:          covID(),
		EventKey:    "order.created",
		ProjectID:   projectID,
		SourceType:  "job_run",
		JobRunID:    run.ID,
		Status:      "waiting",
		RequestedAt: now,
		ExpiresAt:   expiresAt,
	}
	if err := q.CreateEventTrigger(ctx, waitTrigger); err != nil {
		t.Fatalf("CreateEventTrigger(waiting) error = %v", err)
	}

	recvTrigger := &domain.EventTrigger{
		ID:          covID(),
		EventKey:    "order.shipped",
		ProjectID:   projectID,
		SourceType:  "job_run",
		JobRunID:    run.ID,
		Status:      "received",
		RequestedAt: now,
		ReceivedAt:  &now,
		ExpiresAt:   expiresAt,
	}
	if err := q.CreateEventTrigger(ctx, recvTrigger); err != nil {
		t.Fatalf("CreateEventTrigger(received) error = %v", err)
	}

	stats, err = q.GetEventTriggerStats(ctx, projectID)
	if err != nil {
		t.Fatalf("GetEventTriggerStats() error = %v", err)
	}
	if stats.TotalCount != 2 {
		t.Fatalf("total = %d, want 2", stats.TotalCount)
	}
	if stats.WaitingCount != 1 {
		t.Fatalf("waiting = %d, want 1", stats.WaitingCount)
	}
	if stats.ReceivedCount != 1 {
		t.Fatalf("received = %d, want 1", stats.ReceivedCount)
	}
}

// ---------------------------------------------------------------------------
// Audit events: StreamAuditEvents
// ---------------------------------------------------------------------------

func TestStreamAuditEvents(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-stream-audit-" + covID()

	now := time.Now().UTC()

	ev1 := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "user-1",
		ActorType:    "user",
		Action:       "job.create",
		ResourceType: "job",
		ResourceID:   covID(),
		Details:      json.RawMessage(`{}`),
	}
	if err := q.CreateAuditEvent(ctx, ev1); err != nil {
		t.Fatalf("CreateAuditEvent(1) error = %v", err)
	}

	ev2 := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "user-2",
		ActorType:    "user",
		Action:       "job.delete",
		ResourceType: "job",
		ResourceID:   covID(),
		Details:      json.RawMessage(`{}`),
	}
	if err := q.CreateAuditEvent(ctx, ev2); err != nil {
		t.Fatalf("CreateAuditEvent(2) error = %v", err)
	}

	// Stream all events.
	from := now.Add(-1 * time.Minute)
	to := now.Add(1 * time.Minute)
	var streamed []domain.AuditEvent
	err := q.StreamAuditEvents(ctx, projectID, "", "", from, to, func(ev *domain.AuditEvent) error {
		streamed = append(streamed, *ev)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamAuditEvents() error = %v", err)
	}
	if len(streamed) != 2 {
		t.Fatalf("expected 2 streamed events, got %d", len(streamed))
	}

	// Filter by actorID.
	var filtered []domain.AuditEvent
	err = q.StreamAuditEvents(ctx, projectID, "user-1", "", from, to, func(ev *domain.AuditEvent) error {
		filtered = append(filtered, *ev)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamAuditEvents(actorID) error = %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered event, got %d", len(filtered))
	}

	// Callback error propagates.
	wantErr := errors.New("stop early")
	err = q.StreamAuditEvents(ctx, projectID, "", "", from, to, func(_ *domain.AuditEvent) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("StreamAuditEvents() error = %v, want %v", err, wantErr)
	}
}

// ---------------------------------------------------------------------------
// Store utilities
// ---------------------------------------------------------------------------

func TestAdvisoryXactLock(t *testing.T) {
	ctx := context.Background()
	_ = covStore(t) // validates testDB is initialized

	// AdvisoryXactLock should succeed inside a transaction (pool-level call
	// also works -- pg_advisory_xact_lock auto-releases at transaction end).
	err := store.WithTx(ctx, testDB.Pool, func(txQ *store.Queries) error {
		return txQ.AdvisoryXactLock(ctx, 42)
	})
	if err != nil {
		t.Fatalf("AdvisoryXactLock() error = %v", err)
	}
}

func TestReindexIndexConcurrently_EmptyName(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)

	err := q.ReindexIndexConcurrently(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty index name")
	}
}

func TestReindexIndexConcurrently_ValidIndex(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)

	// jobs_pkey is a well-known index that always exists after migrations.
	err := q.ReindexIndexConcurrently(ctx, "jobs_pkey")
	if err != nil {
		t.Fatalf("ReindexIndexConcurrently(jobs_pkey) error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Cost / analytics
// ---------------------------------------------------------------------------

func TestGetCostByMachine_ReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)

	// GetCostByMachine is a stub that returns empty results.
	result, err := q.GetCostByMachine(ctx, "any-project", time.Now().Add(-24*time.Hour), time.Now())
	if err != nil {
		t.Fatalf("GetCostByMachine() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %d", len(result))
	}
}

func TestListActiveJobIDs(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	// No jobs -- empty result.
	ids, err := q.ListActiveJobIDs(ctx)
	if err != nil {
		t.Fatalf("ListActiveJobIDs() error = %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 IDs, got %d", len(ids))
	}

	// Create one enabled and one disabled job.
	enabled := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: ptr("proj-active-ids")})
	disabled := testutil.BuildJob(&testutil.JobOpts{ProjectID: ptr("proj-active-ids")})
	disabled.Enabled = false
	if err := q.CreateJob(ctx, disabled); err != nil {
		t.Fatalf("CreateJob(disabled) error = %v", err)
	}

	ids, err = q.ListActiveJobIDs(ctx)
	if err != nil {
		t.Fatalf("ListActiveJobIDs() error = %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 active ID, got %d", len(ids))
	}
	if ids[0] != enabled.ID {
		t.Fatalf("active ID = %q, want %q", ids[0], enabled.ID)
	}
}

// ---------------------------------------------------------------------------
// CRUD: DeleteRunState
// ---------------------------------------------------------------------------

func TestDeleteRunState(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-del-state-" + covID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: ptr(projectID)})
	run := testutil.BuildRun(job, nil)
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	// Upsert a state entry.
	st := &domain.RunState{
		RunID:    run.ID,
		StateKey: "cursor",
		Value:    json.RawMessage(`"page-3"`),
	}
	if err := q.UpsertRunState(ctx, st); err != nil {
		t.Fatalf("UpsertRunState() error = %v", err)
	}

	// Verify it exists.
	got, err := q.GetRunState(ctx, run.ID, "cursor")
	if err != nil {
		t.Fatalf("GetRunState() error = %v", err)
	}
	if got == nil {
		t.Fatal("expected run state, got nil")
	}

	// Delete it.
	if err := q.DeleteRunState(ctx, run.ID, "cursor"); err != nil {
		t.Fatalf("DeleteRunState() error = %v", err)
	}

	// Verify it is gone.
	got, err = q.GetRunState(ctx, run.ID, "cursor")
	if err != nil {
		t.Fatalf("GetRunState() after delete error = %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete, got value")
	}

	// Deleting a non-existent key should not error.
	if err := q.DeleteRunState(ctx, run.ID, "nonexistent"); err != nil {
		t.Fatalf("DeleteRunState(nonexistent) error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Circuit breaker: RecordEndpointCircuitSuccess
// ---------------------------------------------------------------------------

func TestRecordEndpointCircuitSuccess(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	endpoint := "https://example.com/circuit-success-" + covID()

	// Open the circuit first with failures.
	now := time.Now().UTC()
	if err := q.RecordEndpointCircuitFailure(ctx, endpoint, now, 1, 2*time.Minute); err != nil {
		t.Fatalf("RecordEndpointCircuitFailure() error = %v", err)
	}

	// Record a success -- circuit should close.
	if err := q.RecordEndpointCircuitSuccess(ctx, endpoint); err != nil {
		t.Fatalf("RecordEndpointCircuitSuccess() error = %v", err)
	}

	// Verify it is now allowed.
	allowed, _, err := q.CanDispatchEndpoint(ctx, endpoint, now.Add(time.Second))
	if err != nil {
		t.Fatalf("CanDispatchEndpoint() error = %v", err)
	}
	if !allowed {
		t.Fatal("expected dispatch to be allowed after circuit success")
	}

	// Calling on a new endpoint should also work (upsert).
	newEndpoint := "https://example.com/circuit-new-" + covID()
	if err := q.RecordEndpointCircuitSuccess(ctx, newEndpoint); err != nil {
		t.Fatalf("RecordEndpointCircuitSuccess(new) error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Webhook operations
// ---------------------------------------------------------------------------

func TestGetWebhookSubscription(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-webhook-sub-" + covID()

	// Not found.
	_, err := q.GetWebhookSubscription(ctx, "nonexistent-id")
	if !errors.Is(err, store.ErrWebhookSubscriptionNotFound) {
		t.Fatalf("GetWebhookSubscription(nonexistent) error = %v, want ErrWebhookSubscriptionNotFound", err)
	}

	// Create a subscription.
	sub := &domain.WebhookSubscription{
		ProjectID:  projectID,
		WebhookURL: "https://example.com/hooks",
		EventTypes: []string{"run.completed", "run.failed"},
		Secret:     "whsec_test123",
		Active:     true,
	}
	if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateWebhookSubscription() error = %v", err)
	}

	got, err := q.GetWebhookSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetWebhookSubscription() error = %v", err)
	}
	if got.ID != sub.ID {
		t.Fatalf("ID = %q, want %q", got.ID, sub.ID)
	}
	if got.WebhookURL != "https://example.com/hooks" {
		t.Fatalf("WebhookURL = %q, want %q", got.WebhookURL, "https://example.com/hooks")
	}
	if got.Secret != "whsec_test123" {
		t.Fatalf("Secret = %q, want %q", got.Secret, "whsec_test123")
	}
	if !got.Active {
		t.Fatal("expected Active = true")
	}
}

func TestResetStuckWebhookDeliveries(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-stuck-webhooks-" + covID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: ptr(projectID)})
	run := testutil.BuildRun(job, nil)
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	// Create a stuck delivery: pending with a next_retry_at more than 5 minutes ago.
	stuckRetryAt := time.Now().UTC().Add(-10 * time.Minute)
	stuckDelivery := &domain.WebhookDelivery{
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/hook",
		Status:      domain.WebhookStatusPending,
		Attempts:    1,
		MaxAttempts: 5,
		NextRetryAt: &stuckRetryAt,
	}
	if err := q.CreateWebhookDelivery(ctx, stuckDelivery); err != nil {
		t.Fatalf("CreateWebhookDelivery(stuck) error = %v", err)
	}

	// Create a non-stuck delivery: pending but recent next_retry_at.
	recentRetryAt := time.Now().UTC().Add(-1 * time.Minute)
	recentDelivery := &domain.WebhookDelivery{
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/hook2",
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &recentRetryAt,
	}
	if err := q.CreateWebhookDelivery(ctx, recentDelivery); err != nil {
		t.Fatalf("CreateWebhookDelivery(recent) error = %v", err)
	}

	reset, err := q.ResetStuckWebhookDeliveries(ctx)
	if err != nil {
		t.Fatalf("ResetStuckWebhookDeliveries() error = %v", err)
	}
	if reset != 1 {
		t.Fatalf("reset = %d, want 1", reset)
	}
}

// ---------------------------------------------------------------------------
// Health: AtomicRecordHealthResult
// ---------------------------------------------------------------------------

func TestAtomicRecordHealthResult(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	endpoint := "https://example.com/health-" + covID()

	// First call inserts a new row.
	result, err := q.AtomicRecordHealthResult(ctx,
		endpoint,
		1.0, 0.0, 1.0, // success, timeout, latency signals
		0.3,            // alpha (EMA weight)
		0.5, 0.3, 0.2, // weights: success, timeout, latency
		42.0, // last latency ms
	)
	if err != nil {
		t.Fatalf("AtomicRecordHealthResult() first call error = %v", err)
	}
	if result.EndpointURL != endpoint {
		t.Fatalf("endpoint = %q, want %q", result.EndpointURL, endpoint)
	}
	if result.TotalRequests != 1 {
		t.Fatalf("total_requests = %d, want 1", result.TotalRequests)
	}
	if result.LastLatencyMs != 42.0 {
		t.Fatalf("last_latency_ms = %f, want 42.0", result.LastLatencyMs)
	}

	// Second call should update (upsert).
	result2, err := q.AtomicRecordHealthResult(ctx,
		endpoint,
		1.0, 0.0, 0.8,
		0.3,
		0.5, 0.3, 0.2,
		55.0,
	)
	if err != nil {
		t.Fatalf("AtomicRecordHealthResult() second call error = %v", err)
	}
	if result2.TotalRequests != 2 {
		t.Fatalf("total_requests = %d, want 2", result2.TotalRequests)
	}
	if result2.LastLatencyMs != 55.0 {
		t.Fatalf("last_latency_ms = %f, want 55.0", result2.LastLatencyMs)
	}
	if result2.HealthScore < 0 || result2.HealthScore > 100 {
		t.Fatalf("health_score = %f, expected 0..100", result2.HealthScore)
	}
}

// ---------------------------------------------------------------------------
// SLO: PruneSLOEvaluations
// ---------------------------------------------------------------------------

func TestPruneSLOEvaluations(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-slo-prune-" + covID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: ptr(projectID)})

	slo := &domain.JobSLO{
		ID:          covID(),
		JobID:       job.ID,
		ProjectID:   projectID,
		Metric:      "success_rate",
		Target:      99.5,
		WindowHours: 24,
	}
	if err := q.CreateJobSLO(ctx, slo); err != nil {
		t.Fatalf("CreateJobSLO() error = %v", err)
	}

	// Insert 5 evaluations.
	for i := 0; i < 5; i++ {
		eval := &domain.JobSLOEvaluation{
			ID:              covID(),
			SLOID:           slo.ID,
			CurrentValue:    99.0 + float64(i)*0.1,
			BudgetRemaining: 0.5 - float64(i)*0.1,
			EvaluatedAt:     time.Now().UTC().Add(time.Duration(i) * time.Second),
		}
		if err := q.InsertSLOEvaluation(ctx, eval); err != nil {
			t.Fatalf("InsertSLOEvaluation(%d) error = %v", i, err)
		}
	}

	// Prune to keep 2.
	pruned, err := q.PruneSLOEvaluations(ctx, 2)
	if err != nil {
		t.Fatalf("PruneSLOEvaluations() error = %v", err)
	}
	if pruned != 3 {
		t.Fatalf("pruned = %d, want 3", pruned)
	}

	// Prune again with keepPerSLO=2 should delete nothing.
	pruned2, err := q.PruneSLOEvaluations(ctx, 2)
	if err != nil {
		t.Fatalf("PruneSLOEvaluations() second call error = %v", err)
	}
	if pruned2 != 0 {
		t.Fatalf("expected 0 pruned on second call, got %d", pruned2)
	}
}

// ---------------------------------------------------------------------------
// GetCostOutliers (requires run_usage + job_runs)
// ---------------------------------------------------------------------------

func TestGetCostOutliers_EmptyResult(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	from := time.Now().UTC().Add(-24 * time.Hour)
	to := time.Now().UTC()

	outliers, err := q.GetCostOutliers(ctx, "nonexistent-project", from, to, 2.0)
	if err != nil {
		t.Fatalf("GetCostOutliers() error = %v", err)
	}
	if len(outliers) != 0 {
		t.Fatalf("expected 0 outliers, got %d", len(outliers))
	}
}

// ---------------------------------------------------------------------------
// ScanAll generic helper
// ---------------------------------------------------------------------------

func TestScanAll(t *testing.T) {
	ctx := context.Background()
	covClean(t, ctx)

	q := covStore(t)

	// Create two jobs.
	testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: ptr("proj-scanall")})
	testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: ptr("proj-scanall")})

	type idRow struct {
		ID string `db:"id"`
	}

	results, err := store.ScanAll[idRow](ctx, testDB.Pool,
		"SELECT id FROM jobs WHERE project_id = $1 ORDER BY id", "proj-scanall")
	if err != nil {
		t.Fatalf("ScanAll() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID == "" || results[1].ID == "" {
		t.Fatal("expected non-empty IDs")
	}
}
