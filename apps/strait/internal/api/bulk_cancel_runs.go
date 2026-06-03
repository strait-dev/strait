package api

import (
	"context"

	"strait/internal/domain"
	"strait/internal/store"
)

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
