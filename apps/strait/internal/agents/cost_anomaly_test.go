package agents

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

type mockAnomalyStore struct {
	anomalies   []domain.CostAnomaly
	openAnomaly *domain.CostAnomaly
	createErr   error
}

func (m *mockAnomalyStore) CreateCostAnomaly(_ context.Context, anomaly *domain.CostAnomaly) error {
	if m.createErr != nil {
		return m.createErr
	}
	anomaly.ID = "anomaly-" + anomaly.AgentID
	anomaly.DetectedAt = time.Now().UTC()
	m.anomalies = append(m.anomalies, *anomaly)
	return nil
}

func (m *mockAnomalyStore) GetOpenAnomalyForAgent(_ context.Context, _ string) (*domain.CostAnomaly, error) {
	return m.openAnomaly, nil
}

func makeDailyCosts(agentID string, baselineDays int, baselineCost, todayCost int64) []AgentDailyCost {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	var costs []AgentDailyCost
	for i := 1; i <= baselineDays; i++ {
		costs = append(costs, AgentDailyCost{
			AgentID:      agentID,
			Date:         today.AddDate(0, 0, -i),
			CostMicrousd: baselineCost,
		})
	}
	if todayCost > 0 {
		costs = append(costs, AgentDailyCost{
			AgentID:      agentID,
			Date:         today,
			CostMicrousd: todayCost,
		})
	}
	return costs
}

func TestDetectAnomalies_NormalSpending(t *testing.T) {
	store := &mockAnomalyStore{}
	d := NewAnomalyDetector(store, 2.0)

	// Baseline 100, today 150 => 1.5x, below 2.0 threshold.
	costs := makeDailyCosts("agent-1", 5, 100_000, 150_000)
	anomalies, err := d.DetectAnomalies(context.Background(), costs)
	if err != nil {
		t.Fatal(err)
	}
	if len(anomalies) != 0 {
		t.Errorf("expected 0 anomalies, got %d", len(anomalies))
	}
}

func TestDetectAnomalies_SpikeDetected(t *testing.T) {
	store := &mockAnomalyStore{}
	d := NewAnomalyDetector(store, 2.0)

	// Baseline 100, today 300 => 3x, above 2.0 threshold.
	costs := makeDailyCosts("agent-1", 5, 100_000, 300_000)
	anomalies, err := d.DetectAnomalies(context.Background(), costs)
	if err != nil {
		t.Fatal(err)
	}
	if len(anomalies) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(anomalies))
	}
	a := anomalies[0]
	if a.AgentID != "agent-1" {
		t.Errorf("expected agent-1, got %s", a.AgentID)
	}
	if a.Multiplier < 2.9 || a.Multiplier > 3.1 {
		t.Errorf("expected multiplier ~3.0, got %f", a.Multiplier)
	}
	if a.Status != "open" {
		t.Errorf("expected status open, got %s", a.Status)
	}
}

func TestDetectAnomalies_InsufficientBaseline(t *testing.T) {
	store := &mockAnomalyStore{}
	d := NewAnomalyDetector(store, 2.0)

	// Only 2 days of data (including today) — below 3-entry minimum.
	costs := makeDailyCosts("agent-1", 1, 100_000, 500_000)
	anomalies, err := d.DetectAnomalies(context.Background(), costs)
	if err != nil {
		t.Fatal(err)
	}
	if len(anomalies) != 0 {
		t.Errorf("expected 0 anomalies with insufficient data, got %d", len(anomalies))
	}
}

func TestDetectAnomalies_CustomThreshold(t *testing.T) {
	store := &mockAnomalyStore{}
	d := NewAnomalyDetector(store, 3.0) // Higher threshold.

	// Baseline 100, today 250 => 2.5x — below 3.0 threshold.
	costs := makeDailyCosts("agent-1", 5, 100_000, 250_000)
	anomalies, err := d.DetectAnomalies(context.Background(), costs)
	if err != nil {
		t.Fatal(err)
	}
	if len(anomalies) != 0 {
		t.Errorf("expected 0 anomalies at 3x threshold, got %d", len(anomalies))
	}
}

func TestDetectAnomalies_ZeroBaseline(t *testing.T) {
	store := &mockAnomalyStore{}
	d := NewAnomalyDetector(store, 2.0)

	// Baseline is all zeros.
	costs := makeDailyCosts("agent-1", 5, 0, 100_000)
	anomalies, err := d.DetectAnomalies(context.Background(), costs)
	if err != nil {
		t.Fatal(err)
	}
	if len(anomalies) != 0 {
		t.Errorf("expected 0 anomalies with zero baseline, got %d", len(anomalies))
	}
}

func TestDetectAnomalies_ExistingOpenAnomaly(t *testing.T) {
	store := &mockAnomalyStore{
		openAnomaly: &domain.CostAnomaly{
			ID:      "existing-1",
			AgentID: "agent-1",
			Status:  "open",
		},
	}
	d := NewAnomalyDetector(store, 2.0)

	// 3x spike but already flagged.
	costs := makeDailyCosts("agent-1", 5, 100_000, 300_000)
	anomalies, err := d.DetectAnomalies(context.Background(), costs)
	if err != nil {
		t.Fatal(err)
	}
	if len(anomalies) != 0 {
		t.Errorf("expected 0 anomalies when already flagged, got %d", len(anomalies))
	}
}

func TestDetectAnomalies_MultipleAgents(t *testing.T) {
	store := &mockAnomalyStore{}
	d := NewAnomalyDetector(store, 2.0)

	// Agent-1: normal (1.5x). Agent-2: spike (3x).
	costs := makeDailyCosts("agent-1", 5, 100_000, 150_000)
	costs = append(costs, makeDailyCosts("agent-2", 5, 100_000, 300_000)...)

	anomalies, err := d.DetectAnomalies(context.Background(), costs)
	if err != nil {
		t.Fatal(err)
	}
	if len(anomalies) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(anomalies))
	}
	if anomalies[0].AgentID != "agent-2" {
		t.Errorf("expected anomaly for agent-2, got %s", anomalies[0].AgentID)
	}
}

func TestDetectAnomalies_TodayOnlyNoBaseline(t *testing.T) {
	store := &mockAnomalyStore{}
	d := NewAnomalyDetector(store, 2.0)

	// Only today's data, no baseline days.
	today := time.Now().UTC().Truncate(24 * time.Hour)
	costs := []AgentDailyCost{
		{AgentID: "agent-1", Date: today, CostMicrousd: 500_000},
		{AgentID: "agent-1", Date: today, CostMicrousd: 500_000},
		{AgentID: "agent-1", Date: today, CostMicrousd: 500_000},
	}
	anomalies, err := d.DetectAnomalies(context.Background(), costs)
	if err != nil {
		t.Fatal(err)
	}
	if len(anomalies) != 0 {
		t.Errorf("expected 0 anomalies with no baseline, got %d", len(anomalies))
	}
}


func TestDetectAnomalies_BaselineTooShortAfterSplit(t *testing.T) {
	store := &mockAnomalyStore{}
	d := NewAnomalyDetector(store, 2.0)

	// 5 total entries but 4 are today, only 1 baseline day.
	// After split: baselineDays has 1 entry (< 3 minimum), so should skip.
	today := time.Now().UTC().Truncate(24 * time.Hour)
	costs := []AgentDailyCost{
		{AgentID: "agent-1", Date: today, CostMicrousd: 100_000},
		{AgentID: "agent-1", Date: today, CostMicrousd: 100_000},
		{AgentID: "agent-1", Date: today, CostMicrousd: 100_000},
		{AgentID: "agent-1", Date: today, CostMicrousd: 100_000},
		{AgentID: "agent-1", Date: today.AddDate(0, 0, -1), CostMicrousd: 10_000},
	}
	anomalies, err := d.DetectAnomalies(context.Background(), costs)
	if err != nil {
		t.Fatal(err)
	}
	if len(anomalies) != 0 {
		t.Errorf("expected 0 anomalies when baseline too short after split, got %d", len(anomalies))
	}
}