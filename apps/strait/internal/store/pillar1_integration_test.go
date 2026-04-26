//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// ---------- helpers ----------

func mustCreateAgentForPillar1(t *testing.T, ctx context.Context, q *store.Queries, suffix string) *domain.Agent {
	t.Helper()

	projectID := newID()
	project := &domain.Project{ID: projectID, OrgID: "org-pillar1-" + suffix, Name: "P1 Project " + suffix}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, q, projectID)

	agent := &domain.Agent{
		ID:        newID(),
		ProjectID: projectID,
		JobID:     job.ID,
		Name:      "agent-" + suffix,
		Slug:      "agent-slug-" + suffix + "-" + newID()[:8],
		Model:     "gpt-4o",
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	return agent
}

// ---------- Golden Sets ----------

func TestGoldenSetCRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	agent := mustCreateAgentForPillar1(t, ctx, q, "gs-crud")

	cases := json.RawMessage(`[{"id":"c1","input":{"q":"hello"},"expected_output":{"a":"hi"}},{"id":"c2","input":{"q":"bye"},"expected_output":{"a":"goodbye"}}]`)
	gs := &domain.GoldenSet{
		AgentID:   agent.ID,
		ProjectID: agent.ProjectID,
		Name:      "default",
		Cases:     cases,
	}
	if err := q.CreateGoldenSet(ctx, gs); err != nil {
		t.Fatalf("CreateGoldenSet() error = %v", err)
	}
	if gs.ID == "" {
		t.Fatal("CreateGoldenSet() did not populate ID")
	}
	if gs.CreatedAt.IsZero() {
		t.Fatal("CreateGoldenSet() did not populate CreatedAt")
	}

	// Get
	got, err := q.GetGoldenSet(ctx, agent.ID, "default")
	if err != nil {
		t.Fatalf("GetGoldenSet() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetGoldenSet() returned nil")
	}
	if got.ID != gs.ID {
		t.Fatalf("GetGoldenSet().ID = %q, want %q", got.ID, gs.ID)
	}
	if got.AgentID != agent.ID {
		t.Fatalf("GetGoldenSet().AgentID = %q, want %q", got.AgentID, agent.ID)
	}
	if got.ProjectID != agent.ProjectID {
		t.Fatalf("GetGoldenSet().ProjectID = %q, want %q", got.ProjectID, agent.ProjectID)
	}
	if got.Name != "default" {
		t.Fatalf("GetGoldenSet().Name = %q, want %q", got.Name, "default")
	}

	// List
	sets, err := q.ListGoldenSets(ctx, agent.ID)
	if err != nil {
		t.Fatalf("ListGoldenSets() error = %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("ListGoldenSets() count = %d, want 1", len(sets))
	}

	// Upsert (same name, new cases)
	updatedCases := json.RawMessage(`[{"id":"c3","input":{"q":"updated"},"expected_output":{"a":"yes"}}]`)
	gs2 := &domain.GoldenSet{
		AgentID:   agent.ID,
		ProjectID: agent.ProjectID,
		Name:      "default",
		Cases:     updatedCases,
	}
	if err := q.CreateGoldenSet(ctx, gs2); err != nil {
		t.Fatalf("CreateGoldenSet(upsert) error = %v", err)
	}
	if gs2.ID != gs.ID {
		t.Fatalf("Upsert should keep same ID: got %q, want %q", gs2.ID, gs.ID)
	}
	gotAfterUpsert, err := q.GetGoldenSet(ctx, agent.ID, "default")
	if err != nil {
		t.Fatalf("GetGoldenSet(after upsert) error = %v", err)
	}
	var parsedCases []json.RawMessage
	if err := json.Unmarshal(gotAfterUpsert.Cases, &parsedCases); err != nil {
		t.Fatalf("Unmarshal cases: %v", err)
	}
	if len(parsedCases) != 1 {
		t.Fatalf("After upsert, expected 1 case, got %d", len(parsedCases))
	}

	// Delete
	if err := q.DeleteGoldenSet(ctx, agent.ID, "default"); err != nil {
		t.Fatalf("DeleteGoldenSet() error = %v", err)
	}
	gotDeleted, err := q.GetGoldenSet(ctx, agent.ID, "default")
	if err != nil {
		t.Fatalf("GetGoldenSet(after delete) error = %v", err)
	}
	if gotDeleted != nil {
		t.Fatal("GetGoldenSet() should return nil after delete")
	}
}

func TestGoldenSetMultipleSets(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	agent := mustCreateAgentForPillar1(t, ctx, q, "gs-multi")

	names := []string{"alpha", "beta", "gamma"}
	for _, name := range names {
		gs := &domain.GoldenSet{
			AgentID:   agent.ID,
			ProjectID: agent.ProjectID,
			Name:      name,
			Cases:     json.RawMessage(`[{"id":"` + name + `"}]`),
		}
		if err := q.CreateGoldenSet(ctx, gs); err != nil {
			t.Fatalf("CreateGoldenSet(%s) error = %v", name, err)
		}
	}

	sets, err := q.ListGoldenSets(ctx, agent.ID)
	if err != nil {
		t.Fatalf("ListGoldenSets() error = %v", err)
	}
	if len(sets) != 3 {
		t.Fatalf("ListGoldenSets() count = %d, want 3", len(sets))
	}
	// Verify ordered by name
	if sets[0].Name != "alpha" || sets[1].Name != "beta" || sets[2].Name != "gamma" {
		t.Fatalf("ListGoldenSets() order = [%s, %s, %s], want [alpha, beta, gamma]",
			sets[0].Name, sets[1].Name, sets[2].Name)
	}

	// Delete one
	if err := q.DeleteGoldenSet(ctx, agent.ID, "beta"); err != nil {
		t.Fatalf("DeleteGoldenSet(beta) error = %v", err)
	}

	sets, err = q.ListGoldenSets(ctx, agent.ID)
	if err != nil {
		t.Fatalf("ListGoldenSets(after delete) error = %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("ListGoldenSets() count = %d, want 2", len(sets))
	}
}

// ---------- Model Routing ----------

func TestModelRoutingCRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	agent := mustCreateAgentForPillar1(t, ctx, q, "mr-crud")

	// Upsert "simple"
	route := &domain.ModelRoute{
		AgentID:   agent.ID,
		Tier:      "simple",
		Model:     "gpt-4o-mini",
		UpdatedBy: "system",
	}
	if err := q.UpsertModelRouting(ctx, route); err != nil {
		t.Fatalf("UpsertModelRouting(simple) error = %v", err)
	}
	if route.ID == "" {
		t.Fatal("UpsertModelRouting() did not populate ID")
	}
	if route.UpdatedAt.IsZero() {
		t.Fatal("UpsertModelRouting() did not populate UpdatedAt")
	}

	// GetModelRouting — 1 route
	routes, err := q.GetModelRouting(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetModelRouting() error = %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("GetModelRouting() count = %d, want 1", len(routes))
	}

	// GetModelRoutingByTier — "simple"
	byTier, err := q.GetModelRoutingByTier(ctx, agent.ID, "simple")
	if err != nil {
		t.Fatalf("GetModelRoutingByTier(simple) error = %v", err)
	}
	if byTier == nil {
		t.Fatal("GetModelRoutingByTier(simple) returned nil")
	}
	if byTier.Model != "gpt-4o-mini" {
		t.Fatalf("GetModelRoutingByTier().Model = %q, want %q", byTier.Model, "gpt-4o-mini")
	}
	if byTier.UpdatedBy != "system" {
		t.Fatalf("GetModelRoutingByTier().UpdatedBy = %q, want %q", byTier.UpdatedBy, "system")
	}

	// Upsert "complex"
	route2 := &domain.ModelRoute{
		AgentID:   agent.ID,
		Tier:      "complex",
		Model:     "gpt-4o",
		UpdatedBy: "system",
	}
	if err := q.UpsertModelRouting(ctx, route2); err != nil {
		t.Fatalf("UpsertModelRouting(complex) error = %v", err)
	}

	routes, err = q.GetModelRouting(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetModelRouting(2 routes) error = %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("GetModelRouting() count = %d, want 2", len(routes))
	}

	// Upsert "simple" with different model — verify update
	route3 := &domain.ModelRoute{
		AgentID:   agent.ID,
		Tier:      "simple",
		Model:     "claude-3-haiku",
		UpdatedBy: "autopilot",
	}
	if err := q.UpsertModelRouting(ctx, route3); err != nil {
		t.Fatalf("UpsertModelRouting(simple update) error = %v", err)
	}
	if route3.ID != route.ID {
		t.Fatalf("Upsert should keep same ID: got %q, want %q", route3.ID, route.ID)
	}
	updated, err := q.GetModelRoutingByTier(ctx, agent.ID, "simple")
	if err != nil {
		t.Fatalf("GetModelRoutingByTier(after update) error = %v", err)
	}
	if updated.Model != "claude-3-haiku" {
		t.Fatalf("After update, Model = %q, want %q", updated.Model, "claude-3-haiku")
	}
	if updated.UpdatedBy != "autopilot" {
		t.Fatalf("After update, UpdatedBy = %q, want %q", updated.UpdatedBy, "autopilot")
	}

	// Delete all routing
	if err := q.DeleteModelRouting(ctx, agent.ID); err != nil {
		t.Fatalf("DeleteModelRouting() error = %v", err)
	}
	routes, err = q.GetModelRouting(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetModelRouting(after delete) error = %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("GetModelRouting() count = %d, want 0 after delete", len(routes))
	}
}

func TestModelRoutingQualityScore(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	agent := mustCreateAgentForPillar1(t, ctx, q, "mr-quality")

	route := &domain.ModelRoute{
		AgentID:      agent.ID,
		Tier:         "standard",
		Model:        "gpt-4o",
		QualityScore: 90.5,
		UpdatedBy:    "system",
	}
	if err := q.UpsertModelRouting(ctx, route); err != nil {
		t.Fatalf("UpsertModelRouting() error = %v", err)
	}

	got, err := q.GetModelRoutingByTier(ctx, agent.ID, "standard")
	if err != nil {
		t.Fatalf("GetModelRoutingByTier() error = %v", err)
	}
	if got.QualityScore != 90.5 {
		t.Fatalf("QualityScore = %f, want 90.5", got.QualityScore)
	}

	// Upsert with previous_model
	route2 := &domain.ModelRoute{
		AgentID:       agent.ID,
		Tier:          "standard",
		Model:         "claude-3-opus",
		QualityScore:  92.0,
		PreviousModel: "gpt-4o",
		UpdatedBy:     "autopilot",
	}
	if err := q.UpsertModelRouting(ctx, route2); err != nil {
		t.Fatalf("UpsertModelRouting(with previous_model) error = %v", err)
	}

	got2, err := q.GetModelRoutingByTier(ctx, agent.ID, "standard")
	if err != nil {
		t.Fatalf("GetModelRoutingByTier(after update) error = %v", err)
	}
	if got2.PreviousModel != "gpt-4o" {
		t.Fatalf("PreviousModel = %q, want %q", got2.PreviousModel, "gpt-4o")
	}
	if got2.QualityScore != 92.0 {
		t.Fatalf("QualityScore = %f, want 92.0", got2.QualityScore)
	}
	if got2.Model != "claude-3-opus" {
		t.Fatalf("Model = %q, want %q", got2.Model, "claude-3-opus")
	}
}

// ---------- Cost Anomalies ----------

func TestCostAnomalyCRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	agent := mustCreateAgentForPillar1(t, ctx, q, "ca-crud")

	anomaly := &domain.CostAnomaly{
		AgentID:             agent.ID,
		ProjectID:           agent.ProjectID,
		DailyCostMicrousd:   500000,
		BaselineAvgMicrousd: 100000,
		Multiplier:          5.0,
		Threshold:           3.0,
		Status:              "open",
	}
	if err := q.CreateCostAnomaly(ctx, anomaly); err != nil {
		t.Fatalf("CreateCostAnomaly() error = %v", err)
	}
	if anomaly.ID == "" {
		t.Fatal("CreateCostAnomaly() did not populate ID")
	}
	if anomaly.DetectedAt.IsZero() {
		t.Fatal("CreateCostAnomaly() did not populate DetectedAt")
	}

	// List
	anomalies, err := q.ListCostAnomalies(ctx, agent.ID, 10)
	if err != nil {
		t.Fatalf("ListCostAnomalies() error = %v", err)
	}
	if len(anomalies) != 1 {
		t.Fatalf("ListCostAnomalies() count = %d, want 1", len(anomalies))
	}
	if anomalies[0].Status != "open" {
		t.Fatalf("ListCostAnomalies()[0].Status = %q, want %q", anomalies[0].Status, "open")
	}
	if anomalies[0].DailyCostMicrousd != 500000 {
		t.Fatalf("DailyCostMicrousd = %d, want 500000", anomalies[0].DailyCostMicrousd)
	}
	if anomalies[0].Multiplier != 5.0 {
		t.Fatalf("Multiplier = %f, want 5.0", anomalies[0].Multiplier)
	}

	// Get open anomaly
	open, err := q.GetOpenAnomalyForAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetOpenAnomalyForAgent() error = %v", err)
	}
	if open == nil {
		t.Fatal("GetOpenAnomalyForAgent() returned nil, expected open anomaly")
	}
	if open.ID != anomaly.ID {
		t.Fatalf("GetOpenAnomalyForAgent().ID = %q, want %q", open.ID, anomaly.ID)
	}

	// Resolve
	if err := q.UpdateCostAnomalyStatus(ctx, anomaly.ID, "resolved"); err != nil {
		t.Fatalf("UpdateCostAnomalyStatus(resolved) error = %v", err)
	}

	// Get open — should be nil now
	open, err = q.GetOpenAnomalyForAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetOpenAnomalyForAgent(after resolve) error = %v", err)
	}
	if open != nil {
		t.Fatal("GetOpenAnomalyForAgent() should return nil after resolve")
	}

	// List — still 1, but resolved
	anomalies, err = q.ListCostAnomalies(ctx, agent.ID, 10)
	if err != nil {
		t.Fatalf("ListCostAnomalies(after resolve) error = %v", err)
	}
	if len(anomalies) != 1 {
		t.Fatalf("ListCostAnomalies() count = %d, want 1", len(anomalies))
	}
	if anomalies[0].Status != "resolved" {
		t.Fatalf("Status = %q, want %q", anomalies[0].Status, "resolved")
	}
	if anomalies[0].ResolvedAt == nil {
		t.Fatal("ResolvedAt should be set after resolve")
	}
}

func TestCostAnomalyStatusValidation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	agent := mustCreateAgentForPillar1(t, ctx, q, "ca-status")

	anomaly := &domain.CostAnomaly{
		AgentID:             agent.ID,
		ProjectID:           agent.ProjectID,
		DailyCostMicrousd:   200000,
		BaselineAvgMicrousd: 50000,
		Multiplier:          4.0,
		Threshold:           3.0,
		Status:              "open",
	}
	if err := q.CreateCostAnomaly(ctx, anomaly); err != nil {
		t.Fatalf("CreateCostAnomaly() error = %v", err)
	}

	// Invalid status
	if err := q.UpdateCostAnomalyStatus(ctx, anomaly.ID, "invalid"); err == nil {
		t.Fatal("UpdateCostAnomalyStatus(invalid) should return error")
	}

	// Valid: resolved
	if err := q.UpdateCostAnomalyStatus(ctx, anomaly.ID, "resolved"); err != nil {
		t.Fatalf("UpdateCostAnomalyStatus(resolved) error = %v", err)
	}

	// Create a new anomaly for snoozed test
	anomaly2 := &domain.CostAnomaly{
		AgentID:             agent.ID,
		ProjectID:           agent.ProjectID,
		DailyCostMicrousd:   300000,
		BaselineAvgMicrousd: 50000,
		Multiplier:          6.0,
		Threshold:           3.0,
		Status:              "open",
	}
	if err := q.CreateCostAnomaly(ctx, anomaly2); err != nil {
		t.Fatalf("CreateCostAnomaly(2) error = %v", err)
	}

	// Valid: snoozed
	if err := q.UpdateCostAnomalyStatus(ctx, anomaly2.ID, "snoozed"); err != nil {
		t.Fatalf("UpdateCostAnomalyStatus(snoozed) error = %v", err)
	}

	// Verify snoozed_until is set
	anomalies, err := q.ListCostAnomalies(ctx, agent.ID, 10)
	if err != nil {
		t.Fatalf("ListCostAnomalies() error = %v", err)
	}
	var snoozed *domain.CostAnomaly
	for i := range anomalies {
		if anomalies[i].ID == anomaly2.ID {
			snoozed = &anomalies[i]
			break
		}
	}
	if snoozed == nil {
		t.Fatal("Snoozed anomaly not found in list")
	}
	if snoozed.Status != "snoozed" {
		t.Fatalf("Status = %q, want %q", snoozed.Status, "snoozed")
	}
	if snoozed.SnoozedUntil == nil {
		t.Fatal("SnoozedUntil should be set after snooze")
	}
}

// ---------- Autopilot Actions ----------

func TestAutopilotActionsCRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	agent := mustCreateAgentForPillar1(t, ctx, q, "ap-crud")

	action1 := &domain.AutopilotAction{
		AgentID:       agent.ID,
		Tier:          "simple",
		PreviousModel: "gpt-4o",
		NewModel:      "gpt-4o-mini",
		BudgetPct:     75.0,
		QualityScore:  88.5,
		Action:        "downgrade",
		Reason:        "Budget threshold exceeded",
	}
	if err := q.CreateAutopilotAction(ctx, action1); err != nil {
		t.Fatalf("CreateAutopilotAction() error = %v", err)
	}
	if action1.ID == "" {
		t.Fatal("CreateAutopilotAction() did not populate ID")
	}
	if action1.CreatedAt.IsZero() {
		t.Fatal("CreateAutopilotAction() did not populate CreatedAt")
	}

	// List
	actions, err := q.ListAutopilotActions(ctx, agent.ID, 10)
	if err != nil {
		t.Fatalf("ListAutopilotActions() error = %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("ListAutopilotActions() count = %d, want 1", len(actions))
	}
	if actions[0].Tier != "simple" {
		t.Fatalf("Tier = %q, want %q", actions[0].Tier, "simple")
	}
	if actions[0].Action != "downgrade" {
		t.Fatalf("Action = %q, want %q", actions[0].Action, "downgrade")
	}
	if actions[0].BudgetPct != 75.0 {
		t.Fatalf("BudgetPct = %f, want 75.0", actions[0].BudgetPct)
	}

	// Get latest
	latest, err := q.GetLatestAutopilotAction(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetLatestAutopilotAction() error = %v", err)
	}
	if latest == nil {
		t.Fatal("GetLatestAutopilotAction() returned nil")
	}
	if latest.ID != action1.ID {
		t.Fatalf("GetLatestAutopilotAction().ID = %q, want %q", latest.ID, action1.ID)
	}
	if latest.Reason != "Budget threshold exceeded" {
		t.Fatalf("Reason = %q, want %q", latest.Reason, "Budget threshold exceeded")
	}

	// Sleep briefly to ensure ordering by created_at
	time.Sleep(10 * time.Millisecond)

	// Create second action
	action2 := &domain.AutopilotAction{
		AgentID:       agent.ID,
		Tier:          "complex",
		PreviousModel: "gpt-4o-mini",
		NewModel:      "gpt-4o",
		BudgetPct:     50.0,
		QualityScore:  95.0,
		Action:        "revert",
		Reason:        "Quality degradation detected",
	}
	if err := q.CreateAutopilotAction(ctx, action2); err != nil {
		t.Fatalf("CreateAutopilotAction(2) error = %v", err)
	}

	// Get latest — should be action2
	latest, err = q.GetLatestAutopilotAction(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetLatestAutopilotAction(2) error = %v", err)
	}
	if latest.ID != action2.ID {
		t.Fatalf("GetLatestAutopilotAction().ID = %q, want %q (most recent)", latest.ID, action2.ID)
	}
	if latest.Action != "revert" {
		t.Fatalf("Action = %q, want %q", latest.Action, "revert")
	}

	// List with limit 1 — should return only 1
	actions, err = q.ListAutopilotActions(ctx, agent.ID, 1)
	if err != nil {
		t.Fatalf("ListAutopilotActions(limit=1) error = %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("ListAutopilotActions(limit=1) count = %d, want 1", len(actions))
	}
	if actions[0].ID != action2.ID {
		t.Fatalf("ListAutopilotActions(limit=1)[0].ID = %q, want %q (most recent)", actions[0].ID, action2.ID)
	}
}

// ---------- Cascade Delete ----------

func TestGoldenSetCascadeDelete(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	agent := mustCreateAgentForPillar1(t, ctx, q, "cascade")

	gs := &domain.GoldenSet{
		AgentID:   agent.ID,
		ProjectID: agent.ProjectID,
		Name:      "cascade-test",
		Cases:     json.RawMessage(`[{"id":"c1"}]`),
	}
	if err := q.CreateGoldenSet(ctx, gs); err != nil {
		t.Fatalf("CreateGoldenSet() error = %v", err)
	}

	// Verify it exists
	got, err := q.GetGoldenSet(ctx, agent.ID, "cascade-test")
	if err != nil {
		t.Fatalf("GetGoldenSet() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetGoldenSet() should exist before cascade delete")
	}

	// Delete agent — should CASCADE
	if err := q.DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatalf("DeleteAgent() error = %v", err)
	}

	// Golden set should be gone
	got, err = q.GetGoldenSet(ctx, agent.ID, "cascade-test")
	if err != nil {
		t.Fatalf("GetGoldenSet(after cascade) error = %v", err)
	}
	if got != nil {
		t.Fatal("GetGoldenSet() should return nil after agent cascade delete")
	}
}
