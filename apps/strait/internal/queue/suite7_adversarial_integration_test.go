//go:build integration

package queue_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
)

// Suite 7: Adversarial and security tests.

func TestAdversarial_SQLInjectionViaIdempotencyKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-adv-idem-sqli")
	q := mustQueue(t)

	malicious := []string{
		"'; DROP TABLE job_runs; --",
		"key' OR '1'='1",
		"key'; UPDATE job_runs SET status='completed'; --",
		`key"); DELETE FROM jobs; --`,
		"key$$ BEGIN; DROP TABLE jobs; END $$",
	}
	for _, key := range malicious {
		r := &domain.JobRun{
			ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
			IdempotencyKey: key,
		}
		// Should not error and should not execute injected SQL.
		_ = q.Enqueue(ctx, r)
	}
	// Table must still exist and be intact.
	var count int
	err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_runs WHERE job_id=$1`, job.ID).Scan(&count)
	if err != nil {
		t.Fatalf("table broken after injection: %v", err)
	}
}

func TestAdversarial_SQLInjectionViaConcurrencyKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-adv-ck-sqli")
	q := mustQueue(t)

	r := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		ConcurrencyKey: "'; DROP TABLE jobs; --",
	}
	_ = q.Enqueue(ctx, r)
	var count int
	_ = testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM jobs`).Scan(&count)
	if count == 0 {
		t.Error("jobs table was dropped by injection")
	}
}

func TestAdversarial_SQLInjectionViaMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-adv-meta-sqli")
	q := mustQueue(t)

	r := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		Metadata: map[string]string{
			"key":    "value'; DROP TABLE job_runs; --",
			"nested": `{"sql":"'); DELETE FROM jobs; --"}`,
		},
	}
	if err := q.Enqueue(ctx, r); err != nil {
		t.Fatalf("enqueue with malicious metadata: %v", err)
	}
	// Verify the metadata is stored as JSONB, not interpolated.
	batch, _ := q.DequeueN(ctx, 1)
	if len(batch) != 1 {
		t.Fatal("run not dequeued")
	}
	if len(batch[0].Metadata) == 0 {
		t.Error("metadata lost")
	}
}

func TestAdversarial_SQLInjectionViaPayload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-adv-payload-sqli")
	q := mustQueue(t)

	payload := `{"sql":"'; DROP TABLE job_runs; --","$body$":"END; DROP TABLE jobs; $body$"}`
	r := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		Payload: json.RawMessage(payload),
	}
	_ = q.Enqueue(ctx, r)
	var count int
	_ = testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_runs WHERE job_id=$1`, job.ID).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 run, got %d", count)
	}
}

func TestAdversarial_DequeueNegativeBatchSize(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	batch, err := q.DequeueN(ctx, -1)
	if err != nil {
		// Negative should either error or return empty, not panic.
		return
	}
	if len(batch) != 0 {
		t.Errorf("negative batch returned %d runs", len(batch))
	}
}

func TestAdversarial_EnqueueEmptyJobID(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	r := &domain.JobRun{ID: newID(), ProjectID: "p", JobID: ""}
	err := q.Enqueue(ctx, r)
	// FK constraint should catch empty job_id.
	if err == nil {
		t.Error("enqueue with empty job_id should fail")
	}
}

func TestAdversarial_EnqueueEmptyProjectID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-adv-empty-proj")
	q := mustQueue(t)
	r := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: ""}
	// Empty project_id may or may not be allowed by the schema, but
	// should not panic.
	_ = q.Enqueue(ctx, r)
}

func TestAdversarial_ResourceExhaustion_1000Enqueues(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-adv-exhaust")
	q := mustQueue(t)

	bp := queue.NewBackpressure(testDB.Pool, queue.BackpressureConfig{
		DefaultMaxTokens:    50,
		DefaultRefillPerSec: 0,
	}, true)

	var throttled int
	for range 1000 {
		if err := bp.TryConsume(ctx, job.ProjectID); err != nil {
			throttled++
			continue
		}
		r := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
		_ = q.Enqueue(ctx, r)
	}
	if throttled < 900 {
		t.Errorf("throttled only %d of 1000, want >= 900 (cap=50)", throttled)
	}
}

func TestAdversarial_OutboxWriteWithNonexistentJob(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)

	tx, _ := testDB.Pool.Begin(ctx)
	defer tx.Rollback(ctx)
	err := queue.WriteOutboxInTx(ctx, tx, []queue.OutboxEntry{{
		ProjectID: "p1", JobID: "nonexistent-job-id-12345",
	}})
	if err == nil {
		t.Error("outbox write with nonexistent job should fail")
	}
}

func TestAdversarial_IdentifierValidationRejectsInjection(t *testing.T) {
	// Already covered by store/ident_test.go fuzz, but verify the
	// integration path: a malicious partition name in the tuner
	// would be caught by ValidateIdent before reaching SQL.
	cases := []string{
		`"; DROP TABLE job_runs; --`,
		"job_runs; DELETE FROM jobs",
		"a\x00b",
	}
	for _, c := range cases {
		if err := store.ValidateIdent(c); err == nil {
			t.Errorf("ValidateIdent(%q) should reject", c)
		}
	}
}

func TestAdversarial_CursorWithZeroTime(t *testing.T) {
	c := queue.NewClaimCursor(60 * time.Second)
	c.Advance(time.Time{}, "id-zero")
	ts, id, ok := c.Snapshot()
	// Zero-time advance should still be valid.
	if !ok {
		t.Error("cursor should be valid after zero-time advance")
	}
	if id != "id-zero" {
		t.Errorf("id = %q", id)
	}
	_ = ts
}

func TestAdversarial_DLQDepthEnforcedAtDBLevel(t *testing.T) {
	// The DLQ cap enforcer lives in the worker package; here we verify
	// the underlying dlq_counts counter stays consistent under
	// adversarial DLQ insertions via direct SQL.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-adv-dlq-depth")

	// Insert 5 dead_letter rows directly.
	for range 5 {
		_, _ = testDB.Pool.Exec(ctx, `
			INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
			VALUES ($1, $2, $3, 'dead_letter', 1, 'manual', NOW(), NOW())
		`, newID(), job.ID, job.ProjectID)
	}
	// Counter should match.
	var count int
	_ = testDB.Pool.QueryRow(ctx, `SELECT COALESCE(count,0) FROM dlq_counts WHERE job_id=$1`, job.ID).Scan(&count)
	if count != 5 {
		t.Errorf("dlq counter = %d, want 5", count)
	}
}
