//go:build integration

package queue_test

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

var testDB *testutil.TestDB

type enqueueQueue interface {
	Enqueue(ctx context.Context, run *domain.JobRun) error
}

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testDB, err = testutil.SetupSharedTestDB(ctx, "../../migrations", "queue")
	if err != nil {
		log.Fatalf("setup test db: %v", err)
	}

	code := m.Run()
	testDB.Cleanup(ctx)
	os.Exit(code)
}

func mustQueue(t *testing.T) *queue.PgQueQueue {
	t.Helper()

	if testDB == nil || testDB.Pool == nil {
		t.Fatal("testDB is not initialized")
	}

	q := queue.NewPgQueQueue(testDB.Pool, queue.NewPostgresRunWriter(testDB.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "queue-" + newID(),
		ReceiveWindow: 100,
	})
	tickerCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go q.RunTicker(tickerCtx)
	return q
}

func mustStore(t *testing.T) *store.Queries {
	t.Helper()

	if testDB == nil || testDB.Pool == nil {
		t.Fatal("testDB is not initialized")
	}

	return store.New(testDB.Pool)
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()

	if err := testDB.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
}

func mustCreateJob(t *testing.T, ctx context.Context, st *store.Queries, projectID string) *domain.Job {
	t.Helper()

	job := &domain.Job{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "job-" + newID(),
		Slug:        "slug-" + newID(),
		EndpointURL: "https://example.com/queue-job",
		MaxAttempts: 3,
		TimeoutSecs: 300,
		Enabled:     true,
	}

	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	return job
}

func mustEnqueueRun(t *testing.T, ctx context.Context, q enqueueQueue, job *domain.Job) *domain.JobRun {
	t.Helper()
	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Priority:  1,
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return run
}

func markWorkerJobQueue(t *testing.T, ctx context.Context, job *domain.Job, queueName string) {
	t.Helper()
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE jobs SET execution_mode = 'worker', queue_name = $2 WHERE id = $1`,
		job.ID, queueName,
	); err != nil {
		t.Fatalf("mark worker job queue: %v", err)
	}
	job.ExecutionMode = domain.ExecutionModeWorker
	job.Queue = queueName
}

func mustCreateEnvironment(t *testing.T, ctx context.Context, st *store.Queries, projectID, slug string) string {
	t.Helper()
	env := &domain.Environment{
		ProjectID: projectID,
		Name:      slug,
		Slug:      slug,
	}
	if err := st.CreateEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateEnvironment(%s): %v", slug, err)
	}
	return env.ID
}

func markWorkerJobQueueEnvironment(t *testing.T, ctx context.Context, job *domain.Job, queueName, environmentID string) {
	t.Helper()
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE jobs SET execution_mode = 'worker', queue_name = $2, environment_id = $3 WHERE id = $1`,
		job.ID, queueName, environmentID,
	); err != nil {
		t.Fatalf("mark worker job queue environment: %v", err)
	}
	job.ExecutionMode = domain.ExecutionModeWorker
	job.Queue = queueName
	job.EnvironmentID = environmentID
}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}
