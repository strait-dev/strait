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

	"github.com/stretchr/testify/require"
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
		require.NoError(
			t,
			err)

	})
	require.False(t,

		adminOutboxTestDB ==
			nil || adminOutboxTestDB.
			Pool ==
			nil)

	return adminOutboxTestDB
}

func adminOutboxStoreForTest(t *testing.T) *store.Queries {
	t.Helper()
	return store.New(getAdminOutboxTestDB(t).Pool)
}

func cleanAdminOutboxTables(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(
		t,
		getAdminOutboxTestDB(t).CleanTables(ctx))

}

func newAdminOutboxPgQueQueue(t *testing.T) *queue.PgQueQueue {
	t.Helper()
	db := getAdminOutboxTestDB(t).Pool
	return queue.NewPgQueQueue(db, queue.NewPostgresRunWriter(db), queue.PgQueConfig{
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
	require.NoError(
		t,
		st.CreateJob(ctx,
			job))

	return job
}

func writeAdminOutboxEntry(t *testing.T, ctx context.Context, entry queue.OutboxEntry) {
	t.Helper()

	tx, err := getAdminOutboxTestDB(t).Pool.Begin(ctx)
	require.NoError(
		t,
		err)

	defer func() { _ = tx.Rollback(ctx) }()
	require.NoError(
		t,
		queue.WriteOutboxInTx(ctx, tx, []queue.
			OutboxEntry{entry}))
	require.NoError(
		t,
		tx.Commit(ctx))

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
		require.Failf(t, "test failure",

			"delete job: %v", err)
	}

	flusher := scheduler.NewOutboxFlusher(getAdminOutboxTestDB(t).Pool, newAdminOutboxPgQueQueue(t), scheduler.OutboxFlusherConfig{
		BatchSize: 1,
	})
	require.NoError(
		t,
		flusher.
			FlushOnceForTest(ctx))

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.outboxAdminStore = st

	w := httptest.NewRecorder()
	req := authedProjectRequest(http.MethodGet, "/v1/admin/outbox/"+entry.ID, "", job.ProjectID)
	srv.ServeHTTP(w, req)
	require.Equal(t,

		http.StatusOK,
		w.Code,
	)

	var row AdminOutboxRow
	require.NoError(
		t,
		json.Unmarshal(w.Body.
			Bytes(), &row,
		))
	require.Equal(t,

		entry.ID,
		row.ID)
	require.False(t,

		row.ProjectID !=
			job.
				ProjectID || row.
			JobID != job.
			ID)
	require.False(t,

		row.ConsumedAt.
			IsZero())
	require.NotEqual(
		t, "", row.
			Error)
	require.Equal(t,

		entry.Priority,
		row.
			Priority)
	require.Equal(t,

		string(entry.
			Payload,
		), string(row.Payload))

	var gotMeta map[string]any
	require.NoError(
		t,
		json.Unmarshal(row.
			Metadata, &gotMeta,
		))
	require.False(t,

		gotMeta["source"] !=
			"integration" ||
			gotMeta["kind"] != "admin-readback",
	)

}
