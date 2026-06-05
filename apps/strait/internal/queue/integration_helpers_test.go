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
	"github.com/stretchr/testify/require"
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
	require.False(t, testDB ==
		nil ||
		testDB.Pool ==
			nil,
	)

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
	require.False(t, testDB ==
		nil ||
		testDB.Pool ==
			nil,
	)

	return store.New(testDB.Pool)
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(t, testDB.
		CleanTables(ctx))

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
	require.NoError(t, st.CreateJob(ctx,
		job))

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
	require.NoError(t, q.Enqueue(ctx,
		run))

	return run
}

func markWorkerJobQueue(t *testing.T, ctx context.Context, job *domain.Job, queueName string) {
	t.Helper()
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE jobs SET execution_mode = 'worker', queue_name = $2 WHERE id = $1`,
		job.ID, queueName,
	); err != nil {
		require.Failf(t, "test failure",

			"mark worker job queue: %v", err)
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
	require.NoError(t, st.CreateEnvironment(ctx,
		env))

	return env.ID
}

func markWorkerJobQueueEnvironment(t *testing.T, ctx context.Context, job *domain.Job, queueName, environmentID string) {
	t.Helper()
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE jobs SET execution_mode = 'worker', queue_name = $2, environment_id = $3 WHERE id = $1`,
		job.ID, queueName, environmentID,
	); err != nil {
		require.Failf(t, "test failure",

			"mark worker job queue environment: %v", err)
	}
	job.ExecutionMode = domain.ExecutionModeWorker
	job.Queue = queueName
	job.EnvironmentID = environmentID
}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}
