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
	"github.com/stretchr/testify/require"
)

// Helpers local to this file

func covStore(t *testing.T) *store.Queries {
	t.Helper()
	require.False(t, testDB ==
		nil ||
		testDB.Pool ==
			nil)

	return store.New(testDB.Pool)
}

func covClean(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(t, testDB.
		CleanTables(ctx))

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
			require.Failf(t, "test failure",

				"clean %s: %v", tbl, err)
		}
	}
}

func covID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func ptr[T any](v T) *T {
	return new(v)
}

// Workflow operations

func TestBulkCancelWorkflowRuns(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-bulk-cancel-" + covID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-bulk-cancel"),
		Slug:      new("wf-bulk-cancel-" + covID()),
	})

	// Create three runs: two pending (cancellable) and one already completed.
	run1 := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
	run2 := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
	run3 := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
		Status:    ptr(domain.WfStatusCompleted),
	})

	now := time.Now().UTC()
	canceled, err := q.BulkCancelWorkflowRuns(ctx, projectID, []string{run1.ID, run2.ID, run3.ID}, now)
	require.NoError(t, err)
	require.Len(t, canceled,

		2)

	// Only the two pending runs should be canceled.

	// Verify the completed run was not affected.
	got, err := q.GetWorkflowRun(ctx, run3.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusCompleted,
		got.
			Status)

	// Verify a canceled run has the right fields.
	got1, err := q.GetWorkflowRun(ctx, run1.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusCanceled,
		got1.
			Status)
	require.Equal(t, "canceled by user (bulk)",

		got1.Error)

}

func TestBulkCancelWorkflowRuns_SkipsEveryTerminalStatus(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-bulk-cancel-terminal-" + covID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-bulk-cancel-terminal"),
		Slug:      new("wf-bulk-cancel-terminal-" + covID()),
	})

	terminalStatuses := []domain.WorkflowRunStatus{
		domain.WfStatusCompleted,
		domain.WfStatusFailed,
		domain.WfStatusTimedOut,
		domain.WfStatusCanceled,
		domain.WfStatusCompensated,
		domain.WfStatusCompensationFailed,
	}
	ids := make([]string, 0, len(terminalStatuses)+1)
	wantStatusByID := make(map[string]domain.WorkflowRunStatus, len(terminalStatuses))
	for _, status := range terminalStatuses {
		status := status
		run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
			ProjectID: new(projectID),
			Status:    &status,
		})
		if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_runs SET finished_at = $1, error = $2 WHERE id = $3`,
			time.Now().UTC().Add(-time.Hour), "terminal must be immutable", run.ID); err != nil {
			require.Failf(t, "test failure",

				"seed terminal %s: %v", status, err)
		}
		ids = append(ids, run.ID)
		wantStatusByID[run.ID] = status
	}

	runningStatus := domain.WfStatusRunning
	running := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
		Status:    &runningStatus,
	})
	ids = append(ids, running.ID)

	canceled, err := q.BulkCancelWorkflowRuns(ctx, projectID, ids, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, len(canceled) !=
		1 || canceled[0] != running.
		ID)

	for id, wantStatus := range wantStatusByID {
		got, err := q.GetWorkflowRun(ctx, id)
		require.NoError(t, err)
		require.Equal(t, wantStatus,

			got.
				Status)
		require.Equal(t, "terminal must be immutable",

			got.Error)

	}

	gotRunning, err := q.GetWorkflowRun(ctx, running.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusCanceled,
		gotRunning.
			Status)

}

func TestBulkCancelWorkflowRuns_EmptyIDs(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	canceled, err := q.BulkCancelWorkflowRuns(ctx, "nonexistent-project", []string{}, time.Now().UTC())
	require.NoError(t, err)
	require.Len(t, canceled,

		0)

}

func TestListWorkflowRunLabels(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-wf-labels-" + covID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-labels"),
		Slug:      new("wf-labels-" + covID()),
	})
	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})

	// Empty labels initially.
	labels, err := q.ListWorkflowRunLabels(ctx, run.ID)
	require.NoError(t, err)
	require.Len(t, labels,
		0,
	)

	// Create labels.
	want := map[string]string{"env": "staging", "team": "infra"}
	require.NoError(t, q.CreateWorkflowRunLabels(ctx, run.ID, want))

	labels, err = q.ListWorkflowRunLabels(ctx, run.ID)
	require.NoError(t, err)
	require.Len(t, labels,
		2,
	)
	require.False(t, labels["env"] !=
		"staging" ||
		labels["team"] !=
			"infra",
	)
	require.NoError(t, q.CreateWorkflowRunLabels(ctx, run.ID, map[string]string{"env": "production"}))

	// Upsert one label.

	labels, err = q.ListWorkflowRunLabels(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, "production",

		labels["env"],
	)

}

func TestListWorkflowSnapshotsByWorkflow(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-wf-snap-" + covID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-snap"),
		Slug:      new("wf-snap-" + covID()),
	})

	// Create two snapshots with distinct version IDs.
	steps := []domain.WorkflowStep{
		{ID: covID(), WorkflowID: wf.ID, StepRef: "step-a", JobID: covID()},
	}
	wfCopy := *wf
	wfCopy.VersionID = domain.NewVersionID()
	snap1, err := q.GetOrCreateWorkflowSnapshot(ctx, &wfCopy, steps)
	require.NoError(t, err)

	wfCopy2 := *wf
	wfCopy2.VersionID = domain.NewVersionID()
	snap2, err := q.GetOrCreateWorkflowSnapshot(ctx, &wfCopy2, steps)
	require.NoError(t, err)

	snapshots, err := q.ListWorkflowSnapshotsByWorkflow(ctx, wf.ID, 10)
	require.NoError(t, err)
	require.Len(t, snapshots,

		2)
	require.Equal(t, snap2.
		ID,
		snapshots[0].ID)
	require.Equal(t, snap1.
		ID,
		snapshots[1].ID)

	// Newest first.

	// Limit works.
	limited, err := q.ListWorkflowSnapshotsByWorkflow(ctx, wf.ID, 1)
	require.NoError(t, err)
	require.Len(t, limited,

		1)

}

func TestListWorkflowStepDecisions(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-step-decisions-" + covID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-decisions"),
		Slug:      new("wf-decisions-" + covID()),
	})
	jobA := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	stepA := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   new(jobA.ID),
		StepRef: new("step-a"),
	})
	jobB := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	stepB := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   new(jobB.ID),
		StepRef: new("step-b"),
	})
	run := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
	srA := testutil.MustCreateWorkflowStepRun(t, ctx, q, run.ID, stepA.ID, &testutil.WorkflowStepRunOpts{
		StepRef: new("step-a"),
	})
	srB := testutil.MustCreateWorkflowStepRun(t, ctx, q, run.ID, stepB.ID, &testutil.WorkflowStepRunOpts{
		StepRef: new("step-b"),
	})

	d1 := &domain.WorkflowStepDecision{
		WorkflowRunID: run.ID,
		StepRunID:     srA.ID,
		StepRef:       "step-a",
		DecisionType:  "retry",
		Decision:      "retry",
		Explanation:   "transient failure",
	}
	require.NoError(t, q.CreateWorkflowStepDecision(ctx, d1))

	d2 := &domain.WorkflowStepDecision{
		WorkflowRunID: run.ID,
		StepRunID:     srB.ID,
		StepRef:       "step-b",
		DecisionType:  "skip",
		Decision:      "skip",
		Explanation:   "condition not met",
		Details:       json.RawMessage(`{"reason":"missing_input"}`),
	}
	require.NoError(t, q.CreateWorkflowStepDecision(ctx, d2))

	// List all decisions for the run.
	decisions, err := q.ListWorkflowStepDecisions(ctx, run.ID, "", "", 100, nil)
	require.NoError(t, err)
	require.Len(t, decisions,

		2)

	// Filter by step_ref.
	filtered, err := q.ListWorkflowStepDecisions(ctx, run.ID, "step-a", "", 100, nil)
	require.NoError(t, err)
	require.Len(t, filtered,

		1)
	require.Equal(t, "step-a",

		filtered[0].StepRef,
	)

	// Filter by decision_type.
	byType, err := q.ListWorkflowStepDecisions(ctx, run.ID, "", "skip", 100, nil)
	require.NoError(t, err)
	require.Len(t, byType,
		1,
	)
	require.Equal(t, "skip",

		byType[0].DecisionType,
	)

	var detailsMap map[string]string
	require.NoError(t, json.
		Unmarshal(byType[0].
			Details, &detailsMap,
		))
	require.Equal(t, "missing_input",

		detailsMap["reason"])

}

func TestListOrphanedStepRuns(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-orphaned-" + covID()

	// Create a workflow with a step, a run, and a step run.
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("wf-orphaned"),
		Slug:      new("wf-orphaned-" + covID()),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   new(stepJob.ID),
		StepRef: new("step-orphan-" + covID()),
	})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})

	// Create a job run that is finished (completed) and its finished_at is old.
	finishedAt := time.Now().UTC().Add(-5 * time.Minute)
	jobRun := testutil.BuildRun(stepJob, &testutil.RunOpts{
		Status: ptr(domain.StatusCompleted),
	})
	jobRun.FinishedAt = &finishedAt
	jobRun.WorkflowStepRunID = ""
	require.NoError(t, q.CreateRun(ctx,
		jobRun))

	// will be set after step run created

	// Create a step run that is still "running" -- this is orphaned because the
	// job run is already completed.
	stepRun := testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		Status:  ptr(domain.StepRunning),
		StepRef: new(step.StepRef),
	})

	// Link job run to step run.
	_, err := testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET workflow_step_run_id = $1 WHERE id = $2`,
		stepRun.ID, jobRun.ID,
	)
	require.NoError(t, err)

	orphaned, err := q.ListOrphanedStepRuns(ctx)
	require.NoError(t, err)
	require.Len(t, orphaned,

		1)
	require.Equal(t, stepRun.
		ID, orphaned[0].StepRunID,
	)
	require.Equal(t, domain.
		StatusCompleted,
		orphaned[0].JobStatus,
	)

}

// Job operations

func TestDeleteExpiredJobMemory(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-expire-mem-" + covID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})

	// Insert a memory row with an already-expired TTL.
	pastExpiry := time.Now().UTC().Add(-1 * time.Hour)
	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_memory (id, job_id, project_id, memory_key, value, size_bytes, ttl_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, covID(), job.ID, projectID, "expired-key", `"old"`, 3, pastExpiry)
	require.NoError(t, err)

	// Insert a memory row with no TTL (should not be deleted).
	_, err = testDB.Pool.Exec(ctx, `
		INSERT INTO job_memory (id, job_id, project_id, memory_key, value, size_bytes)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, covID(), job.ID, projectID, "no-ttl-key", `"fresh"`, 5)
	require.NoError(t, err)

	deleted, err := q.DeleteExpiredJobMemory(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

	// Verify the non-expired row still exists.
	remaining, err := q.ListJobMemory(ctx, job.ID)
	require.NoError(t, err)
	require.Len(t, remaining,

		1)
	require.Equal(t, "no-ttl-key",

		remaining[0].
			MemoryKey)

}

// Event operations

func TestGetEventTriggerStats(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-trigger-stats-" + covID()

	// Empty stats should return zero counts.
	stats, err := q.GetEventTriggerStats(ctx, projectID, "")
	require.NoError(t, err)
	require.EqualValues(t, 0, stats.
		TotalCount,
	)

	// Create a job + run to link triggers to.
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	run := testutil.BuildRun(job, nil)
	require.NoError(t, q.CreateRun(ctx,
		run))

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
	require.NoError(t, q.CreateEventTrigger(ctx,
		waitTrigger))

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
	require.NoError(t, q.CreateEventTrigger(ctx,
		recvTrigger))

	stats, err = q.GetEventTriggerStats(ctx, projectID, "")
	require.NoError(t, err)
	require.EqualValues(t, 2, stats.
		TotalCount,
	)
	require.EqualValues(t, 1, stats.
		WaitingCount,
	)
	require.EqualValues(t, 1, stats.
		ReceivedCount,
	)

}

// Audit events: StreamAuditEvents

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
	require.NoError(t, q.CreateAuditEvent(ctx, ev1))

	ev2 := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "user-2",
		ActorType:    "user",
		Action:       "job.delete",
		ResourceType: "job",
		ResourceID:   covID(),
		Details:      json.RawMessage(`{}`),
	}
	require.NoError(t, q.CreateAuditEvent(ctx, ev2))

	// Stream all events.
	from := now.Add(-1 * time.Minute)
	to := now.Add(1 * time.Minute)
	var streamed []domain.AuditEvent
	err := q.StreamAuditEvents(ctx, projectID, "", "", from, to, func(ev *domain.AuditEvent) error {
		streamed = append(streamed, *ev)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, streamed,

		2)

	// Filter by actorID.
	var filtered []domain.AuditEvent
	err = q.StreamAuditEvents(ctx, projectID, "user-1", "", from, to, func(ev *domain.AuditEvent) error {
		filtered = append(filtered, *ev)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, filtered,

		1)

	// Callback error propagates.
	wantErr := errors.New("stop early")
	err = q.StreamAuditEvents(ctx, projectID, "", "", from, to, func(_ *domain.AuditEvent) error {
		return wantErr
	})
	require.True(t, errors.Is(err, wantErr))

}

// TestGetAuditEvent verifies tenant-isolated single-event reads.
func TestGetAuditEvent(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectA := "proj-a-" + covID()
	projectB := "proj-b-" + covID()

	ev := &domain.AuditEvent{
		ProjectID:    projectA,
		ActorID:      "user-a",
		ActorType:    "user",
		Action:       "job.create",
		ResourceType: "job",
		ResourceID:   covID(),
		Details:      json.RawMessage(`{}`),
	}
	require.NoError(t, q.CreateAuditEvent(ctx, ev))

	got, err := q.GetAuditEvent(ctx, projectA, ev.ID)
	require.NoError(t, err)
	require.False(t, got.ID !=
		ev.ID ||
		got.ProjectID !=
			projectA,
	)

	// Cross-tenant must surface as ErrAuditEventNotFound, never the row.
	_, err = q.GetAuditEvent(ctx, projectB, ev.ID)
	require.True(t, errors.Is(err, store.
		ErrAuditEventNotFound,
	))

	// Unknown id.
	_, err = q.GetAuditEvent(ctx, projectA, "ev-does-not-exist")
	require.True(t, errors.Is(err, store.
		ErrAuditEventNotFound,
	))

}

// Store utilities

func TestAdvisoryXactLock(t *testing.T) {
	ctx := context.Background()
	_ = covStore(t) // validates testDB is initialized

	// AdvisoryXactLock should succeed inside a transaction (pool-level call
	// also works -- pg_advisory_xact_lock auto-releases at transaction end).
	err := store.WithTx(ctx, testDB.Pool, func(txQ *store.Queries) error {
		return txQ.AdvisoryXactLock(ctx, 42)
	})
	require.NoError(t, err)

}

func TestReindexIndexConcurrently_EmptyName(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)

	err := q.ReindexIndexConcurrently(ctx, "")
	require.Error(t, err)

}

func TestReindexIndexConcurrently_ValidIndex(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)

	// jobs_pkey is a well-known index that always exists after migrations.
	err := q.ReindexIndexConcurrently(ctx, "jobs_pkey")
	require.NoError(t, err)

}

// CRUD: DeleteRunState

func TestDeleteRunState(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-del-state-" + covID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	run := testutil.BuildRun(job, nil)
	require.NoError(t, q.CreateRun(ctx,
		run))

	// Upsert a state entry.
	st := &domain.RunState{
		RunID:    run.ID,
		StateKey: "cursor",
		Value:    json.RawMessage(`"page-3"`),
	}
	require.NoError(t, q.UpsertRunState(ctx, st))

	got, err := q.GetRunState(ctx, run.ID, "cursor")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NoError(t, q.DeleteRunState(ctx, run.
		ID, "cursor"))

	// Delete it.

	got, err = q.GetRunState(ctx, run.ID, "cursor")
	require.NoError(t, err)
	require.Nil(t, got)
	require.NoError(t, q.DeleteRunState(ctx, run.
		ID, "nonexistent",
	))

	// Deleting a non-existent key should not error.

}

// Circuit breaker: RecordEndpointCircuitSuccess

func TestRecordEndpointCircuitSuccess(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	endpoint := "https://example.com/circuit-success-" + covID()

	// Open the circuit first with failures.
	now := time.Now().UTC()
	require.NoError(t, q.RecordEndpointCircuitFailure(ctx, endpoint,
		now, 1,
		2*time.Minute,
	))
	require.NoError(t, q.RecordEndpointCircuitSuccess(ctx, endpoint))

	// Record a success -- circuit should close.

	// Verify it is now allowed.
	allowed, _, err := q.CanDispatchEndpoint(ctx, endpoint, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, allowed)

	// Calling on a new endpoint should also work (upsert).
	newEndpoint := "https://example.com/circuit-new-" + covID()
	require.NoError(t, q.RecordEndpointCircuitSuccess(ctx, newEndpoint))

}

// Webhook operations

func TestGetWebhookSubscription(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-webhook-sub-" + covID()

	// Not found.
	_, err := q.GetWebhookSubscription(ctx, "nonexistent-id")
	require.True(t, errors.Is(err, store.
		ErrWebhookSubscriptionNotFound,
	))

	// Create a subscription.
	sub := &domain.WebhookSubscription{
		ProjectID:  projectID,
		WebhookURL: "https://example.com/hooks",
		EventTypes: []string{"run.completed", "run.failed"},
		Secret:     "whsec_test123",
		Active:     true,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, sub))

	got, err := q.GetWebhookSubscription(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, sub.ID,

		got.ID)
	require.Equal(t, "https://example.com/hooks",

		got.WebhookURL)
	require.Equal(t, "whsec_test123",

		got.Secret,
	)
	require.True(t, got.Active)

}

func TestResetStuckWebhookDeliveries(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-stuck-webhooks-" + covID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	run := testutil.BuildRun(job, nil)
	require.NoError(t, q.CreateRun(ctx,
		run))

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
	require.NoError(t, q.CreateWebhookDelivery(ctx,
		stuckDelivery,
	))

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
	require.NoError(t, q.CreateWebhookDelivery(ctx,
		recentDelivery,
	))

	reset, err := q.ResetStuckWebhookDeliveries(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, reset)

}

// Health: AtomicRecordHealthResult

func TestAtomicRecordHealthResult(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	endpoint := "https://example.com/health-" + covID()

	// First call inserts a new row.
	result, err := q.AtomicRecordHealthResult(ctx,
		endpoint,
		1.0, 0.0, 1.0, // success, timeout, latency signals
		0.3,           // alpha (EMA weight)
		0.5, 0.3, 0.2, // weights: success, timeout, latency
		42.0, // last latency ms
	)
	require.NoError(t, err)
	require.Equal(t, endpoint,

		result.
			EndpointURL,
	)
	require.EqualValues(t, 1, result.
		TotalRequests,
	)
	require.EqualValues(t, 42.0,
		result.
			LastLatencyMs,
	)

	// Second call should update (upsert).
	result2, err := q.AtomicRecordHealthResult(ctx,
		endpoint,
		1.0, 0.0, 0.8,
		0.3,
		0.5, 0.3, 0.2,
		55.0,
	)
	require.NoError(t, err)
	require.EqualValues(t, 2, result2.
		TotalRequests,
	)
	require.EqualValues(t, 55.0,
		result2.
			LastLatencyMs,
	)
	require.False(t, result2.
		HealthScore <
		0 ||
		result2.HealthScore >
			100)

}

// SLO: PruneSLOEvaluations

func TestPruneSLOEvaluations(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	projectID := "proj-slo-prune-" + covID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})

	slo := &domain.JobSLO{
		ID:          covID(),
		JobID:       job.ID,
		ProjectID:   projectID,
		Metric:      "success_rate",
		Target:      99.5,
		WindowHours: 24,
	}
	require.NoError(t, q.CreateJobSLO(ctx, slo))

	// Insert 5 evaluations.
	for i := range 5 {
		eval := &domain.JobSLOEvaluation{
			ID:              covID(),
			SLOID:           slo.ID,
			CurrentValue:    99.0 + float64(i)*0.1,
			BudgetRemaining: 0.5 - float64(i)*0.1,
			EvaluatedAt:     time.Now().UTC().Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, q.InsertSLOEvaluation(ctx,
			eval))

	}

	// Prune to keep 2.
	pruned, err := q.PruneSLOEvaluations(ctx, 2)
	require.NoError(t, err)
	require.EqualValues(t, 3, pruned)

	// Prune again with keepPerSLO=2 should delete nothing.
	pruned2, err := q.PruneSLOEvaluations(ctx, 2)
	require.NoError(t, err)
	require.EqualValues(t, 0, pruned2)

}

// GetCostOutliers.

func TestGetCostOutliers_EmptyResult(t *testing.T) {
	ctx := context.Background()
	q := covStore(t)
	covClean(t, ctx)

	from := time.Now().UTC().Add(-24 * time.Hour)
	to := time.Now().UTC()

	outliers, err := q.GetCostOutliers(ctx, "nonexistent-project", from, to, 2.0)
	require.NoError(t, err)
	require.Len(t, outliers,

		0)

}

// ScanAll generic helper

func TestScanAll(t *testing.T) {
	ctx := context.Background()
	covClean(t, ctx)

	q := covStore(t)

	// Create two jobs.
	testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new("proj-scanall")})
	testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new("proj-scanall")})

	type idRow struct {
		ID string `db:"id"`
	}

	results, err := store.ScanAll[idRow](ctx, testDB.Pool,
		"SELECT id FROM jobs WHERE project_id = $1 ORDER BY id", "proj-scanall")
	require.NoError(t, err)
	require.Len(t, results,

		2)
	require.False(t, results[0].ID ==
		"" || results[1].ID == "")

}
