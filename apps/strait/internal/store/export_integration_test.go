//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// Integration tests for the Stream* export iterators, which had zero
// direct coverage prior to this commit despite being the data export /
// backup / migration code path.
// StreamJobs

func TestStreamJobs_CallbackPerRow(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-stream-jobs-" + newID()
	const want = 5
	created := make(map[string]bool, want)
	for range want {
		job := mustCreateJob(t, ctx, q, projectID)
		created[job.ID] = true
	}

	var streamed []string
	require.NoError(t, q.StreamJobs(ctx,
		projectID,
		func(j *domain.Job) error {
			streamed = append(streamed,
				j.ID)
			return nil
		}),
	)
	require.Len(t, streamed,

		want)

	for _, id := range streamed {
		require.True(t, created[id])

	}
}

func TestStreamJobs_OrderedByCreatedAtAsc(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-stream-jobs-order-" + newID()
	// Create jobs in sequence. DEFAULT created_at = NOW() is monotonic
	// on the same DB connection, so insertion order == creation order.
	firstID := mustCreateJob(t, ctx, q, projectID).ID
	secondID := mustCreateJob(t, ctx, q, projectID).ID
	thirdID := mustCreateJob(t, ctx, q, projectID).ID

	var order []string
	require.NoError(t, q.StreamJobs(ctx,
		projectID,
		func(j *domain.Job) error {
			order = append(order, j.
				ID)
			return nil
		}))
	require.Len(t, order, 3)
	require.False(t, order[0] != firstID ||
		order[1] != secondID ||
		order[2] !=
			thirdID)

}

func TestStreamJobs_CallbackError_StopsIteration(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-stream-jobs-err-" + newID()
	for range 5 {
		mustCreateJob(t, ctx, q, projectID)
	}

	sentinel := errors.New("stop at 3")
	visited := 0
	err := q.StreamJobs(ctx, projectID, func(_ *domain.Job) error {
		visited++
		if visited == 3 {
			return sentinel
		}
		return nil
	})
	require.True(t, errors.Is(err, sentinel))
	require.EqualValues(t, 3, visited)

}

func TestStreamJobs_EmptyProject_NoCallback(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	called := false
	require.NoError(t, q.StreamJobs(ctx,
		"proj-empty-"+
			newID(), func(
			_ *domain.
				Job) error {
			called = true
			return nil
		}))
	require.False(t, called)

}

// StreamRuns: time-window filtering + boundary inclusivity

func TestStreamRuns_TimeWindowInclusive(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-stream-runs-" + newID()
	job := mustCreateJob(t, ctx, q, projectID)

	// Create three runs. Their created_at timestamps come from Postgres
	// NOW() at insert time. We then fetch them back to get the exact
	// timestamps for boundary calculations.
	var runIDs []string
	for range 3 {
		r := baseRun(job, newID())
		require.NoError(t, q.CreateRun(ctx,
			r))

		runIDs = append(runIDs, r.ID)
	}

	// Read back the middle run's created_at to use as the window bound.
	middle, err := q.GetRun(ctx, runIDs[1])
	require.NoError(t, err)

	// Window [middle.created_at, middle.created_at] should return exactly
	// the middle run. The query uses created_at >= $2 AND created_at <= $3
	// so both ends are inclusive.
	var visited []string
	require.NoError(t, q.StreamRuns(ctx,
		projectID,
		middle.
			CreatedAt,
		middle.
			CreatedAt,
		func(r *domain.JobRun) error {
			visited =
				append(visited, r.ID)
			return nil
		}))
	require.False(t, len(visited) !=
		1 || visited[0] != middle.
		ID)

	// Wide window returns all three.
	visited = nil
	require.NoError(t, q.StreamRuns(ctx,
		projectID,
		middle.
			CreatedAt.
			Add(-time.
				Hour), middle.CreatedAt.Add(time.Hour), func(r *domain.
			JobRun) error {
			visited = append(visited, r.ID)
			return nil
		}))
	require.Len(t, visited,

		3)

}

func TestStreamRuns_CallbackError_StopsIteration(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-stream-runs-err-" + newID()
	job := mustCreateJob(t, ctx, q, projectID)
	for range 5 {
		r := baseRun(job, newID())
		require.NoError(t, q.CreateRun(ctx,
			r))

	}

	sentinel := errors.New("boom")
	visited := 0
	err := q.StreamRuns(ctx, projectID,
		time.Now().Add(-time.Hour), time.Now().Add(time.Hour),
		func(_ *domain.JobRun) error {
			visited++
			if visited == 2 {
				return sentinel
			}
			return nil
		})
	require.True(t, errors.Is(err, sentinel))
	require.EqualValues(t, 2, visited)

}

// StreamWorkflows: callback iteration sanity

func TestStreamWorkflows_CallbackPerRow(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-stream-wf-" + newID()
	const want = 3
	created := make(map[string]bool, want)
	for i := range want {
		wf := &domain.Workflow{
			ID:        "wf-" + newID(),
			ProjectID: projectID,
			Name:      "wf-" + newID(),
			Slug:      "wf-slug-" + newID(),
			Enabled:   true,
			Version:   i + 1,
		}
		require.NoError(t, q.CreateWorkflow(ctx, wf))

		created[wf.ID] = true
	}

	var visited []string
	require.NoError(t, q.StreamWorkflows(ctx, projectID,
		func(w *domain.
			Workflow) error {
			visited = append(visited, w.ID)
			return nil
		}))
	require.Len(t, visited,

		want)

	for _, id := range visited {
		require.True(t, created[id])

	}
}

// Count*ByOrg quota methods: cross-org isolation

func TestCountProjectsByOrg_CrossOrgIsolation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgA := "org-count-a-" + newID()
	orgB := "org-count-b-" + newID()

	for i := range 3 {
		p := &domain.Project{
			ID:    "proj-a-" + newID(),
			Name:  "A-" + string(rune('0'+i)),
			OrgID: orgA,
		}
		require.NoError(t, q.CreateProject(ctx, p))

	}
	for range 2 {
		p := &domain.Project{
			ID:    "proj-b-" + newID(),
			Name:  "B",
			OrgID: orgB,
		}
		require.NoError(t, q.CreateProject(ctx, p))

	}

	countA, err := q.CountProjectsByOrg(ctx, orgA)
	require.NoError(t, err)
	require.EqualValues(t, 3, countA)

	countB, err := q.CountProjectsByOrg(ctx, orgB)
	require.NoError(t, err)
	require.EqualValues(t, 2, countB)

	countEmpty, err := q.CountProjectsByOrg(ctx, "org-empty-"+newID())
	require.NoError(t, err)
	require.EqualValues(t, 0, countEmpty)

}

func TestCountProjectsByOrg_SoftDeletedExcluded(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-soft-" + newID()

	alive := &domain.Project{ID: "proj-alive-" + newID(), Name: "alive", OrgID: orgID}
	require.NoError(t, q.CreateProject(ctx, alive))

	dead := &domain.Project{ID: "proj-dead-" + newID(), Name: "dead", OrgID: orgID}
	require.NoError(t, q.CreateProject(ctx, dead))

	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE projects SET deleted_at = NOW() WHERE id = $1`, dead.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"soft-delete: %v", err)
	}

	count, err := q.CountProjectsByOrg(ctx, orgID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

}

// GetLogDrain cross-tenant guard

func TestGetLogDrain_WrongProject_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projA := "proj-get-drain-a-" + newID()
	projB := "proj-get-drain-b-" + newID()

	drain := &domain.LogDrain{
		ID:          "drain-" + newID(),
		ProjectID:   projA,
		Name:        "drain-name",
		DrainType:   "http",
		EndpointURL: "https://example.com/log",
		AuthType:    "none",
		Enabled:     true,
	}
	require.NoError(t, q.CreateLogDrain(ctx, drain))

	// Correct project can fetch it.
	got, err := q.GetLogDrain(ctx, drain.ID, projA)
	require.NoError(t, err)
	require.Equal(t, drain.
		ID,
		got.ID,
	)

	// Wrong project returns not-found.
	if _, err := q.GetLogDrain(ctx, drain.ID, projB); !errors.Is(err, store.ErrLogDrainNotFound) {
		require.Failf(t, "test failure",

			"cross-tenant GetLogDrain: err = %v, want ErrLogDrainNotFound", err)
	}
}
