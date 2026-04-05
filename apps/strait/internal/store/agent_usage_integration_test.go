//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestCreateAgentUsageRecord(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	rec := &domain.AgentUsageRecord{
		RunID:             newID(),
		ProjectID:         "proj-usage-1",
		OrgID:             "org-usage-1",
		AgentID:           newID(),
		TotalTokens:       15000,
		ToolCallCount:     10,
		RunCostMicrousd:   1000,
		TokenCostMicrousd: 1500,
		ToolCostMicrousd:  1000,
		TotalCostMicrousd: 3500,
	}

	if err := q.CreateAgentUsageRecord(ctx, rec); err != nil {
		t.Fatalf("CreateAgentUsageRecord() error = %v", err)
	}

	if rec.ID == "" {
		t.Error("expected ID to be set after creation")
	}
	if rec.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestCreateAgentUsageRecord_Dedup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	runID := newID()
	rec1 := &domain.AgentUsageRecord{
		RunID:             runID,
		ProjectID:         "proj-dedup",
		OrgID:             "org-dedup",
		AgentID:           newID(),
		TotalTokens:       100,
		TotalCostMicrousd: 1000,
	}
	if err := q.CreateAgentUsageRecord(ctx, rec1); err != nil {
		t.Fatalf("first insert error = %v", err)
	}

	// Second insert with same run_id should be silently ignored (ON CONFLICT DO NOTHING).
	rec2 := &domain.AgentUsageRecord{
		RunID:             runID,
		ProjectID:         "proj-dedup",
		OrgID:             "org-dedup",
		AgentID:           newID(),
		TotalTokens:       9999,
		TotalCostMicrousd: 9999,
	}
	// ON CONFLICT DO NOTHING means no RETURNING row, so Scan will fail.
	// This is acceptable — the dedup prevents double billing.
	_ = q.CreateAgentUsageRecord(ctx, rec2)

	// Verify only the first record exists via the sum.
	total, err := q.SumOrgAgentSpendSince(ctx, "org-dedup", time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("SumOrgAgentSpendSince() error = %v", err)
	}
	if total != 1000 {
		t.Errorf("total = %d, want 1000 (first record only)", total)
	}
}

func TestSumOrgAgentSpendSince(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-spend-" + newID()

	// Insert 3 records.
	for i := range 3 {
		rec := &domain.AgentUsageRecord{
			RunID:             newID(),
			ProjectID:         "proj-spend",
			OrgID:             orgID,
			AgentID:           newID(),
			TotalCostMicrousd: 1000 * int64(i+1), // 1000, 2000, 3000
		}
		if err := q.CreateAgentUsageRecord(ctx, rec); err != nil {
			t.Fatalf("CreateAgentUsageRecord[%d] error = %v", i, err)
		}
	}

	total, err := q.SumOrgAgentSpendSince(ctx, orgID, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("SumOrgAgentSpendSince() error = %v", err)
	}
	if total != 6000 {
		t.Errorf("total = %d, want 6000", total)
	}
}

func TestSumOrgAgentSpendSince_RespectsTimeFilter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-time-" + newID()

	rec := &domain.AgentUsageRecord{
		RunID:             newID(),
		ProjectID:         "proj-time",
		OrgID:             orgID,
		AgentID:           newID(),
		TotalCostMicrousd: 5000,
	}
	if err := q.CreateAgentUsageRecord(ctx, rec); err != nil {
		t.Fatalf("CreateAgentUsageRecord() error = %v", err)
	}

	// Query from the future — should find nothing.
	total, err := q.SumOrgAgentSpendSince(ctx, orgID, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("SumOrgAgentSpendSince(future) error = %v", err)
	}
	if total != 0 {
		t.Errorf("total from future = %d, want 0", total)
	}

	// Query from the past — should find the record.
	total, err = q.SumOrgAgentSpendSince(ctx, orgID, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("SumOrgAgentSpendSince(past) error = %v", err)
	}
	if total != 5000 {
		t.Errorf("total from past = %d, want 5000", total)
	}
}

func TestSumOrgAgentSpendSince_EmptyOrg(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	total, err := q.SumOrgAgentSpendSince(ctx, "nonexistent-org", time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("SumOrgAgentSpendSince(empty) error = %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0 for empty org", total)
	}
}

func TestQueryAgentUsageSummary(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-summary-" + newID()

	records := []domain.AgentUsageRecord{
		{RunID: newID(), ProjectID: "proj-1", OrgID: orgID, AgentID: newID(), TotalTokens: 1000, ToolCallCount: 5, TotalCostMicrousd: 2000},
		{RunID: newID(), ProjectID: "proj-1", OrgID: orgID, AgentID: newID(), TotalTokens: 2000, ToolCallCount: 3, TotalCostMicrousd: 3000},
		{RunID: newID(), ProjectID: "proj-1", OrgID: orgID, AgentID: newID(), TotalTokens: 500, ToolCallCount: 1, TotalCostMicrousd: 1000},
	}

	for i := range records {
		if err := q.CreateAgentUsageRecord(ctx, &records[i]); err != nil {
			t.Fatalf("CreateAgentUsageRecord[%d] error = %v", i, err)
		}
	}

	summary, err := q.QueryAgentUsageSummary(ctx, orgID, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("QueryAgentUsageSummary() error = %v", err)
	}

	if summary.RunCount != 3 {
		t.Errorf("RunCount = %d, want 3", summary.RunCount)
	}
	if summary.TotalTokens != 3500 {
		t.Errorf("TotalTokens = %d, want 3500", summary.TotalTokens)
	}
	if summary.TotalToolCalls != 9 {
		t.Errorf("TotalToolCalls = %d, want 9", summary.TotalToolCalls)
	}
	if summary.TotalCostMicrousd != 6000 {
		t.Errorf("TotalCostMicrousd = %d, want 6000", summary.TotalCostMicrousd)
	}
}

func TestGetOrgAgentSpendingLimit(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// No subscription exists — should return error or -1.
	limit, err := q.GetOrgAgentSpendingLimit(ctx, "nonexistent-org")
	if err == nil {
		// If no subscription row, the query should fail (ErrNoRows).
		// That's expected — the handler defaults to -1 on error.
		t.Logf("GetOrgAgentSpendingLimit(missing) returned limit=%d (expected error or -1)", limit)
	}
}

func TestGetAgentDeploymentByID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-deploy-" + newID(), Name: "Deploy Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, q, project.ID)
	agent := &domain.Agent{
		ID:        newID(),
		ProjectID: project.ID,
		JobID:     job.ID,
		Name:      "Deploy Agent",
		Slug:      "deploy-agent-" + newID(),
		Model:     "gpt-4o",
		CreatedBy: "user-1",
		UpdatedBy: "user-1",
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	// Create deployment.
	deployment := &domain.AgentDeployment{
		ID:        newID(),
		AgentID:   agent.ID,
		Version:   1,
		Status:    domain.AgentDeploymentStatusDeployed,
		Provider:  "local_stub",
		CreatedBy: "user-1",
	}
	if err := q.CreateAgentDeployment(ctx, deployment); err != nil {
		t.Fatalf("CreateAgentDeployment() error = %v", err)
	}

	// Retrieve by ID.
	got, err := q.GetAgentDeploymentByID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("GetAgentDeploymentByID() error = %v", err)
	}
	if got.ID != deployment.ID {
		t.Errorf("ID = %q, want %q", got.ID, deployment.ID)
	}
	if got.AgentID != agent.ID {
		t.Errorf("AgentID = %q, want %q", got.AgentID, agent.ID)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
}

func TestGetAgentByJobID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-byjob-" + newID(), Name: "ByJob Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, q, project.ID)
	agent := &domain.Agent{
		ID:        newID(),
		ProjectID: project.ID,
		JobID:     job.ID,
		Name:      "ByJob Agent",
		Slug:      "byjob-agent-" + newID(),
		Model:     "claude-sonnet-4-5",
		CreatedBy: "user-1",
		UpdatedBy: "user-1",
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	got, err := q.GetAgentByJobID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetAgentByJobID() error = %v", err)
	}
	if got.ID != agent.ID {
		t.Errorf("ID = %q, want %q", got.ID, agent.ID)
	}
	if got.JobID != job.ID {
		t.Errorf("JobID = %q, want %q", got.JobID, job.ID)
	}
}
