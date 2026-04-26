package agents

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/domain"
)

// AnomalyStore defines store methods for cost anomaly detection.
type AnomalyStore interface {
	CreateCostAnomaly(ctx context.Context, anomaly *domain.CostAnomaly) error
	GetOpenAnomalyForAgent(ctx context.Context, agentID string) (*domain.CostAnomaly, error)
}

// AnomalyAnalytics defines ClickHouse query methods for anomaly detection.
type AnomalyAnalytics interface {
	// QueryAgentDailyCosts returns daily cost totals per agent for the given window.
	QueryAgentDailyCosts(ctx context.Context, projectID string, from, to time.Time) ([]AgentDailyCost, error)
}

// AgentDailyCost represents a single day's cost for an agent.
type AgentDailyCost struct {
	AgentID      string
	AgentSlug    string
	Date         time.Time
	CostMicrousd int64
}

// AnomalyDetector checks for cost anomalies across agents.
type AnomalyDetector struct {
	store     AnomalyStore
	threshold float64 // multiplier, default 2.0
}

// NewAnomalyDetector creates a new anomaly detector.
func NewAnomalyDetector(store AnomalyStore, threshold float64) *AnomalyDetector {
	if threshold <= 0 {
		threshold = 2.0
	}
	return &AnomalyDetector{store: store, threshold: threshold}
}

// DetectAnomalies checks daily costs against rolling 7-day baseline.
// Returns detected anomalies.
func (d *AnomalyDetector) DetectAnomalies(ctx context.Context, dailyCosts []AgentDailyCost) ([]domain.CostAnomaly, error) {
	// Group by agent.
	byAgent := make(map[string][]AgentDailyCost)
	for _, dc := range dailyCosts {
		byAgent[dc.AgentID] = append(byAgent[dc.AgentID], dc)
	}

	var anomalies []domain.CostAnomaly
	today := time.Now().UTC().Truncate(24 * time.Hour)

	for agentID, costs := range byAgent {
		// Need at least 3 days of data for a meaningful baseline.
		if len(costs) < 3 {
			continue
		}

		// Separate today's cost from baseline.
		var todayCost int64
		var baselineDays []int64
		for _, c := range costs {
			day := c.Date.Truncate(24 * time.Hour)
			if day.Equal(today) {
				todayCost += c.CostMicrousd
			} else {
				baselineDays = append(baselineDays, c.CostMicrousd)
			}
		}

		if len(baselineDays) < 3 {
			continue
		}

		if len(baselineDays) == 0 || todayCost == 0 {
			continue
		}

		// Compute baseline average.
		var sum int64
		for _, v := range baselineDays {
			sum += v
		}
		baselineAvg := sum / int64(len(baselineDays))
		if baselineAvg == 0 {
			continue
		}

		multiplier := float64(todayCost) / float64(baselineAvg)
		if multiplier > d.threshold {
			// Check if there's already an open anomaly.
			existing, _ := d.store.GetOpenAnomalyForAgent(ctx, agentID)
			if existing != nil {
				continue // already flagged
			}

			anomaly := domain.CostAnomaly{
				AgentID:             agentID,
				DailyCostMicrousd:   todayCost,
				BaselineAvgMicrousd: baselineAvg,
				Multiplier:          multiplier,
				Threshold:           d.threshold,
				Status:              "open",
			}
			if err := d.store.CreateCostAnomaly(ctx, &anomaly); err != nil {
				slog.Error("create cost anomaly", "agent_id", agentID, "error", err)
				continue
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies, nil
}
