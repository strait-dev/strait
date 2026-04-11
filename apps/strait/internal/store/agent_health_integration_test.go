//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
)

// TestGetAgentHealthStats_SingleCTE_MatchesExpectedValues pins the
// shape of the Phase F2 CTE rewrite. Constructs a known dataset and
// asserts every returned field matches the hand-computed expected
// value. Acts as a regression net for the 4-query-to-1-CTE collapse —
// any drift in the aggregate math trips this test.
func TestGetAgentHealthStats_SingleCTE_MatchesExpectedValues(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-health", Name: "Health Test"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	job := mustCreateJob(t, ctx, q, project.ID)
	agent := &domain.Agent{
		ID:        newID(),
		ProjectID: project.ID,
		JobID:     job.ID,
		Name:      "Health Agent",
		Slug:      "health-" + newID(),
		Model:     "gpt-5.4",
		Config:    json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	// Dataset:
	//   3 completed runs (durations 2s, 4s, 6s -> avg 4)
	//   1 failed run with an oom error_class event
	//   1 failed run with a timeout error_class event
	//   1 system_failed run with no event
	// Expected: total=6, completed=3, failed=3, avg_duration≈4, oom=1, timeout=1
	now := time.Now().UTC()
	starts := []time.Time{now.Add(-10 * time.Second), now.Add(-8 * time.Second), now.Add(-6 * time.Second)}
	finishes := []time.Time{now.Add(-8 * time.Second), now.Add(-4 * time.Second), now.Add(0 * time.Second)}
	for i := range 3 {
		start := starts[i]
		finish := finishes[i]
		run := &domain.JobRun{
			ID: newID(), JobID: job.ID, ProjectID: project.ID,
			Status: domain.StatusCompleted, Attempt: 1,
			TriggeredBy: domain.TriggerManual,
			StartedAt:   &start,
			FinishedAt:  &finish,
		}
		if err := q.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun(completed %d) error = %v", i, err)
		}
	}

	oomRun := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: project.ID,
		Status: domain.StatusFailed, Attempt: 1,
		TriggeredBy: domain.TriggerManual,
	}
	if err := q.CreateRun(ctx, oomRun); err != nil {
		t.Fatalf("CreateRun(oom) error = %v", err)
	}
	if err := q.InsertEvent(ctx, &domain.RunEvent{
		RunID: oomRun.ID, Type: "error", Level: "error",
		Message: "oom", Data: mustJSONForTest(map[string]any{"error_class": "oom"}),
	}); err != nil {
		t.Fatalf("InsertEvent(oom) error = %v", err)
	}

	timeoutRun := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: project.ID,
		Status: domain.StatusFailed, Attempt: 1,
		TriggeredBy: domain.TriggerManual,
	}
	if err := q.CreateRun(ctx, timeoutRun); err != nil {
		t.Fatalf("CreateRun(timeout) error = %v", err)
	}
	if err := q.InsertEvent(ctx, &domain.RunEvent{
		RunID: timeoutRun.ID, Type: "error", Level: "error",
		Message: "timeout", Data: mustJSONForTest(map[string]any{"error_class": "timeout"}),
	}); err != nil {
		t.Fatalf("InsertEvent(timeout) error = %v", err)
	}

	systemFailedRun := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: project.ID,
		Status: domain.StatusSystemFailed, Attempt: 1,
		TriggeredBy: domain.TriggerManual,
	}
	if err := q.CreateRun(ctx, systemFailedRun); err != nil {
		t.Fatalf("CreateRun(system_failed) error = %v", err)
	}

	// One run_usage row for the oom run, cost 1000 microusd.
	if err := q.CreateRunUsage(ctx, &domain.RunUsage{
		RunID:            oomRun.ID,
		Provider:         "openai",
		Model:            "gpt-5.4",
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		CostMicrousd:     1000,
	}); err != nil {
		t.Fatalf("CreateRunUsage() error = %v", err)
	}

	stats, err := q.GetAgentHealthStats(ctx, agent.ID, now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("GetAgentHealthStats() error = %v", err)
	}

	if stats.TotalRuns != 6 {
		t.Errorf("TotalRuns = %d, want 6", stats.TotalRuns)
	}
	if stats.CompletedRuns != 3 {
		t.Errorf("CompletedRuns = %d, want 3", stats.CompletedRuns)
	}
	if stats.FailedRuns != 3 {
		// 2 failed + 1 system_failed
		t.Errorf("FailedRuns = %d, want 3 (2 failed + 1 system_failed)", stats.FailedRuns)
	}
	if stats.OOMRuns != 1 {
		t.Errorf("OOMRuns = %d, want 1", stats.OOMRuns)
	}
	if stats.TimeoutRuns != 1 {
		t.Errorf("TimeoutRuns = %d, want 1", stats.TimeoutRuns)
	}
	if stats.RateLimitedRuns != 0 {
		t.Errorf("RateLimitedRuns = %d, want 0", stats.RateLimitedRuns)
	}
	// Three completed durations: 2s, 4s, 6s -> avg 4s
	if stats.AvgDurationSecs < 3.5 || stats.AvgDurationSecs > 4.5 {
		t.Errorf("AvgDurationSecs = %f, want ~4.0", stats.AvgDurationSecs)
	}
	// One run_usage row with cost 1000 microusd. AvgCostMicrousd is a
	// float (the SQL AVG is COALESCE'd to 0 and cast to float via the
	// Go scan target). Expect ~1000.
	if stats.AvgCostMicrousd < 999 || stats.AvgCostMicrousd > 1001 {
		t.Errorf("AvgCostMicrousd = %f, want ~1000", stats.AvgCostMicrousd)
	}
	// Success rate: 3/6 = 50%
	if stats.SuccessRate < 49 || stats.SuccessRate > 51 {
		t.Errorf("SuccessRate = %f, want ~50", stats.SuccessRate)
	}
}

// TestGetAgentHealthStats_EmptyWindow_ReturnsZeroes checks the
// zero-runs path of the CTE (every CTE has zero rows and the
// CROSS JOIN still yields one output row with zeros).
func TestGetAgentHealthStats_EmptyWindow_ReturnsZeroes(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-health-empty", Name: "Empty"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	job := mustCreateJob(t, ctx, q, project.ID)
	agent := &domain.Agent{
		ID:        newID(),
		ProjectID: project.ID,
		JobID:     job.ID,
		Name:      "Empty Window Agent",
		Slug:      "empty-window-" + newID(),
		Model:     "gpt-5.4",
		Config:    json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	stats, err := q.GetAgentHealthStats(ctx, agent.ID, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("GetAgentHealthStats() error = %v", err)
	}
	if stats.TotalRuns != 0 {
		t.Errorf("TotalRuns = %d, want 0", stats.TotalRuns)
	}
	if stats.CompletedRuns != 0 || stats.FailedRuns != 0 {
		t.Errorf("non-zero run counts on empty window: completed=%d failed=%d", stats.CompletedRuns, stats.FailedRuns)
	}
	if stats.AvgDurationSecs != 0 {
		t.Errorf("AvgDurationSecs = %f, want 0", stats.AvgDurationSecs)
	}
	if stats.AvgCostMicrousd != 0 {
		t.Errorf("AvgCostMicrousd = %f, want 0", stats.AvgCostMicrousd)
	}
	if stats.HealthLevel != "unknown" {
		t.Errorf("HealthLevel = %q, want unknown", stats.HealthLevel)
	}
}

// TestGetAgentHealthStats_AgentNotFound propagates the agent lookup
// error, matching the pre-F2 behavior.
func TestGetAgentHealthStats_AgentNotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetAgentHealthStats(ctx, "agent-does-not-exist-"+newID(), time.Now().Add(-1*time.Hour))
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

// mustJSONForTest is a tiny helper to build a json.RawMessage for
// run event data in tests.
func mustJSONForTest(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}
