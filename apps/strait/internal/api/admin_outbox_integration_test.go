//go:build integration

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/scheduler"
	"strait/internal/store"
	"strait/internal/testutil"
)

var (
	adminOutboxTestDB     *testutil.TestDB
	adminOutboxTestDBOnce sync.Once
)

func getAdminOutboxTestDB(t *testing.T) *testutil.TestDB {
	t.Helper()

	adminOutboxTestDBOnce.Do(func() {
		ctx := context.Background()
		var err error
		adminOutboxTestDB, err = testutil.SetupSharedTestDB(ctx, "../../migrations", "api-admin-outbox")
		if err != nil {
			t.Fatalf("SetupTestDB() error = %v", err)
		}
	})

	if adminOutboxTestDB == nil || adminOutboxTestDB.Pool == nil {
		t.Fatal("adminOutboxTestDB is not initialized")
	}
	return adminOutboxTestDB
}

func adminOutboxStoreForTest(t *testing.T) *store.Queries {
	t.Helper()
	return store.New(getAdminOutboxTestDB(t).Pool)
}

func cleanAdminOutboxTables(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := getAdminOutboxTestDB(t).CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
}

func newAdminOutboxPgQueQueue(t *testing.T) *queue.PgQueQueue {
	t.Helper()
	db := getAdminOutboxTestDB(t).Pool
	return queue.NewPgQueQueue(db, queue.NewPostgresQueue(db), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "api-admin-outbox-" + time.Now().UTC().Format("150405.000000000"),
		ReceiveWindow: 100,
	})
}

func createAdminOutboxJob(t *testing.T, ctx context.Context, st *store.Queries, projectID string) *domain.Job {
	t.Helper()
	job := &domain.Job{
		ID:          "job-" + time.Now().UTC().Format("20060102150405.000000000"),
		ProjectID:   projectID,
		Name:        "job-" + time.Now().UTC().Format("150405.000000000"),
		Slug:        "slug-" + time.Now().UTC().Format("150405.000000000"),
		EndpointURL: "https://example.com/integration-test",
		MaxAttempts: 3,
		TimeoutSecs: 300,
		Enabled:     true,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	return job
}

func writeAdminOutboxEntry(t *testing.T, ctx context.Context, entry queue.OutboxEntry) {
	t.Helper()

	tx, err := getAdminOutboxTestDB(t).Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := queue.WriteOutboxInTx(ctx, tx, []queue.OutboxEntry{entry}); err != nil {
		t.Fatalf("WriteOutboxInTx() error = %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
}

func TestAdminOutboxGet_ReturnsQuarantinedRowFromRealFlusherState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := adminOutboxStoreForTest(t)
	cleanAdminOutboxTables(t, ctx)

	job := createAdminOutboxJob(t, ctx, st, "proj-admin-outbox")
	entry := queue.OutboxEntry{
		ID:        "outbox-admin-readback",
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		Payload:   json.RawMessage(`{"kind":"poison","value":7}`),
		Metadata: map[string]any{
			"source": "integration",
			"kind":   "admin-readback",
		},
		IdempotencyKey: "admin-readback-key",
		Priority:       7,
	}
	writeAdminOutboxEntry(t, ctx, entry)

	if _, err := getAdminOutboxTestDB(t).Pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("delete job: %v", err)
	}

	flusher := scheduler.NewOutboxFlusher(getAdminOutboxTestDB(t).Pool, newAdminOutboxPgQueQueue(t), scheduler.OutboxFlusherConfig{
		BatchSize: 1,
	})
	if err := flusher.FlushOnceForTest(ctx); err != nil {
		t.Fatalf("FlushOnceForTest() error = %v", err)
	}

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.outboxAdminStore = st

	w := httptest.NewRecorder()
	req := authedProjectRequest(http.MethodGet, "/v1/admin/outbox/"+entry.ID, "", job.ProjectID)
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var row AdminOutboxRow
	if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if row.ID != entry.ID {
		t.Fatalf("row ID = %q, want %q", row.ID, entry.ID)
	}
	if row.ProjectID != job.ProjectID || row.JobID != job.ID {
		t.Fatalf("unexpected row identity: project=%q job=%q", row.ProjectID, row.JobID)
	}
	if row.ConsumedAt.IsZero() {
		t.Fatal("expected consumed_at to be set")
	}
	if row.Error == "" {
		t.Fatal("expected stored quarantine error")
	}
	if row.Priority != entry.Priority {
		t.Fatalf("Priority = %d, want %d", row.Priority, entry.Priority)
	}
	if string(row.Payload) != string(entry.Payload) {
		t.Fatalf("Payload = %s, want %s", string(row.Payload), string(entry.Payload))
	}

	var gotMeta map[string]any
	if err := json.Unmarshal(row.Metadata, &gotMeta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if gotMeta["source"] != "integration" || gotMeta["kind"] != "admin-readback" {
		t.Fatalf("unexpected metadata: %+v", gotMeta)
	}
}
