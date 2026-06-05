//go:build integration

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var (
	runsFilterTestDB     *testutil.TestDB
	runsFilterTestDBOnce sync.Once
)

func getRunsFilterTestDB(t *testing.T) *testutil.TestDB {
	t.Helper()

	runsFilterTestDBOnce.Do(func() {
		ctx := context.Background()
		var err error
		runsFilterTestDB, err = testutil.SetupSharedTestDB(ctx, "../../migrations", "api-runs-filter")
		require.NoError(
			t,
			err)

	})
	require.False(t,

		runsFilterTestDB ==
			nil || runsFilterTestDB.
			Pool ==
			nil)

	return runsFilterTestDB
}

func runsFilterStoreForTest(t *testing.T) *store.Queries {
	t.Helper()
	return store.New(getRunsFilterTestDB(t).Pool)
}

func cleanRunsFilterTables(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(
		t,
		getRunsFilterTestDB(t).CleanTables(
			ctx))

}

func createRunsFilterJob(t *testing.T, ctx context.Context, st *store.Queries, projectID string, mode domain.ExecutionMode) *domain.Job {
	t.Helper()

	job := &domain.Job{
		ID:            uuid.Must(uuid.NewV7()).String(),
		ProjectID:     projectID,
		Name:          "job-" + uuid.Must(uuid.NewV7()).String(),
		Slug:          "slug-" + uuid.Must(uuid.NewV7()).String(),
		EndpointURL:   "https://example.com/integration-test",
		MaxAttempts:   3,
		TimeoutSecs:   300,
		Enabled:       true,
		ExecutionMode: mode,
		Queue:         "default",
	}
	require.NoError(
		t,
		st.CreateJob(ctx,
			job))

	return job
}

func createRunsFilterRun(t *testing.T, ctx context.Context, st *store.Queries, job *domain.Job, status domain.RunStatus, tags map[string]string, triggeredBy, errorClass string, executionMode domain.ExecutionMode) *domain.JobRun {
	t.Helper()

	now := time.Now().UTC()
	run := &domain.JobRun{
		ID:            uuid.Must(uuid.NewV7()).String(),
		JobID:         job.ID,
		ProjectID:     job.ProjectID,
		Status:        status,
		Attempt:       1,
		Payload:       json.RawMessage(`{"integration":true}`),
		TriggeredBy:   triggeredBy,
		StartedAt:     &now,
		HeartbeatAt:   &now,
		ExecutionMode: executionMode,
		Tags:          tags,
	}

	if status != domain.StatusExecuting {
		run.Status = domain.StatusExecuting
	}
	require.NoError(
		t,
		st.CreateRun(ctx,
			run))

	if status != domain.StatusExecuting {
		fields := map[string]any{
			"finished_at": now,
		}
		if status != domain.StatusCompleted {
			fields["error"] = "integration failure"
			fields["error_class"] = errorClass
		}
		require.NoError(
			t,
			st.UpdateRunStatus(ctx, run.ID, domain.
				StatusExecuting,
				status, fields,
			))

	}

	return run
}

func decodeRunListResponse(t *testing.T, body []byte) []domain.JobRun {
	t.Helper()

	var resp struct {
		Data []domain.JobRun `json:"data"`
	}
	require.NoError(
		t,
		json.Unmarshal(body,
			&resp))

	return resp.Data
}

func TestHandleListRuns_TagFilterComposesWithOtherFilters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := runsFilterStoreForTest(t)
	cleanRunsFilterTables(t, ctx)

	projectID := "proj-runs-filters-compose"
	job := createRunsFilterJob(t, ctx, st, projectID, domain.ExecutionModeWorker)

	matching := createRunsFilterRun(t, ctx, st, job, domain.StatusFailed, map[string]string{"team": "infra"}, "api", "timeout", domain.ExecutionModeWorker)
	createRunsFilterRun(t, ctx, st, job, domain.StatusFailed, map[string]string{"team": "infra"}, "cron", "timeout", domain.ExecutionModeWorker)
	createRunsFilterRun(t, ctx, st, job, domain.StatusFailed, map[string]string{"team": "infra"}, "api", "connection", domain.ExecutionModeWorker)
	createRunsFilterRun(t, ctx, st, job, domain.StatusFailed, map[string]string{"team": "app"}, "api", "timeout", domain.ExecutionModeWorker)
	createRunsFilterRun(t, ctx, st, job, domain.StatusCompleted, map[string]string{"team": "infra"}, "api", "", domain.ExecutionModeWorker)

	srv := newTestServer(t, st, nil, nil)
	req := authedProjectRequest(http.MethodGet, "/v1/runs?tag_key=team&tag_value=infra&status=failed&triggered_by=api&error_class=timeout&limit=10", "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t,

		http.StatusOK,
		w.Code,
	)

	runs := decodeRunListResponse(t, w.Body.Bytes())
	require.Len(t, runs,

		1)
	require.Equal(t,

		matching.
			ID, runs[0].
			ID)

}

func TestHandleListRuns_StatusesMultiValueFiltersResults(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := runsFilterStoreForTest(t)
	cleanRunsFilterTables(t, ctx)

	projectID := "proj-runs-statuses"
	workerJob := createRunsFilterJob(t, ctx, st, projectID, domain.ExecutionModeWorker)
	httpJob := createRunsFilterJob(t, ctx, st, projectID, domain.ExecutionModeHTTP)

	failedRun := createRunsFilterRun(t, ctx, st, workerJob, domain.StatusFailed, map[string]string{"team": "infra"}, "api", "timeout", domain.ExecutionModeWorker)
	timedOutRun := createRunsFilterRun(t, ctx, st, workerJob, domain.StatusTimedOut, map[string]string{"team": "infra"}, "api", "timeout", domain.ExecutionModeWorker)
	createRunsFilterRun(t, ctx, st, workerJob, domain.StatusCompleted, map[string]string{"team": "infra"}, "api", "", domain.ExecutionModeWorker)
	createRunsFilterRun(t, ctx, st, workerJob, domain.StatusFailed, map[string]string{"team": "app"}, "api", "timeout", domain.ExecutionModeWorker)
	createRunsFilterRun(t, ctx, st, httpJob, domain.StatusFailed, map[string]string{"team": "infra"}, "api", "timeout", domain.ExecutionModeHTTP)

	srv := newTestServer(t, st, nil, nil)
	req := authedProjectRequest(http.MethodGet, "/v1/runs?tag_key=team&tag_value=infra&statuses[]=failed&statuses[]=timed_out&execution_mode=worker&limit=10", "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t,

		http.StatusOK,
		w.Code,
	)

	runs := decodeRunListResponse(t, w.Body.Bytes())
	require.Len(t, runs,

		2)

	gotIDs := []string{runs[0].ID, runs[1].ID}
	slices.Sort(gotIDs)
	wantIDs := []string{failedRun.ID, timedOutRun.ID}
	slices.Sort(wantIDs)
	require.True(t,
		slices.
			Equal(gotIDs,
				wantIDs))

}
