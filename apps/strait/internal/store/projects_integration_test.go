//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"
)

func TestCreateProject(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	p := &domain.Project{
		ID:    newID(),
		OrgID: "org-proj-create",
		Name:  "Test Project",
	}

	if err := st.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	got, err := st.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if got.ID != p.ID {
		t.Fatalf("ID = %q, want %q", got.ID, p.ID)
	}
	if got.OrgID != p.OrgID {
		t.Fatalf("OrgID = %q, want %q", got.OrgID, p.OrgID)
	}
	if got.Name != p.Name {
		t.Fatalf("Name = %q, want %q", got.Name, p.Name)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should not be zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt should not be zero")
	}
}

func TestCreateProject_Upsert(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	id := newID()
	p := &domain.Project{ID: id, OrgID: "org-upsert", Name: "Original"}
	if err := st.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	p2 := &domain.Project{ID: id, OrgID: "org-upsert", Name: "Updated"}
	if err := st.CreateProject(ctx, p2); err != nil {
		t.Fatalf("CreateProject() upsert error = %v", err)
	}

	got, err := st.GetProject(ctx, id)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if got.Name != "Updated" {
		t.Fatalf("Name = %q, want %q", got.Name, "Updated")
	}
}

func TestCreateProject_UpsertPreservesOrgID(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	id := newID()
	p := &domain.Project{ID: id, OrgID: "org-preserve", Name: "First"}
	if err := st.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	// Upsert with empty org_id should preserve existing.
	p2 := &domain.Project{ID: id, OrgID: "", Name: "Second"}
	if err := st.CreateProject(ctx, p2); err != nil {
		t.Fatalf("CreateProject() upsert error = %v", err)
	}

	got, err := st.GetProject(ctx, id)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if got.OrgID != "org-preserve" {
		t.Fatalf("OrgID = %q, want %q (should be preserved)", got.OrgID, "org-preserve")
	}
}

func TestGetProject_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	_, err := st.GetProject(ctx, "nonexistent-project")
	if !errors.Is(err, store.ErrProjectNotFound) {
		t.Fatalf("GetProject() error = %v, want ErrProjectNotFound", err)
	}
}

func TestListProjectsByOrg(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	orgA := "org-list-a"
	orgB := "org-list-b"

	for i, org := range []string{orgA, orgA, orgB} {
		p := &domain.Project{ID: newID(), OrgID: org, Name: "Project " + string(rune('A'+i))}
		if err := st.CreateProject(ctx, p); err != nil {
			t.Fatalf("CreateProject(%d) error = %v", i, err)
		}
	}

	got, err := st.ListProjectsByOrg(ctx, orgA)
	if err != nil {
		t.Fatalf("ListProjectsByOrg() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	for _, p := range got {
		if p.OrgID != orgA {
			t.Fatalf("OrgID = %q, want %q", p.OrgID, orgA)
		}
	}
}

func TestListProjectsByOrg_Empty(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	got, err := st.ListProjectsByOrg(ctx, "org-empty")
	if err != nil {
		t.Fatalf("ListProjectsByOrg() error = %v", err)
	}
	if got != nil && len(got) != 0 {
		t.Fatalf("expected empty slice, got %d items", len(got))
	}
}

func TestDeleteProject(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	p := &domain.Project{ID: newID(), OrgID: "org-delete", Name: "Delete Me"}
	if err := st.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if err := st.DeleteProject(ctx, p.ID); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	_, err := st.GetProject(ctx, p.ID)
	if !errors.Is(err, store.ErrProjectNotFound) {
		t.Fatalf("GetProject() after delete error = %v, want ErrProjectNotFound", err)
	}

	count, err := st.CountProjectsByOrg(ctx, p.OrgID)
	if err != nil {
		t.Fatalf("CountProjectsByOrg() after delete error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count after delete = %d, want 0", count)
	}

	projects, err := st.ListProjectsByOrg(ctx, p.OrgID)
	if err != nil {
		t.Fatalf("ListProjectsByOrg() after delete error = %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("ListProjectsByOrg() after delete len = %d, want 0", len(projects))
	}
}

func TestDeleteProject_RecreateRevivesProject(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	projectID := newID()
	initial := &domain.Project{ID: projectID, OrgID: "org-revive", Name: "Original"}
	if err := st.CreateProject(ctx, initial); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := st.DeleteProject(ctx, projectID); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	recreated := &domain.Project{ID: projectID, OrgID: "org-revive", Name: "Recreated"}
	if err := st.CreateProject(ctx, recreated); err != nil {
		t.Fatalf("CreateProject() recreate error = %v", err)
	}

	got, err := st.GetProject(ctx, projectID)
	if err != nil {
		t.Fatalf("GetProject() after recreate error = %v", err)
	}
	if got.Name != "Recreated" {
		t.Fatalf("Name after recreate = %q, want %q", got.Name, "Recreated")
	}

	count, err := st.CountProjectsByOrg(ctx, "org-revive")
	if err != nil {
		t.Fatalf("CountProjectsByOrg() after recreate error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count after recreate = %d, want 1", count)
	}
}

func TestDeleteProject_PreservesHistoricalBillingUsage(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	st := mustStore(t)
	billingStore := billing.NewPgStore(testDB.Pool)

	project := &domain.Project{ID: newID(), OrgID: "org-history", Name: "History Project"}
	if err := st.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, st, project.ID)
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := st.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	runCreatedAt := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET created_at = $2 WHERE id = $1`, run.ID, runCreatedAt); err != nil {
		t.Fatalf("set run created_at error = %v", err)
	}

	computeUsage := &domain.RunComputeUsage{
		ID:            newID(),
		RunID:         run.ID,
		ProjectID:     project.ID,
		JobID:         job.ID,
		MachinePreset: "micro",
		MachineID:     "machine-history",
		DurationSecs:  10,
		CostMicrousd:  2_000_000,
	}
	if err := st.CreateRunComputeUsage(ctx, computeUsage); err != nil {
		t.Fatalf("CreateRunComputeUsage() error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE run_compute_usage SET created_at = $2 WHERE id = $1`, computeUsage.ID, runCreatedAt); err != nil {
		t.Fatalf("set compute usage created_at error = %v", err)
	}

	runUsage := &domain.RunUsage{
		ID:               newID(),
		RunID:            run.ID,
		Provider:         "openai",
		Model:            "gpt-5",
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		CostMicrousd:     500_000,
	}
	if err := st.CreateRunUsage(ctx, runUsage); err != nil {
		t.Fatalf("CreateRunUsage() error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE run_usage SET created_at = $2 WHERE id = $1`, runUsage.ID, runCreatedAt); err != nil {
		t.Fatalf("set run usage created_at error = %v", err)
	}

	if err := st.DeleteProject(ctx, project.ID); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	periodDate := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	orgUsage, err := billingStore.GetOrgUsageForPeriod(ctx, project.OrgID, periodDate, periodDate)
	if err != nil {
		t.Fatalf("GetOrgUsageForPeriod() error = %v", err)
	}
	if len(orgUsage) != 1 {
		t.Fatalf("GetOrgUsageForPeriod() len = %d, want 1", len(orgUsage))
	}
	if orgUsage[0].ProjectID != project.ID {
		t.Fatalf("usage project_id = %q, want %q", orgUsage[0].ProjectID, project.ID)
	}
	if orgUsage[0].ComputeCostMicro != 2_000_000 || orgUsage[0].AICostMicro != 500_000 {
		t.Fatalf("usage aggregate = %+v, want compute=2000000 ai=500000", orgUsage[0])
	}

	spend, err := billingStore.SumOrgPeriodSpend(ctx, project.OrgID, periodDate)
	if err != nil {
		t.Fatalf("SumOrgPeriodSpend() error = %v", err)
	}
	if spend != 2_000_000 {
		t.Fatalf("SumOrgPeriodSpend() = %d, want 2000000", spend)
	}

	executing, err := billingStore.CountExecutingRunsByOrg(ctx, project.OrgID)
	if err != nil {
		t.Fatalf("CountExecutingRunsByOrg() error = %v", err)
	}
	if executing != 1 {
		t.Fatalf("CountExecutingRunsByOrg() = %d, want 1", executing)
	}
}

func TestDeleteProject_NonExistent(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	err := st.DeleteProject(ctx, "nonexistent-project")
	if !errors.Is(err, store.ErrProjectNotFound) {
		t.Fatalf("DeleteProject(nonexistent) error = %v, want ErrProjectNotFound", err)
	}
}

func TestCountProjectsByOrg(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	org := "org-count"
	for i := 0; i < 3; i++ {
		p := &domain.Project{ID: newID(), OrgID: org, Name: "P"}
		if err := st.CreateProject(ctx, p); err != nil {
			t.Fatalf("CreateProject(%d) error = %v", i, err)
		}
	}
	// Different org
	p := &domain.Project{ID: newID(), OrgID: "org-other", Name: "P"}
	if err := st.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject(other) error = %v", err)
	}

	count, err := st.CountProjectsByOrg(ctx, org)
	if err != nil {
		t.Fatalf("CountProjectsByOrg() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}

	count2, err := st.CountProjectsByOrg(ctx, "org-other")
	if err != nil {
		t.Fatalf("CountProjectsByOrg(other) error = %v", err)
	}
	if count2 != 1 {
		t.Fatalf("count = %d, want 1", count2)
	}
}
