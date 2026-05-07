//go:build integration

package grpc

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

// TestIntegration_RegisterWorker_CrossProjectIDCollisionDoesNotOverwrite
// pins the Phase F DB guard. With ON CONFLICT (id) gated by
// `WHERE workers.project_id = EXCLUDED.project_id`, an attempted upsert
// from a different project must NOT overwrite the original row's
// queue/hostname/version/status/last_seen_at fields.
func TestIntegration_RegisterWorker_CrossProjectIDCollisionDoesNotOverwrite(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

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

	// Project B attempts to register the same worker_id with different
	// queue/hostname/version. Pre-fix this would silently overwrite
	// project A's row fields.
	wB := &domain.Worker{
		ID:        workerID,
		ProjectID: projectB,
		QueueName: "queue-b",
		Hostname:  "host-b",
		Version:   "9.9.9",
		Status:    domain.WorkerStatusDraining,
	}
	if err := q.RegisterWorker(ctx, wB); err != nil {
		t.Fatalf("register B (should be silent no-op, not error): %v", err)
	}

	// Read back: row must still belong to project A with project A's fields.
	var (
		gotProject, gotQueue, gotHostname, gotVersion, gotStatus string
	)
	err = env.DB.Pool.QueryRow(ctx,
		`SELECT project_id, queue_name, hostname, version, status FROM workers WHERE id = $1`,
		workerID,
	).Scan(&gotProject, &gotQueue, &gotHostname, &gotVersion, &gotStatus)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if gotProject != projectA {
		t.Errorf("project_id overwritten: got %q want %q", gotProject, projectA)
	}
	if gotQueue != "queue-a" {
		t.Errorf("queue_name overwritten: got %q want %q", gotQueue, "queue-a")
	}
	if gotHostname != "host-a" {
		t.Errorf("hostname overwritten: got %q want %q", gotHostname, "host-a")
	}
	if gotVersion != "1.0.0" {
		t.Errorf("version overwritten: got %q want %q", gotVersion, "1.0.0")
	}
	if gotStatus != string(domain.WorkerStatusActive) {
		t.Errorf("status overwritten: got %q want %q", gotStatus, domain.WorkerStatusActive)
	}
}

// TestIntegration_RegisterWorker_SameProjectStillUpserts confirms the
// project-equal happy path still updates queue/hostname/version/status.
func TestIntegration_RegisterWorker_SameProjectStillUpserts(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

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
	err = env.DB.Pool.QueryRow(ctx,
		`SELECT queue_name, hostname, version, status FROM workers WHERE id = $1`,
		workerID,
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

// TestIntegration_GetWorkerProjectByID_NotFoundIsClean confirms the lookup
// helper used at the stream layer treats a missing row as `(false, nil)`,
// not an error.
func TestIntegration_GetWorkerProjectByID_NotFoundIsClean(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

	q := store.New(env.DB.Pool)
	proj, ok, err := q.GetWorkerProjectByID(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("unexpected err on missing row: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false for missing row, got proj=%q", proj)
	}
}
