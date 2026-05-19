package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
)

// SLOStore is the minimal store interface for CDC-driven SLO evaluation.
type SLOStore interface {
	ListJobSLOs(ctx context.Context, jobID string) ([]domain.JobSLOStatus, error)
	InsertSLOEvaluation(ctx context.Context, eval *domain.JobSLOEvaluation) error
}

// SLOHandler inserts SLO evaluation data points from CDC events on job_runs.
// When a run reaches a terminal status, it lists the job's SLOs and creates
// a lightweight evaluation record for each one.
type SLOHandler struct {
	store  SLOStore
	logger *slog.Logger
	dedupe *recentDedupe
}

// NewSLOHandler creates a CDC handler that inserts SLO evaluation data points.
func NewSLOHandler(store SLOStore, logger *slog.Logger) *SLOHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SLOHandler{store: store, logger: logger, dedupe: newRecentDedupe(16_384)}
}

// Table returns the table this handler watches.
func (h *SLOHandler) Table() string { return "job_runs" }

// Handle processes a CDC event for a job run status change.
func (h *SLOHandler) Handle(ctx context.Context, msg Message) error {
	if msg.Action != ActionUpdate {
		return nil
	}

	var record struct {
		ID        string `json:"id"`
		JobID     string `json:"job_id"`
		ProjectID string `json:"project_id"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(msg.Record, &record); err != nil {
		return fmt.Errorf("slo handler: unmarshal record: %w", err)
	}

	status := domain.RunStatus(record.Status)
	if !status.IsTerminal() {
		return nil
	}

	slos, err := h.store.ListJobSLOs(ctx, record.JobID)
	if err != nil {
		h.logger.Warn("cdc slo handler: failed to list SLOs",
			"job_id", record.JobID, "error", err)
		return fmt.Errorf("slo handler: list job slos: %w", err)
	}

	if len(slos) == 0 {
		return nil
	}

	value := sloCurrentValue(status)
	now := time.Now().UTC()

	var insertErrs []error
	for _, slo := range slos {
		if slo.EvaluatedAt != nil {
			continue
		}
		dedupeKey := sloEvaluationDedupeKey(record.ID, slo.ID)
		if !h.dedupe.Remember(dedupeKey) {
			continue
		}
		eval := &domain.JobSLOEvaluation{
			ID:              uuid.Must(uuid.NewV7()).String(),
			SLOID:           slo.ID,
			CurrentValue:    value,
			BudgetRemaining: 0,
			EvaluatedAt:     now,
		}
		if insertErr := h.store.InsertSLOEvaluation(ctx, eval); insertErr != nil {
			h.dedupe.Forget(dedupeKey)
			h.logger.Warn("cdc slo handler: failed to insert evaluation",
				"slo_id", slo.ID, "run_id", record.ID, "error", insertErr)
			insertErrs = append(insertErrs, insertErr)
		}
	}

	if err := errors.Join(insertErrs...); err != nil {
		return fmt.Errorf("slo handler: insert evaluation: %w", err)
	}
	return nil
}

func sloEvaluationDedupeKey(runID, sloID string) string {
	return "slo:run:" + runID + ":" + sloID
}

func sloCurrentValue(status domain.RunStatus) float64 {
	switch status {
	case domain.StatusCompleted:
		return 1.0
	case domain.StatusFailed,
		domain.StatusTimedOut,
		domain.StatusCrashed,
		domain.StatusSystemFailed,
		domain.StatusCanceled,
		domain.StatusExpired,
		domain.StatusDeadLetter:
		return 0.0
	default:
		return 0.0
	}
}
