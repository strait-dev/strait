package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/google/uuid"
)

// SLOEvaluator periodically evaluates all job SLOs and records results.
type SLOEvaluator struct {
	store  *store.Queries
	logger *slog.Logger
}

// NewSLOEvaluator creates a new SLO evaluator.
func NewSLOEvaluator(store *store.Queries, logger *slog.Logger) *SLOEvaluator {
	if logger == nil {
		logger = slog.Default()
	}
	return &SLOEvaluator{store: store, logger: logger}
}

// Evaluate runs a single evaluation cycle for all SLOs.
func (e *SLOEvaluator) Evaluate(ctx context.Context) error {
	slos, err := e.store.ListAllJobSLOs(ctx)
	if err != nil {
		return fmt.Errorf("list slos: %w", err)
	}

	if len(slos) == 0 {
		return nil
	}

	now := time.Now()
	for _, slo := range slos {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := e.evaluateSLO(ctx, slo, now); err != nil {
			e.logger.Warn("slo evaluation failed",
				"slo_id", slo.ID,
				"job_id", slo.JobID,
				"metric", slo.Metric,
				"error", err,
			)
		}
	}

	// Prune old evaluations (keep 288 per SLO = 24h at 5m intervals).
	pruned, err := e.store.PruneSLOEvaluations(ctx, 288)
	if err != nil {
		e.logger.Warn("failed to prune slo evaluations", "error", err)
	} else if pruned > 0 {
		e.logger.Debug("pruned slo evaluations", "count", pruned)
	}

	return nil
}

func (e *SLOEvaluator) evaluateSLO(ctx context.Context, slo domain.JobSLO, now time.Time) error {
	since := now.Add(-time.Duration(slo.WindowHours) * time.Hour)

	stats, err := e.store.GetJobHealthStats(ctx, slo.JobID, since)
	if err != nil {
		return fmt.Errorf("get health stats: %w", err)
	}
	if stats == nil {
		return nil
	}

	currentValue := metricValue(slo.Metric, stats)
	budget := CalculateErrorBudget(currentValue, slo.Target, slo.Metric)

	eval := &domain.JobSLOEvaluation{
		ID:              uuid.Must(uuid.NewV7()).String(),
		SLOID:           slo.ID,
		CurrentValue:    currentValue,
		BudgetRemaining: budget,
		EvaluatedAt:     now,
	}

	if err := e.store.InsertSLOEvaluation(ctx, eval); err != nil {
		return fmt.Errorf("insert evaluation: %w", err)
	}

	if budget < 0.2 {
		e.logger.Warn("slo budget depleting",
			"slo_id", slo.ID,
			"job_id", slo.JobID,
			"metric", slo.Metric,
			"target", slo.Target,
			"current", currentValue,
			"budget_remaining", budget,
		)
	}

	return nil
}

func metricValue(metric string, stats *store.JobHealthStats) float64 {
	switch metric {
	case domain.SLOMetricSuccessRate:
		return stats.SuccessRate
	case domain.SLOMetricP95LatencySecs:
		return stats.P95DurationSecs
	case domain.SLOMetricP99LatencySecs:
		// P99 not yet available in health stats; P95 is used as a lower-bound
		// approximation. This means P99 SLO targets should be set accordingly.
		return stats.P95DurationSecs
	default:
		return 0
	}
}

// CalculateErrorBudget computes the remaining error budget as a fraction [0, 1].
// For success_rate: budget = 1 - ((1 - current) / (1 - target))
// For latency: budget = 1 - (current / target), where lower is better.
func CalculateErrorBudget(current, target float64, metric string) float64 {
	switch metric {
	case domain.SLOMetricSuccessRate:
		if target >= 1.0 {
			if current >= 1.0 {
				return 1.0
			}
			return 0.0
		}
		budget := 1.0 - ((1.0 - current) / (1.0 - target))
		return math.Max(0, math.Min(1, budget))

	case domain.SLOMetricP95LatencySecs, domain.SLOMetricP99LatencySecs:
		if target <= 0 {
			return 1.0
		}
		budget := 1.0 - (current / target)
		return math.Max(0, math.Min(1, budget))

	default:
		return 1.0
	}
}
