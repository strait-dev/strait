package scheduler

import (
	"context"
	"encoding/json"
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
	store          sloEvaluationStore
	logger         *slog.Logger
	notifier       SLOWebhookNotifier
	advisoryLocker AdvisoryLocker
}

type sloEvaluationStore interface {
	ListAllJobSLOs(ctx context.Context) ([]domain.JobSLO, error)
	GetJobHealthCounts(ctx context.Context, jobID string, since time.Time) (*store.JobHealthStats, error)
	GetJobHealthStats(ctx context.Context, jobID string, since time.Time) (*store.JobHealthStats, error)
	InsertSLOEvaluation(ctx context.Context, eval *domain.JobSLOEvaluation) error
	PruneSLOEvaluations(ctx context.Context, keepPerSLO int) (int64, error)
}

// sloEvaluatorLockID is the advisory lock key for single-leader SLO evaluation.
const sloEvaluatorLockID int64 = 900_100_022

// NewSLOEvaluator creates a new SLO evaluator.
func NewSLOEvaluator(store sloEvaluationStore, logger *slog.Logger, opts ...SLOEvaluatorOption) *SLOEvaluator {
	if logger == nil {
		logger = slog.Default()
	}
	e := &SLOEvaluator{store: store, logger: logger}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// SLOEvaluatorOption configures the SLO evaluator.
type SLOEvaluatorOption func(*SLOEvaluator)

// WithSLOWebhookNotifier sets the webhook notifier for SLO budget alerts.
func WithSLOWebhookNotifier(n SLOWebhookNotifier) SLOEvaluatorOption {
	return func(e *SLOEvaluator) {
		e.notifier = n
	}
}

// WithSLOEvaluatorAdvisoryLocker enables single-leader SLO evaluation across replicas.
func WithSLOEvaluatorAdvisoryLocker(locker AdvisoryLocker) SLOEvaluatorOption {
	return func(e *SLOEvaluator) {
		e.advisoryLocker = locker
	}
}

// WithAdvisoryLocker enables single-leader SLO evaluation across replicas.
func (e *SLOEvaluator) WithAdvisoryLocker(locker AdvisoryLocker) *SLOEvaluator {
	e.advisoryLocker = locker
	return e
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

// Run starts periodic SLO evaluation until ctx is canceled.
func (e *SLOEvaluator) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, interval, func() {
				if _, err := e.evaluateWithOptionalLeader(ctx); err != nil {
					e.logger.Warn("slo evaluation cycle failed", "error", err)
				}
			})
		}
	}
}

func (e *SLOEvaluator) evaluateWithOptionalLeader(ctx context.Context) (bool, error) {
	return runWithOptionalAdvisoryLock(ctx, e.advisoryLocker, sloEvaluatorLockID, e.Evaluate)
}

// evaluateSLO queries the hot job_runs table only. WindowHours exceeding
// hot retention is rejected at the store write boundary (CreateJobSLO).
func (e *SLOEvaluator) evaluateSLO(ctx context.Context, slo domain.JobSLO, now time.Time) error {
	since := now.Add(-time.Duration(slo.WindowHours) * time.Hour)

	stats, err := e.getStatsForSLOMetric(ctx, slo.Metric, slo.JobID, since)
	if err != nil {
		return fmt.Errorf("get health stats: %w", err)
	}
	if stats == nil {
		return nil
	}
	if !hasSLOData(slo.Metric, stats) {
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

		if e.notifier != nil {
			alertPayload, marshalErr := json.Marshal(map[string]any{
				"event":            domain.WebhookEventSLOBudgetWarning,
				"slo_id":           slo.ID,
				"job_id":           slo.JobID,
				"metric":           slo.Metric,
				"target":           slo.Target,
				"current_value":    currentValue,
				"budget_remaining": budget,
			})
			if marshalErr != nil {
				e.logger.Warn("failed to marshal slo alert payload", "error", marshalErr)
			} else if notifyErr := e.notifier.NotifySLOBudgetWarning(ctx, slo.ProjectID, alertPayload); notifyErr != nil {
				e.logger.Warn("failed to send slo budget webhook",
					"slo_id", slo.ID,
					"error", notifyErr,
				)
			}
		}
	}

	return nil
}

func (e *SLOEvaluator) getStatsForSLOMetric(ctx context.Context, metric, jobID string, since time.Time) (*store.JobHealthStats, error) {
	if metric == domain.SLOMetricSuccessRate {
		return e.store.GetJobHealthCounts(ctx, jobID, since)
	}
	return e.store.GetJobHealthStats(ctx, jobID, since)
}

func hasSLOData(metric string, stats *store.JobHealthStats) bool {
	if stats == nil || stats.TotalRuns == 0 {
		return false
	}
	switch metric {
	case domain.SLOMetricSuccessRate, domain.SLOMetricP95LatencySecs, domain.SLOMetricP99LatencySecs:
		return true
	default:
		return false
	}
}

func metricValue(metric string, stats *store.JobHealthStats) float64 {
	switch metric {
	case domain.SLOMetricSuccessRate:
		// SuccessRate from GetJobHealthStats is a percentage (0-100);
		// CalculateErrorBudget expects a fraction (0-1).
		return stats.SuccessRate / 100.0
	case domain.SLOMetricP95LatencySecs:
		return stats.P95DurationSecs
	case domain.SLOMetricP99LatencySecs:
		return stats.P99DurationSecs
	default:
		return 0
	}
}

// CalculateErrorBudget computes the remaining error budget as a fraction [0, 1].
// For success_rate: budget = 1 - ((1 - current) / (1 - target))
// For latency: budget = 1 - (current / target), where lower is better.
func CalculateErrorBudget(current, target float64, metric string) float64 {
	if math.IsNaN(current) || math.IsNaN(target) || math.IsInf(current, 0) || math.IsInf(target, 0) {
		return 0.0
	}
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
