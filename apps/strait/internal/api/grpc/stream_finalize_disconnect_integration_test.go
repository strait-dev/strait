//go:build integration

package grpc

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

// TestIntegration_FinalizeDisconnect_MarksOfflineAndAudits pins the Phase G
// contract: the disconnect cleanup path must (a) flip the workers row to
// `offline` and (b) write a worker.disconnected audit event, even though the
// stream's request context has been cancelled.
//
// Pre-fix the deferred block reused the cancelled stream ctx, so neither the
// audit insert nor any status transition reached Postgres. The fix uses a
// detached context with a 5s timeout.
func TestIntegration_FinalizeDisconnect_MarksOfflineAndAudits(t *testing.T) {
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

	const workerID = "disco-worker"
	const projectID = "proj-disco"

	if err := q.RegisterWorker(ctx, &domain.Worker{
		ID:        workerID,
		ProjectID: projectID,
		QueueName: "q",
		Hostname:  "host",
		Version:   "1.0",
		Status:    domain.WorkerStatusActive,
	}); err != nil {
		t.Fatalf("seed worker: %v", err)
	}

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
	}

	// finalizeDisconnect deliberately takes no ctx — it must allocate its own
	// detached context internally so it remains effective when the stream
	// ctx is already cancelled at the time the deferred cleanup fires.
	svc.finalizeDisconnect(projectID, workerID)

	// Workers row must now be offline.
	var status string
	if err := env.DB.Pool.QueryRow(ctx,
		`SELECT status FROM workers WHERE id = $1`, workerID,
	).Scan(&status); err != nil {
		t.Fatalf("read worker status: %v", err)
	}
	if status != string(domain.WorkerStatusOffline) {
		t.Errorf("worker status = %q, want %q", status, domain.WorkerStatusOffline)
	}

	// Audit event must have landed.
	var auditCount int
	if err := env.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events
		 WHERE resource_type = 'worker' AND resource_id = $1 AND action = $2`,
		workerID, domain.AuditActionWorkerDisconnected,
	).Scan(&auditCount); err != nil {
		t.Fatalf("read audit events: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("expected 1 worker.disconnected audit event, got %d", auditCount)
	}
}
