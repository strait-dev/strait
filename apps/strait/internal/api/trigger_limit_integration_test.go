//go:build integration

package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
)

var (
	triggerLimitTestDB     *testutil.TestDB
	triggerLimitTestRedis  *testutil.TestRedis
	triggerLimitTestDBOnce sync.Once
)

func getTriggerLimitTestDB(t *testing.T) *testutil.TestDB {
	t.Helper()
	triggerLimitTestDBOnce.Do(func() {
		var err error
		triggerLimitTestDB, err = testutil.SetupSharedTestDB(context.Background(), "../../migrations", "api-trigger-limit")
		if err != nil {
			t.Fatalf("SetupTestDB() error = %v", err)
		}
		triggerLimitTestRedis, err = testutil.SetupSharedTestRedis(context.Background(), "api-trigger-limit")
		if err != nil {
			t.Fatalf("SetupTestRedis() error = %v", err)
		}
	})
	if triggerLimitTestDB == nil || triggerLimitTestDB.Pool == nil {
		t.Fatal("triggerLimitTestDB is not initialized")
	}
	return triggerLimitTestDB
}

func newTriggerLimitBillingEnforcer(t *testing.T, ctx context.Context, db *testutil.TestDB) *billing.Enforcer {
	t.Helper()
	if triggerLimitTestRedis == nil || triggerLimitTestRedis.Client == nil {
		t.Fatal("triggerLimitTestRedis is not initialized")
	}
	if err := triggerLimitTestRedis.FlushAll(ctx); err != nil {
		t.Fatalf("FlushAll() error = %v", err)
	}
	return billing.NewEnforcer(billing.NewPgStore(db.Pool), triggerLimitTestRedis.Client, nil)
}

func newTriggerLimitPgQueQueue(t *testing.T, db *testutil.TestDB) *queue.PgQueQueue {
	t.Helper()
	return queue.NewPgQueQueue(db.Pool, queue.NewPostgresRunWriter(db.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "api-trigger-limit-" + uuid.Must(uuid.NewV7()).String(),
		ReceiveWindow: 100,
	})
}

func TestIntegration_TriggerLimitGuard_SerializesQueuedQuota(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}

	st := store.NewWithContextRouting(db.Pool)
	q := newTriggerLimitPgQueQueue(t, db)
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testJWTSigningKey,
		},
		Store:           st,
		Queue:           q,
		BillingEnforcer: newTriggerLimitBillingEnforcer(t, ctx, db),
		Edition:         domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	projectID := "project-" + uuid.Must(uuid.NewV7()).String()
	if err := st.CreateProject(ctx, &domain.Project{
		ID:    projectID,
		OrgID: "org-trigger-limit",
		Name:  "trigger limit project",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO project_quotas (project_id, max_queued_runs) VALUES ($1, 1)`, projectID); err != nil {
		t.Fatalf("insert project quota: %v", err)
	}
	job := testutil.MustCreateJob(t, ctx, st, &testutil.JobOpts{ProjectID: &projectID})

	reqCtx := context.WithValue(ctx, ctxProjectIDKey, projectID)
	reqCtx = context.WithValue(reqCtx, ctxActorTypeKey, "api_key")
	reqCtx = context.WithValue(reqCtx, ctxActorIDKey, "apikey:test")

	var successes atomic.Int64
	var quotaFailures atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		concWG.Go(func() {
			defer wg.Done()
			_, err := srv.handleTriggerJob(reqCtx, &TriggerJobInput{
				JobID: job.ID,
				Body:  TriggerRequest{Payload: []byte(`{"ok":true}`)},
			})
			if err == nil {
				successes.Add(1)
				return
			}
			var statusErr huma.StatusError
			if errors.As(err, &statusErr) && statusErr.GetStatus() == http.StatusTooManyRequests &&
				strings.Contains(err.Error(), "project queued quota exceeded") {
				quotaFailures.Add(1)
				return
			}
			t.Errorf("handleTriggerJob unexpected error: %v", err)
		})
	}
	wg.Wait()

	if successes.Load() != 1 {
		t.Fatalf("successful triggers = %d, want 1", successes.Load())
	}
	if quotaFailures.Load() != 7 {
		t.Fatalf("quota failures = %d, want 7", quotaFailures.Load())
	}

	queued, err := st.CountProjectQueuedRuns(ctx, projectID)
	if err != nil {
		t.Fatalf("CountProjectQueuedRuns() error = %v", err)
	}
	if queued != 1 {
		t.Fatalf("queued runs = %d, want 1", queued)
	}
}

func TestIntegration_TriggerLimitGuard_SerializesJobRateLimit(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}

	st := store.NewWithContextRouting(db.Pool)
	q := newTriggerLimitPgQueQueue(t, db)
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
	if _, err := db.Pool.Exec(ctx, `INSERT INTO projects (id, name) VALUES ($1, $2)`, projectID, "trigger rate project"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	job := testutil.MustCreateJob(t, ctx, st, &testutil.JobOpts{ProjectID: &projectID})
	job.RateLimitMax = 1
	job.RateLimitWindowSecs = int((10 * time.Minute).Seconds())
	if err := st.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob() error = %v", err)
	}

	reqCtx := context.WithValue(ctx, ctxProjectIDKey, projectID)
	reqCtx = context.WithValue(reqCtx, ctxActorTypeKey, "api_key")
	reqCtx = context.WithValue(reqCtx, ctxActorIDKey, "apikey:test")

	var successes atomic.Int64
	var rateFailures atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		concWG.Go(func() {
			defer wg.Done()
			_, err := srv.handleTriggerJob(reqCtx, &TriggerJobInput{
				JobID: job.ID,
				Body:  TriggerRequest{Payload: []byte(`{"ok":true}`)},
			})
			if err == nil {
				successes.Add(1)
				return
			}
			var statusErr huma.StatusError
			if errors.As(err, &statusErr) && statusErr.GetStatus() == http.StatusTooManyRequests &&
				strings.Contains(err.Error(), "job rate limit exceeded") {
				rateFailures.Add(1)
				return
			}
			t.Errorf("handleTriggerJob unexpected error: %v", err)
		})
	}
	wg.Wait()

	if successes.Load() != 1 {
		t.Fatalf("successful triggers = %d, want 1", successes.Load())
	}
	if rateFailures.Load() != 7 {
		t.Fatalf("rate failures = %d, want 7", rateFailures.Load())
	}
}

func TestIntegration_TriggerLimitGuard_SerializesProjectQuotaAcrossJobs(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}

	st := store.NewWithContextRouting(db.Pool)
	q := newTriggerLimitPgQueQueue(t, db)
	srv := NewServer(ServerDeps{
		Config: &config.Config{InternalSecret: "test-secret-value", MaxBulkTriggerItems: 500, JWTSigningKey: testJWTSigningKey},
		Store:  st, Queue: q, Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	projectID := "project-" + uuid.Must(uuid.NewV7()).String()
	if _, err := db.Pool.Exec(ctx, `INSERT INTO projects (id, name) VALUES ($1, $2)`, projectID, "trigger cross-job quota project"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO project_quotas (project_id, max_queued_runs) VALUES ($1, 1)`, projectID); err != nil {
		t.Fatalf("insert project quota: %v", err)
	}
	slugA := "job-a-" + uuid.Must(uuid.NewV7()).String()
	slugB := "job-b-" + uuid.Must(uuid.NewV7()).String()
	jobA := testutil.MustCreateJob(t, ctx, st, &testutil.JobOpts{ProjectID: &projectID, Slug: &slugA})
	jobB := testutil.MustCreateJob(t, ctx, st, &testutil.JobOpts{ProjectID: &projectID, Slug: &slugB})

	reqCtx := context.WithValue(ctx, ctxProjectIDKey, projectID)
	reqCtx = context.WithValue(reqCtx, ctxActorTypeKey, "api_key")
	reqCtx = context.WithValue(reqCtx, ctxActorIDKey, "apikey:test")

	var successes atomic.Int64
	var quotaFailures atomic.Int64
	var wg sync.WaitGroup
	for _, job := range []*domain.Job{jobA, jobB} {
		wg.Add(1)
		{
			jobID := job.ID
			concWG.Go(func() {
				defer wg.Done()
				_, err := srv.handleTriggerJob(reqCtx, &TriggerJobInput{JobID: jobID, Body: TriggerRequest{Payload: []byte(`{"ok":true}`)}})
				if err == nil {
					successes.Add(1)
					return
				}
				var statusErr huma.StatusError
				if errors.As(err, &statusErr) && statusErr.GetStatus() == http.StatusTooManyRequests &&
					strings.Contains(err.Error(), "project queued quota exceeded") {
					quotaFailures.Add(1)
					return
				}
				t.Errorf("handleTriggerJob unexpected error: %v", err)
			})
		}
	}
	wg.Wait()

	if successes.Load() != 1 || quotaFailures.Load() != 1 {
		t.Fatalf("successes=%d quotaFailures=%d, want 1/1", successes.Load(), quotaFailures.Load())
	}
}

func TestIntegration_BulkTriggerLimitGuard_RejectsBatchBeyondQueuedQuota(t *testing.T) {
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}

	st := store.NewWithContextRouting(db.Pool)
	q := newTriggerLimitPgQueQueue(t, db)
	srv := NewServer(ServerDeps{
		Config: &config.Config{InternalSecret: "test-secret-value", MaxBulkTriggerItems: 500, JWTSigningKey: testJWTSigningKey},
		Store:  st, Queue: q, BillingEnforcer: newTriggerLimitBillingEnforcer(t, ctx, db), Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	projectID := "project-" + uuid.Must(uuid.NewV7()).String()
	if err := st.CreateProject(ctx, &domain.Project{
		ID:    projectID,
		OrgID: "org-bulk-trigger-limit",
		Name:  "bulk trigger quota project",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO project_quotas (project_id, max_queued_runs) VALUES ($1, 1)`, projectID); err != nil {
		t.Fatalf("insert project quota: %v", err)
	}
	job := testutil.MustCreateJob(t, ctx, st, &testutil.JobOpts{ProjectID: &projectID})

	reqCtx := context.WithValue(ctx, ctxProjectIDKey, projectID)
	reqCtx = context.WithValue(reqCtx, ctxActorTypeKey, "api_key")
	reqCtx = context.WithValue(reqCtx, ctxActorIDKey, "apikey:test")
	_, err := srv.handleBulkTriggerJob(reqCtx, &BulkTriggerJobInput{
		JobID: job.ID,
		Body: BulkTriggerRequest{Items: []BulkTriggerItem{
			{Payload: []byte(`{"n":1}`)},
			{Payload: []byte(`{"n":2}`)},
		}},
	})
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) || statusErr.GetStatus() != http.StatusTooManyRequests ||
		!strings.Contains(err.Error(), "project queued quota exceeded") {
		t.Fatalf("handleBulkTriggerJob error = %v, want queued quota 429", err)
	}

	queued, countErr := st.CountProjectQueuedRuns(ctx, projectID)
	if countErr != nil {
		t.Fatalf("CountProjectQueuedRuns() error = %v", countErr)
	}
	if queued != 0 {
		t.Fatalf("queued runs = %d, want 0 after rolled-back over-quota bulk trigger", queued)
	}
}
