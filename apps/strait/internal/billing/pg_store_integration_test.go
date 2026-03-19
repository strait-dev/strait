//go:build integration

package billing_test

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

var testDB *testutil.TestDB

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testDB, err = testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		log.Fatalf("setup test db: %v", err)
	}

	code := m.Run()
	testDB.Cleanup(ctx)
	os.Exit(code)
}

func mustQueries(t *testing.T) *store.Queries {
	t.Helper()

	if testDB == nil || testDB.Pool == nil {
		t.Fatal("testDB is not initialized")
	}

	return store.New(testDB.Pool)
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()

	if err := testDB.CleanTables(ctx); err != nil {
		t.Fatalf("clean tables: %v", err)
	}
}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func createProject(t *testing.T, ctx context.Context, q *store.Queries, orgID, name string) *domain.Project {
	t.Helper()

	project := &domain.Project{
		ID:    newID(),
		OrgID: orgID,
		Name:  name,
	}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	return project
}

func createJob(t *testing.T, ctx context.Context, q *store.Queries, projectID string) *domain.Job {
	t.Helper()

	job := &domain.Job{
		ID:            newID(),
		ProjectID:     projectID,
		Name:          "job-" + newID(),
		Slug:          "slug-" + newID(),
		Description:   "job description",
		Cron:          "*/5 * * * *",
		PayloadSchema: []byte(`{"type":"object"}`),
		EndpointURL:   "https://example.com/webhook",
		MaxAttempts:   5,
		TimeoutSecs:   120,
		Enabled:       true,
		WebhookURL:    "https://example.com/callback",
		WebhookSecret: "secret",
	}
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	return job
}

func createRun(t *testing.T, ctx context.Context, q *store.Queries, job *domain.Job, status domain.RunStatus) *domain.JobRun {
	t.Helper()

	run := &domain.JobRun{
		ID:            newID(),
		JobID:         job.ID,
		ProjectID:     job.ProjectID,
		Status:        status,
		Attempt:       1,
		Payload:       []byte(`{"hello":"world"}`),
		TriggeredBy:   domain.TriggerManual,
		Priority:      0,
		ExecutionMode: domain.ExecutionModeHTTP,
	}
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	return run
}

func createMember(t *testing.T, ctx context.Context, q *store.Queries, projectID, userID string) {
	t.Helper()

	role := &domain.ProjectRole{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "member-" + newID(),
		Permissions: []string{"jobs:read"},
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}

	member := &domain.ProjectMemberRole{
		ID:        newID(),
		ProjectID: projectID,
		UserID:    userID,
		RoleID:    role.ID,
		GrantedBy: "tester",
	}
	if err := q.AssignMemberRole(ctx, member); err != nil {
		t.Fatalf("AssignMemberRole() error = %v", err)
	}
}

func TestPgStore_AggregatesComputeAndAIUsage(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustQueries(t)
	pgStore := billing.NewPgStore(testDB.Pool)

	orgID := "org-usage"
	projectA := createProject(t, ctx, q, orgID, "Project A")
	projectB := createProject(t, ctx, q, orgID, "Project B")
	_ = createProject(t, ctx, q, "org-other", "Project Other")

	jobA := createJob(t, ctx, q, projectA.ID)
	jobB := createJob(t, ctx, q, projectB.ID)

	day1 := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)

	runA1 := createRun(t, ctx, q, jobA, domain.StatusCompleted)
	runB1 := createRun(t, ctx, q, jobB, domain.StatusCompleted)
	runA2 := createRun(t, ctx, q, jobA, domain.StatusCompleted)
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET created_at = $2 WHERE id = $1`, runA1.ID, day1); err != nil {
		t.Fatalf("set runA1 created_at error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET created_at = $2 WHERE id = $1`, runB1.ID, day1); err != nil {
		t.Fatalf("set runB1 created_at error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET created_at = $2 WHERE id = $1`, runA2.ID, day2); err != nil {
		t.Fatalf("set runA2 created_at error = %v", err)
	}

	computeA1 := &domain.RunComputeUsage{
		ID:            newID(),
		RunID:         runA1.ID,
		ProjectID:     projectA.ID,
		JobID:         jobA.ID,
		MachinePreset: "micro",
		MachineID:     "machine-a1",
		DurationSecs:  30,
		CostMicrousd:  2_000_000,
	}
	if err := q.CreateRunComputeUsage(ctx, computeA1); err != nil {
		t.Fatalf("CreateRunComputeUsage(projectA/day1) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE run_compute_usage SET created_at = $2 WHERE id = $1`, computeA1.ID, day1); err != nil {
		t.Fatalf("set computeA1 created_at error = %v", err)
	}

	aiA1 := &domain.RunUsage{
		ID:               newID(),
		RunID:            runA1.ID,
		Provider:         "openai",
		Model:            "gpt-5.4-mini",
		PromptTokens:     600,
		CompletionTokens: 400,
		TotalTokens:      1000,
		CostMicrousd:     1_000_000,
	}
	if err := q.CreateRunUsage(ctx, aiA1); err != nil {
		t.Fatalf("CreateRunUsage(projectA/day1) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE run_usage SET created_at = $2 WHERE id = $1`, aiA1.ID, day1); err != nil {
		t.Fatalf("set aiA1 created_at error = %v", err)
	}

	computeB1 := &domain.RunComputeUsage{
		ID:            newID(),
		RunID:         runB1.ID,
		ProjectID:     projectB.ID,
		JobID:         jobB.ID,
		MachinePreset: "small",
		MachineID:     "machine-b1",
		DurationSecs:  45,
		CostMicrousd:  3_000_000,
	}
	if err := q.CreateRunComputeUsage(ctx, computeB1); err != nil {
		t.Fatalf("CreateRunComputeUsage(projectB/day1) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE run_compute_usage SET created_at = $2 WHERE id = $1`, computeB1.ID, day1); err != nil {
		t.Fatalf("set computeB1 created_at error = %v", err)
	}

	aiB1 := &domain.RunUsage{
		ID:               newID(),
		RunID:            runB1.ID,
		Provider:         "openai",
		Model:            "gpt-5.4-mini",
		PromptTokens:     300,
		CompletionTokens: 400,
		TotalTokens:      700,
		CostMicrousd:     500_000,
	}
	if err := q.CreateRunUsage(ctx, aiB1); err != nil {
		t.Fatalf("CreateRunUsage(projectB/day1) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE run_usage SET created_at = $2 WHERE id = $1`, aiB1.ID, day1); err != nil {
		t.Fatalf("set aiB1 created_at error = %v", err)
	}

	aiA2 := &domain.RunUsage{
		ID:               newID(),
		RunID:            runA2.ID,
		Provider:         "openai",
		Model:            "gpt-5.4-mini",
		PromptTokens:     100,
		CompletionTokens: 200,
		TotalTokens:      300,
		CostMicrousd:     250_000,
	}
	if err := q.CreateRunUsage(ctx, aiA2); err != nil {
		t.Fatalf("CreateRunUsage(projectA/day2) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE run_usage SET created_at = $2 WHERE id = $1`, aiA2.ID, day2); err != nil {
		t.Fatalf("set aiA2 created_at error = %v", err)
	}

	orgRecords, err := pgStore.GetOrgUsageForPeriod(ctx, orgID, day1.Add(-time.Hour), day2.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetOrgUsageForPeriod() error = %v", err)
	}
	if len(orgRecords) != 3 {
		t.Fatalf("GetOrgUsageForPeriod() len = %d, want 3", len(orgRecords))
	}

	recordMap := make(map[string]billing.UsageRecord, len(orgRecords))
	for _, record := range orgRecords {
		key := record.ProjectID + ":" + record.PeriodDate.Format("2006-01-02")
		recordMap[key] = record
	}

	day1A := recordMap[projectA.ID+":2026-03-10"]
	if day1A.RunsCount != 1 || day1A.ComputeCostMicro != 2_000_000 || day1A.AITokensTotal != 1000 || day1A.AICostMicro != 1_000_000 {
		t.Fatalf("unexpected project A day 1 aggregate: %+v", day1A)
	}
	day1B := recordMap[projectB.ID+":2026-03-10"]
	if day1B.RunsCount != 1 || day1B.ComputeCostMicro != 3_000_000 || day1B.AITokensTotal != 700 || day1B.AICostMicro != 500_000 {
		t.Fatalf("unexpected project B day 1 aggregate: %+v", day1B)
	}
	day2A := recordMap[projectA.ID+":2026-03-11"]
	if day2A.RunsCount != 1 || day2A.ComputeCostMicro != 0 || day2A.AITokensTotal != 300 || day2A.AICostMicro != 250_000 {
		t.Fatalf("unexpected project A day 2 aggregate: %+v", day2A)
	}

	projectARecords, err := pgStore.GetProjectUsageForPeriod(ctx, projectA.ID, day1.Add(-time.Hour), day2.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetProjectUsageForPeriod() error = %v", err)
	}
	if len(projectARecords) != 2 {
		t.Fatalf("GetProjectUsageForPeriod() len = %d, want 2", len(projectARecords))
	}

	day1Records, err := pgStore.GetOrgDailyUsage(ctx, orgID, day1)
	if err != nil {
		t.Fatalf("GetOrgDailyUsage() error = %v", err)
	}
	if len(day1Records) != 2 {
		t.Fatalf("GetOrgDailyUsage() len = %d, want 2", len(day1Records))
	}
}

func TestPgStore_CountMembersAndExecutingRunsByOrg(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustQueries(t)
	pgStore := billing.NewPgStore(testDB.Pool)

	projectA := createProject(t, ctx, q, "org-members", "Project A")
	projectB := createProject(t, ctx, q, "org-members", "Project B")
	projectOther := createProject(t, ctx, q, "org-other", "Project Other")

	createMember(t, ctx, q, projectA.ID, "user-1")
	createMember(t, ctx, q, projectB.ID, "user-1")
	createMember(t, ctx, q, projectB.ID, "user-2")
	createMember(t, ctx, q, projectOther.ID, "user-3")

	members, err := pgStore.CountMembersByOrg(ctx, "org-members")
	if err != nil {
		t.Fatalf("CountMembersByOrg() error = %v", err)
	}
	if members != 2 {
		t.Fatalf("CountMembersByOrg() = %d, want 2", members)
	}

	jobA := createJob(t, ctx, q, projectA.ID)
	jobB := createJob(t, ctx, q, projectB.ID)
	jobOther := createJob(t, ctx, q, projectOther.ID)

	_ = createRun(t, ctx, q, jobA, domain.StatusExecuting)
	_ = createRun(t, ctx, q, jobA, domain.StatusCompleted)
	_ = createRun(t, ctx, q, jobB, domain.StatusExecuting)
	_ = createRun(t, ctx, q, jobOther, domain.StatusExecuting)

	executing, err := pgStore.CountExecutingRunsByOrg(ctx, "org-members")
	if err != nil {
		t.Fatalf("CountExecutingRunsByOrg() error = %v", err)
	}
	if executing != 2 {
		t.Fatalf("CountExecutingRunsByOrg() = %d, want 2", executing)
	}
}
