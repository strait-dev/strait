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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		require.NoError(
			t,
			err)

		triggerLimitTestRedis, err = testutil.SetupSharedTestRedis(context.Background(), "api-trigger-limit")
		require.NoError(
			t,
			err)

	})
	require.False(t,

		triggerLimitTestDB ==
			nil || triggerLimitTestDB.
			Pool ==
			nil)

	return triggerLimitTestDB
}

func newTriggerLimitBillingEnforcer(t *testing.T, ctx context.Context, db *testutil.TestDB) *billing.Enforcer {
	t.Helper()
	require.False(t,

		triggerLimitTestRedis ==
			nil ||
			triggerLimitTestRedis.
				Client ==
				nil)
	require.NoError(
		t,
		triggerLimitTestRedis.
			FlushAll(ctx))

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
	require.NoError(
		t,
		db.CleanTables(ctx))

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
	require.NoError(
		t,
		st.CreateProject(ctx,
			&domain.
				Project{ID: projectID,
				OrgID: "org-trigger-limit",
				Name:  "trigger limit project",
			}))

	if _, err := db.Pool.Exec(ctx, `INSERT INTO project_quotas (project_id, max_queued_runs) VALUES ($1, 1)`, projectID); err != nil {
		require.Failf(t, "test failure",

			"insert project quota: %v", err)
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
			assert.Failf(t, "test failure",

				"handleTriggerJob unexpected error: %v", err)
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, successes.
		Load())
	require.EqualValues(t, 7, quotaFailures.
		Load())

	queued, err := st.CountProjectQueuedRuns(ctx, projectID)
	require.NoError(
		t,
		err)
	require.EqualValues(t, 1, queued)

}

func TestIntegration_TriggerLimitGuard_SerializesJobRateLimit(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	require.NoError(
		t,
		db.CleanTables(ctx))

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
		require.Failf(t, "test failure",

			"insert project: %v", err)
	}
	job := testutil.MustCreateJob(t, ctx, st, &testutil.JobOpts{ProjectID: &projectID})
	job.RateLimitMax = 1
	job.RateLimitWindowSecs = int((10 * time.Minute).Seconds())
	require.NoError(
		t,
		st.UpdateJob(ctx,
			job))

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
			assert.Failf(t, "test failure",

				"handleTriggerJob unexpected error: %v", err)
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, successes.
		Load())
	require.EqualValues(t, 7, rateFailures.
		Load())

}

func TestIntegration_TriggerLimitGuard_SerializesProjectQuotaAcrossJobs(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	require.NoError(
		t,
		db.CleanTables(ctx))

	st := store.NewWithContextRouting(db.Pool)
	q := newTriggerLimitPgQueQueue(t, db)
	srv := NewServer(ServerDeps{
		Config: &config.Config{InternalSecret: "test-secret-value", MaxBulkTriggerItems: 500, JWTSigningKey: testJWTSigningKey},
		Store:  st, Queue: q, Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	projectID := "project-" + uuid.Must(uuid.NewV7()).String()
	if _, err := db.Pool.Exec(ctx, `INSERT INTO projects (id, name) VALUES ($1, $2)`, projectID, "trigger cross-job quota project"); err != nil {
		require.Failf(t, "test failure",

			"insert project: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO project_quotas (project_id, max_queued_runs) VALUES ($1, 1)`, projectID); err != nil {
		require.Failf(t, "test failure",

			"insert project quota: %v", err)
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
				assert.Failf(t, "test failure",

					"handleTriggerJob unexpected error: %v", err)
			})
		}
	}
	wg.Wait()
	require.False(t,

		successes.
			Load() !=
			1 || quotaFailures.
			Load() != 1)

}

func TestIntegration_BulkTriggerLimitGuard_RejectsBatchBeyondQueuedQuota(t *testing.T) {
	ctx := context.Background()
	db := getTriggerLimitTestDB(t)
	require.NoError(
		t,
		db.CleanTables(ctx))

	st := store.NewWithContextRouting(db.Pool)
	q := newTriggerLimitPgQueQueue(t, db)
	srv := NewServer(ServerDeps{
		Config: &config.Config{InternalSecret: "test-secret-value", MaxBulkTriggerItems: 500, JWTSigningKey: testJWTSigningKey},
		Store:  st, Queue: q, BillingEnforcer: newTriggerLimitBillingEnforcer(t, ctx, db), Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	projectID := "project-" + uuid.Must(uuid.NewV7()).String()
	require.NoError(
		t,
		st.CreateProject(ctx,
			&domain.
				Project{ID: projectID,
				OrgID: "org-bulk-trigger-limit",

				Name: "bulk trigger quota project",
			},
		))

	if _, err := db.Pool.Exec(ctx, `INSERT INTO project_quotas (project_id, max_queued_runs) VALUES ($1, 1)`, projectID); err != nil {
		require.Failf(t, "test failure",

			"insert project quota: %v", err)
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
	require.False(t,

		!errors.As(err, &statusErr) ||
			statusErr.GetStatus() !=
				http.
					StatusTooManyRequests ||
			!strings.Contains(err.Error(), "project queued quota exceeded"),
	)

	queued, countErr := st.CountProjectQueuedRuns(ctx, projectID)
	require.Nil(t, countErr)
	require.EqualValues(t, 0, queued)

}
