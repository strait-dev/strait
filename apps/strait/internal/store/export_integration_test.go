//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
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
	if err := q.StreamJobs(ctx, projectID, func(j *domain.Job) error {
		streamed = append(streamed, j.ID)
		return nil
	}); err != nil {
		t.Fatalf("StreamJobs: %v", err)
	}

	if len(streamed) != want {
		t.Fatalf("streamed %d jobs, want %d", len(streamed), want)
	}
	for _, id := range streamed {
		if !created[id] {
			t.Fatalf("unexpected job %q", id)
		}
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
	if err := q.StreamJobs(ctx, projectID, func(j *domain.Job) error {
		order = append(order, j.ID)
		return nil
	}); err != nil {
		t.Fatalf("StreamJobs: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("streamed %d jobs, want 3", len(order))
	}
	if order[0] != firstID || order[1] != secondID || order[2] != thirdID {
		t.Fatalf("order = %v, want [%q, %q, %q]", order, firstID, secondID, thirdID)
	}
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
	if !errors.Is(err, sentinel) {
		t.Fatalf("StreamJobs err = %v, want sentinel", err)
	}
	if visited != 3 {
		t.Fatalf("visited = %d, want 3", visited)
	}
}

func TestStreamJobs_EmptyProject_NoCallback(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	called := false
	if err := q.StreamJobs(ctx, "proj-empty-"+newID(), func(_ *domain.Job) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("StreamJobs on empty project: %v", err)
	}
	if called {
		t.Fatal("callback should not be invoked when project has no jobs")
	}
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
		if err := q.CreateRun(ctx, r); err != nil {
			t.Fatalf("CreateRun: %v", err)
		}
		runIDs = append(runIDs, r.ID)
	}

	// Read back the middle run's created_at to use as the window bound.
	middle, err := q.GetRun(ctx, runIDs[1])
	if err != nil {
		t.Fatalf("GetRun middle: %v", err)
	}

	// Window [middle.created_at, middle.created_at] should return exactly
	// the middle run. The query uses created_at >= $2 AND created_at <= $3
	// so both ends are inclusive.
	var visited []string
	if err := q.StreamRuns(ctx, projectID, middle.CreatedAt, middle.CreatedAt, func(r *domain.JobRun) error {
		visited = append(visited, r.ID)
		return nil
	}); err != nil {
		t.Fatalf("StreamRuns (point window): %v", err)
	}
	if len(visited) != 1 || visited[0] != middle.ID {
		t.Fatalf("point window visited = %v, want [%q]", visited, middle.ID)
	}

	// Wide window returns all three.
	visited = nil
	if err := q.StreamRuns(ctx, projectID,
		middle.CreatedAt.Add(-time.Hour),
		middle.CreatedAt.Add(time.Hour),
		func(r *domain.JobRun) error {
			visited = append(visited, r.ID)
			return nil
		}); err != nil {
		t.Fatalf("StreamRuns (wide window): %v", err)
	}
	if len(visited) != 3 {
		t.Fatalf("wide window visited %d, want 3", len(visited))
	}
}

func TestStreamRuns_CallbackError_StopsIteration(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-stream-runs-err-" + newID()
	job := mustCreateJob(t, ctx, q, projectID)
	for range 5 {
		r := baseRun(job, newID())
		if err := q.CreateRun(ctx, r); err != nil {
			t.Fatalf("CreateRun: %v", err)
		}
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
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
	if visited != 2 {
		t.Fatalf("visited = %d, want 2", visited)
	}
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
		if err := q.CreateWorkflow(ctx, wf); err != nil {
			t.Fatalf("CreateWorkflow: %v", err)
		}
		created[wf.ID] = true
	}

	var visited []string
	if err := q.StreamWorkflows(ctx, projectID, func(w *domain.Workflow) error {
		visited = append(visited, w.ID)
		return nil
	}); err != nil {
		t.Fatalf("StreamWorkflows: %v", err)
	}
	if len(visited) != want {
		t.Fatalf("streamed %d workflows, want %d", len(visited), want)
	}
	for _, id := range visited {
		if !created[id] {
			t.Fatalf("unexpected workflow %q", id)
		}
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
		if err := q.CreateProject(ctx, p); err != nil {
			t.Fatalf("CreateProject A: %v", err)
		}
	}
	for range 2 {
		p := &domain.Project{
			ID:    "proj-b-" + newID(),
			Name:  "B",
			OrgID: orgB,
		}
		if err := q.CreateProject(ctx, p); err != nil {
			t.Fatalf("CreateProject B: %v", err)
		}
	}

	countA, err := q.CountProjectsByOrg(ctx, orgA)
	if err != nil {
		t.Fatalf("CountProjectsByOrg A: %v", err)
	}
	if countA != 3 {
		t.Fatalf("org A count = %d, want 3", countA)
	}

	countB, err := q.CountProjectsByOrg(ctx, orgB)
	if err != nil {
		t.Fatalf("CountProjectsByOrg B: %v", err)
	}
	if countB != 2 {
		t.Fatalf("org B count = %d, want 2", countB)
	}

	countEmpty, err := q.CountProjectsByOrg(ctx, "org-empty-"+newID())
	if err != nil {
		t.Fatalf("CountProjectsByOrg empty: %v", err)
	}
	if countEmpty != 0 {
		t.Fatalf("empty org count = %d, want 0", countEmpty)
	}
}

func TestCountProjectsByOrg_SoftDeletedExcluded(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-soft-" + newID()

	alive := &domain.Project{ID: "proj-alive-" + newID(), Name: "alive", OrgID: orgID}
	if err := q.CreateProject(ctx, alive); err != nil {
		t.Fatalf("CreateProject alive: %v", err)
	}
	dead := &domain.Project{ID: "proj-dead-" + newID(), Name: "dead", OrgID: orgID}
	if err := q.CreateProject(ctx, dead); err != nil {
		t.Fatalf("CreateProject dead: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE projects SET deleted_at = NOW() WHERE id = $1`, dead.ID,
	); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	count, err := q.CountProjectsByOrg(ctx, orgID)
	if err != nil {
		t.Fatalf("CountProjectsByOrg: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (soft-deleted should be excluded)", count)
	}
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
	if err := q.CreateLogDrain(ctx, drain); err != nil {
		t.Fatalf("CreateLogDrain: %v", err)
	}

	// Correct project can fetch it.
	got, err := q.GetLogDrain(ctx, drain.ID, projA)
	if err != nil {
		t.Fatalf("GetLogDrain(correct project): %v", err)
	}
	if got.ID != drain.ID {
		t.Fatalf("got %q, want %q", got.ID, drain.ID)
	}

	// Wrong project returns not-found.
	if _, err := q.GetLogDrain(ctx, drain.ID, projB); !errors.Is(err, store.ErrLogDrainNotFound) {
		t.Fatalf("cross-tenant GetLogDrain: err = %v, want ErrLogDrainNotFound", err)
	}
}
