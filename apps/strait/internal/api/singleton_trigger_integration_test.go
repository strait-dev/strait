//go:build integration

package api

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

// newSingletonTriggerServer builds a cloud server backed by the shared trigger
// integration DB and returns it together with a project id and a request context
// already scoped to that project.
func newSingletonTriggerServer(t *testing.T, ctx context.Context, db *testutil.TestDB) (*Server, store.Store, string, context.Context) {
	t.Helper()
	st := store.NewWithContextRouting(db.Pool)
	q := queue.NewPostgresQueue(db.Pool)
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testJWTSigningKey,
		},
		Store:   st,
		Queue:   q,
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	projectID := "project-" + uuid.Must(uuid.NewV7()).String()
	if _, err := db.Pool.Exec(ctx, `INSERT INTO projects (id, name) VALUES ($1, $2)`, projectID, "singleton trigger project"); err != nil {
		t.Fatalf("insert project: %v", err)
	}

	reqCtx := context.WithValue(ctx, ctxProjectIDKey, projectID)
	reqCtx = context.WithValue(reqCtx, ctxActorTypeKey, "api_key")
	reqCtx = context.WithValue(reqCtx, ctxActorIDKey, "apikey:test")
	return srv, st, projectID, reqCtx
}

// mustCreateSingletonJob creates a job and persists the given singleton config on
// it via UpdateJob so the trigger handler observes it through GetJob.
func mustCreateSingletonJob(t *testing.T, ctx context.Context, st store.Store, projectID, template, policy string, maxDepth *int) *domain.Job {
	t.Helper()
	slug := "singleton-job-" + uuid.Must(uuid.NewV7()).String()
	job := testutil.MustCreateJob(t, ctx, st, &testutil.JobOpts{ProjectID: &projectID, Slug: &slug})
	tpl, err := json.Marshal(domain.SingletonKeyExpr{Template: template})
	if err != nil {
		t.Fatalf("marshal template: %v", err)
	}
	job.SingletonKeyExpr = tpl
	job.SingletonOnConflict = domain.SingletonOnConflict(policy)
	job.SingletonMaxQueueDepth = maxDepth
	if err := st.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob() error = %v", err)
	}
	return job
}

func triggerSingleton(t *testing.T, ctx context.Context, srv *Server, jobID, payload string) map[string]any {
	t.Helper()
	out, err := srv.handleTriggerJob(ctx, &TriggerJobInput{
		JobID: jobID,
		Body:  TriggerRequest{Payload: []byte(payload)},
	})
	if err != nil {
		t.Fatalf("handleTriggerJob(%s) error = %v", jobID, err)
	}
	body, ok := out.Body.(map[string]any)
	if !ok {
		t.Fatalf("trigger body type = %T, want map[string]any", out.Body)
	}
	return body
}

func TestIntegration_SingletonTrigger_Dispatched(t *testing.T) {
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newSingletonTriggerServer(t, ctx, db)
	job := mustCreateSingletonJob(t, ctx, st, projectID, "${id}", string(domain.SingletonOnConflictQueue), nil)

	body := triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"acct-1"}`)
	if got := body["singleton_outcome"]; got != string(domain.SingletonOutcomeDispatched) {
		t.Fatalf("outcome = %v, want dispatched", got)
	}

	q := store.New(db.Pool)
	holder, err := q.GetSingletonHolder(ctx, projectID, domain.SingletonKindJob, job.ID, "acct-1")
	if err != nil {
		t.Fatalf("GetSingletonHolder() error = %v", err)
	}
	if holder.HolderRunID != body["id"].(string) {
		t.Fatalf("holder = %s, want %s", holder.HolderRunID, body["id"])
	}
}

func TestIntegration_SingletonTrigger_Drop(t *testing.T) {
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newSingletonTriggerServer(t, ctx, db)
	job := mustCreateSingletonJob(t, ctx, st, projectID, "${id}", string(domain.SingletonOnConflictDrop), nil)

	first := triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"acct-1"}`)
	second := triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"acct-1"}`)

	if got := second["singleton_outcome"]; got != string(domain.SingletonOutcomeDropped) {
		t.Fatalf("second outcome = %v, want dropped", got)
	}
	if _, ok := second["id"]; ok {
		t.Fatalf("dropped trigger must not return a run id, got %v", second["id"])
	}
	if got := second["singleton_holder_run_id"]; got != first["id"] {
		t.Fatalf("dropped holder = %v, want %v", got, first["id"])
	}

	q := store.New(db.Pool)
	var runCount int
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_runs WHERE job_id = $1`, job.ID).Scan(&runCount); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runCount != 1 {
		t.Fatalf("run count = %d, want 1 (drop creates no run)", runCount)
	}
	waiters, err := q.CountSingletonWaiters(ctx, domain.SingletonKindJob, job.ID, "acct-1")
	if err != nil {
		t.Fatalf("CountSingletonWaiters() error = %v", err)
	}
	if waiters != 0 {
		t.Fatalf("waiters = %d, want 0", waiters)
	}
}

func TestIntegration_SingletonTrigger_QueueParksWaiter(t *testing.T) {
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newSingletonTriggerServer(t, ctx, db)
	job := mustCreateSingletonJob(t, ctx, st, projectID, "${id}", string(domain.SingletonOnConflictQueue), nil)

	_ = triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"acct-1"}`)
	second := triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"acct-1"}`)

	if got := second["singleton_outcome"]; got != string(domain.SingletonOutcomeQueuedBehind) {
		t.Fatalf("second outcome = %v, want queued_behind", got)
	}

	q := store.New(db.Pool)
	parkedID := second["id"].(string)
	status, err := q.GetRunStatus(ctx, parkedID)
	if err != nil {
		t.Fatalf("GetRunStatus() error = %v", err)
	}
	if status != domain.StatusWaiting {
		t.Fatalf("parked run status = %s, want waiting", status)
	}
	// A parked run must not be claimable by the dequeue hot path.
	var claimCount int
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_run_queue WHERE run_id = $1`, parkedID).Scan(&claimCount); err != nil {
		t.Fatalf("count claims: %v", err)
	}
	if claimCount != 0 {
		t.Fatalf("parked run claim rows = %d, want 0", claimCount)
	}
	waiters, err := q.CountSingletonWaiters(ctx, domain.SingletonKindJob, job.ID, "acct-1")
	if err != nil {
		t.Fatalf("CountSingletonWaiters() error = %v", err)
	}
	if waiters != 1 {
		t.Fatalf("waiters = %d, want 1", waiters)
	}
}

func TestIntegration_SingletonTrigger_QueueCapOverflowDropped(t *testing.T) {
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newSingletonTriggerServer(t, ctx, db)
	depth := 1
	job := mustCreateSingletonJob(t, ctx, st, projectID, "${id}", string(domain.SingletonOnConflictQueue), &depth)

	_ = triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"acct-1"}`) // holder
	first := triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"acct-1"}`)
	if got := first["singleton_outcome"]; got != string(domain.SingletonOutcomeQueuedBehind) {
		t.Fatalf("first waiter outcome = %v, want queued_behind", got)
	}
	// Second waiter would exceed the depth-1 cap -> dropped.
	overflow := triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"acct-1"}`)
	if got := overflow["singleton_outcome"]; got != string(domain.SingletonOutcomeDropped) {
		t.Fatalf("overflow outcome = %v, want dropped", got)
	}

	q := store.New(db.Pool)
	waiters, err := q.CountSingletonWaiters(ctx, domain.SingletonKindJob, job.ID, "acct-1")
	if err != nil {
		t.Fatalf("CountSingletonWaiters() error = %v", err)
	}
	if waiters != 1 {
		t.Fatalf("waiters = %d, want 1 (cap held)", waiters)
	}
}

func TestIntegration_SingletonTrigger_ReplaceCancelsHolder(t *testing.T) {
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newSingletonTriggerServer(t, ctx, db)
	job := mustCreateSingletonJob(t, ctx, st, projectID, "${id}", string(domain.SingletonOnConflictReplace), nil)

	holder := triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"acct-1"}`)
	replacement := triggerSingleton(t, reqCtx, srv, job.ID, `{"id":"acct-1"}`)

	if got := replacement["singleton_outcome"]; got != string(domain.SingletonOutcomeReplaced) {
		t.Fatalf("outcome = %v, want replaced", got)
	}
	if got := replacement["singleton_holder_run_id"]; got != holder["id"] {
		t.Fatalf("replaced holder = %v, want %v", got, holder["id"])
	}

	q := store.New(db.Pool)
	holderStatus, err := q.GetRunStatus(ctx, holder["id"].(string))
	if err != nil {
		t.Fatalf("GetRunStatus(holder) error = %v", err)
	}
	if holderStatus != domain.StatusCanceled {
		t.Fatalf("holder status = %s, want canceled", holderStatus)
	}
	newStatus, err := q.GetRunStatus(ctx, replacement["id"].(string))
	if err != nil {
		t.Fatalf("GetRunStatus(replacement) error = %v", err)
	}
	if newStatus != domain.StatusWaiting {
		t.Fatalf("replacement status = %s, want waiting (acquires key on holder release)", newStatus)
	}
	// The lock is still held by the canceled holder until Phase 3 release/promote.
	lock, err := q.GetSingletonHolder(ctx, projectID, domain.SingletonKindJob, job.ID, "acct-1")
	if err != nil {
		t.Fatalf("GetSingletonHolder() error = %v", err)
	}
	if lock.HolderRunID != holder["id"].(string) {
		t.Fatalf("lock holder = %s, want canceled holder %s", lock.HolderRunID, holder["id"])
	}
}

func TestIntegration_SingletonTrigger_ConcurrentExactlyOneDispatched(t *testing.T) {
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newSingletonTriggerServer(t, ctx, db)
	job := mustCreateSingletonJob(t, ctx, st, projectID, "${id}", string(domain.SingletonOnConflictQueue), nil)

	const n = 16
	var dispatched, queuedBehind, other atomic.Int64
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := srv.handleTriggerJob(reqCtx, &TriggerJobInput{
				JobID: job.ID,
				Body:  TriggerRequest{Payload: []byte(`{"id":"acct-1"}`)},
			})
			if err != nil {
				t.Errorf("handleTriggerJob error = %v", err)
				return
			}
			body := out.Body.(map[string]any)
			switch body["singleton_outcome"] {
			case string(domain.SingletonOutcomeDispatched):
				dispatched.Add(1)
			case string(domain.SingletonOutcomeQueuedBehind):
				queuedBehind.Add(1)
			default:
				other.Add(1)
			}
		}()
	}
	wg.Wait()

	if dispatched.Load() != 1 {
		t.Fatalf("dispatched = %d, want exactly 1", dispatched.Load())
	}
	if queuedBehind.Load() != n-1 {
		t.Fatalf("queued_behind = %d, want %d", queuedBehind.Load(), n-1)
	}
	if other.Load() != 0 {
		t.Fatalf("unexpected other outcomes = %d", other.Load())
	}

	q := store.New(db.Pool)
	waiters, err := q.CountSingletonWaiters(ctx, domain.SingletonKindJob, job.ID, "acct-1")
	if err != nil {
		t.Fatalf("CountSingletonWaiters() error = %v", err)
	}
	if waiters != n-1 {
		t.Fatalf("waiters = %d, want %d", waiters, n-1)
	}
}
