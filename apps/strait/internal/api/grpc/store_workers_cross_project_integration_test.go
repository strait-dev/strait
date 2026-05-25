//go:build integration

package grpc

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

// TestIntegration_RegisterWorker_SameIDAcrossProjectsCreatesSeparateRows
// proves worker IDs are tenant-local in the persistent workers table. A
// project must not be able to squat on a common worker ID and block another
// project from registering the same name.
func TestIntegration_RegisterWorker_SameIDAcrossProjectsCreatesSeparateRows(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)

	const workerID = "shared-id"
	const projectA = "proj-A"
	const projectB = "proj-B"

	// Project A registers first.
	wA := &domain.Worker{
		ID:        workerID,
		ProjectID: projectA,
		QueueName: "queue-a",
		Hostname:  "host-a",
		Version:   "1.0.0",
		Status:    domain.WorkerStatusActive,
	}
	if err := q.RegisterWorker(ctx, wA); err != nil {
		t.Fatalf("register A: %v", err)
	}

	wB := &domain.Worker{
		ID:        workerID,
		ProjectID: projectB,
		QueueName: "queue-b",
		Hostname:  "host-b",
		Version:   "9.9.9",
		Status:    domain.WorkerStatusDraining,
	}
	if err := q.RegisterWorker(ctx, wB); err != nil {
		t.Fatalf("register B: %v", err)
	}

	rows, err := env.DB.Pool.Query(ctx,
		`SELECT project_id, queue_name, hostname, version, status
		 FROM workers
		 WHERE id = $1
		 ORDER BY project_id`,
		workerID,
	)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	defer rows.Close()

	type workerRow struct {
		projectID string
		queue     string
		hostname  string
		version   string
		status    string
	}
	var got []workerRow
	for rows.Next() {
		var row workerRow
		if err := rows.Scan(&row.projectID, &row.queue, &row.hostname, &row.version, &row.status); err != nil {
			t.Fatalf("scan worker row: %v", err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("workers with shared id = %d, want 2: %+v", len(got), got)
	}
	if got[0] != (workerRow{projectID: projectA, queue: "queue-a", hostname: "host-a", version: "1.0.0", status: string(domain.WorkerStatusActive)}) {
		t.Fatalf("project A row mismatch: %+v", got[0])
	}
	if got[1] != (workerRow{projectID: projectB, queue: "queue-b", hostname: "host-b", version: "9.9.9", status: string(domain.WorkerStatusDraining)}) {
		t.Fatalf("project B row mismatch: %+v", got[1])
	}
}

// TestIntegration_RegisterWorker_SameProjectStillUpserts confirms the
// project-equal happy path still updates queue/hostname/version/status.
func TestIntegration_RegisterWorker_SameProjectStillUpserts(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const workerID = "stable-id"
	const projectID = "proj-stable"

	if err := q.RegisterWorker(ctx, &domain.Worker{
		ID:        workerID,
		ProjectID: projectID,
		QueueName: "old-queue",
		Hostname:  "old-host",
		Version:   "0.1.0",
		Status:    domain.WorkerStatusActive,
	}); err != nil {
		t.Fatalf("first register: %v", err)
	}

	if err := q.RegisterWorker(ctx, &domain.Worker{
		ID:        workerID,
		ProjectID: projectID,
		QueueName: "new-queue",
		Hostname:  "new-host",
		Version:   "0.2.0",
		Status:    domain.WorkerStatusDraining,
	}); err != nil {
		t.Fatalf("second register: %v", err)
	}

	var (
		gotQueue, gotHostname, gotVersion, gotStatus string
	)
	err := env.DB.Pool.QueryRow(ctx,
		`SELECT queue_name, hostname, version, status FROM workers WHERE id = $1 AND project_id = $2`,
		workerID, projectID,
	).Scan(&gotQueue, &gotHostname, &gotVersion, &gotStatus)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if gotQueue != "new-queue" || gotHostname != "new-host" || gotVersion != "0.2.0" {
		t.Errorf("same-project re-register failed to upsert: queue=%q host=%q version=%q",
			gotQueue, gotHostname, gotVersion)
	}
	if gotStatus != string(domain.WorkerStatusDraining) {
		t.Errorf("status not upserted: got %q want %q", gotStatus, domain.WorkerStatusDraining)
	}
}

func TestIntegration_WorkerTasksReferenceProjectScopedWorker(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const workerID = "shared-id"
	if err := q.RegisterWorker(ctx, &domain.Worker{
		ID:        workerID,
		ProjectID: "proj-a",
		QueueName: "q",
		Status:    domain.WorkerStatusActive,
	}); err != nil {
		t.Fatalf("register worker: %v", err)
	}

	err := q.CreateWorkerTask(ctx, &domain.WorkerTask{
		ID:        "task-cross-project",
		WorkerID:  workerID,
		ProjectID: "proj-b",
		RunID:     "run-1",
		Attempt:   1,
		Status:    domain.WorkerTaskStatusAssigned,
	})
	if err == nil {
		t.Fatal("expected project-scoped worker FK to reject task for same worker_id in another project")
	}
}

// TestIntegration_GetWorkerProjectByID_NotFoundIsClean confirms the lookup
// helper used at the stream layer treats a missing row as `(false, nil)`,
// not an error.
func TestIntegration_GetWorkerProjectByID_NotFoundIsClean(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	proj, ok, err := q.GetWorkerProjectByID(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("unexpected err on missing row: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false for missing row, got proj=%q", proj)
	}
}
