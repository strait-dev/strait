package api

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

type BulkCancelRequest struct {
	RunIDs []string `json:"run_ids" validate:"required,min=1"`
}

type BulkCancelResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type BulkCancelResponse struct {
	Results  []BulkCancelResult `json:"results"`
	Total    int                `json:"total"`
	Canceled int                `json:"canceled"`
	Failed   int                `json:"failed"`
}

type BulkCancelRunsInput struct {
	Body BulkCancelRequest
}

type BulkCancelRunsOutput struct {
	Body BulkCancelResponse
}

func (s *Server) handleBulkCancelRuns(ctx context.Context, input *BulkCancelRunsInput) (*BulkCancelRunsOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	if len(req.RunIDs) > 100 {
		return nil, huma.Error400BadRequest("maximum 100 run IDs per bulk cancel request")
	}

	// Fetch once, then partition locally so the response preserves the
	// caller's requested run IDs and reports per-run failures.
	runsMap, err := s.store.GetRunsByIDs(ctx, req.RunIDs)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to fetch runs")
	}

	canceled := 0
	selection := s.selectBulkCancelableRuns(ctx, req.RunIDs, runsMap)

	if len(selection.cancelableIDs) > 0 {
		now := time.Now()
		cancelResults, cancelErr := s.store.BulkCancelRuns(ctx, selection.cancelableIDs, now, "canceled by user (bulk)")
		if cancelErr != nil {
			return nil, huma.Error500InternalServerError("failed to cancel runs")
		}

		canceled = selection.appendStoreResults(runsMap, cancelResults)

		// Child cancellation is best-effort: parent cancellation has already
		// succeeded, and retrying the whole request would duplicate results.
		if _, err := s.store.CancelChildRunsByParentIDs(ctx, selection.cancelableIDs, now, "parent run canceled (bulk)"); err != nil {
			slog.Error("failed to cancel child runs in bulk", "error", err)
		}
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunBulkCancelled, "run", "", map[string]any{
		"total":    len(req.RunIDs),
		"canceled": canceled,
		"failed":   selection.failed,
	})

	return &BulkCancelRunsOutput{
		Body: BulkCancelResponse{
			Results:  selection.results,
			Total:    len(req.RunIDs),
			Canceled: canceled,
			Failed:   selection.failed,
		},
	}, nil
}

type bulkCancelSelection struct {
	results       []BulkCancelResult
	cancelableIDs []string
	failed        int
}

func (s *Server) selectBulkCancelableRuns(ctx context.Context, runIDs []string, runsMap map[string]*domain.JobRun) bulkCancelSelection {
	selection := bulkCancelSelection{
		results:       make([]BulkCancelResult, 0, len(runIDs)),
		cancelableIDs: make([]string, 0, len(runIDs)),
	}
	for _, runID := range runIDs {
		run, ok := runsMap[runID]
		if !ok || run == nil {
			selection.appendFailure(runID, "failed", "run not found")
			continue
		}
		if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
			selection.appendFailure(runID, "failed", "run not found")
			continue
		}
		if err := s.requireRunEnvironmentMatch(ctx, run); err != nil {
			selection.appendFailure(runID, "failed", "run not found")
			continue
		}
		if run.Status.IsTerminal() {
			selection.appendFailure(runID, string(run.Status), "run already in terminal state")
			continue
		}
		selection.cancelableIDs = append(selection.cancelableIDs, runID)
	}
	return selection
}

func (s *Server) requireRunEnvironmentMatch(ctx context.Context, run *domain.JobRun) error {
	if environmentIDFromContext(ctx) == "" || run == nil {
		return nil
	}
	job, err := s.store.GetJob(ctx, run.JobID)
	if err != nil {
		return err
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return err
	}
	return requireEnvironmentMatch(ctx, job.EnvironmentID)
}

func (s *bulkCancelSelection) appendFailure(id, status, errMsg string) {
	s.results = append(s.results, BulkCancelResult{ID: id, Status: status, Error: errMsg})
	s.failed++
}

func (s *bulkCancelSelection) appendStoreResults(runsMap map[string]*domain.JobRun, cancelResults []store.BulkCancelResult) int {
	canceledSet := make(map[string]struct{}, len(cancelResults))
	canceled := 0
	for _, result := range cancelResults {
		canceledSet[result.ID] = struct{}{}
		s.results = append(s.results, BulkCancelResult{ID: result.ID, Status: string(domain.StatusCanceled)})
		canceled++
	}

	// A run can leave the cancelable set between the initial fetch and the
	// update. Keep that race visible in the per-run response.
	for _, id := range s.cancelableIDs {
		if _, ok := canceledSet[id]; ok {
			continue
		}
		status := "failed"
		if run := runsMap[id]; run != nil {
			status = string(run.Status)
		}
		s.appendFailure(id, status, "failed to cancel (status may have changed)")
	}
	return canceled
}
