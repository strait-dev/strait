package api

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
)

func (s *Server) runMatchesEnvironment(ctx context.Context, run domain.JobRun, jobEnvCache map[string]bool) (bool, error) {
	if environmentIDFromContext(ctx) == "" {
		return true, nil
	}
	if allowed, ok := jobEnvCache[run.JobID]; ok {
		return allowed, nil
	}

	job, err := s.store.GetJob(ctx, run.JobID)
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, huma.Error404NotFound("run not found")
	}

	allowed := requireEnvironmentMatch(ctx, job.EnvironmentID) == nil
	jobEnvCache[run.JobID] = allowed
	return allowed, nil
}

func (s *Server) listDeadLetterRunsForEnvironment(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	jobEnvCache := make(map[string]bool)
	filtered := make([]domain.JobRun, 0, limit)
	pageCursor := cursor
	fetchLimit := max(limit, 25)

	for {
		page, err := s.store.ListDeadLetterRuns(ctx, projectID, fetchLimit, pageCursor)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			return filtered, nil
		}

		for _, run := range page {
			allowed, err := s.runMatchesEnvironment(ctx, run, jobEnvCache)
			if err != nil {
				return nil, err
			}
			if !allowed {
				continue
			}
			filtered = append(filtered, run)
			if len(filtered) >= limit {
				return filtered, nil
			}
		}

		if len(page) < fetchLimit {
			return filtered, nil
		}
		lastCreatedAt := page[len(page)-1].CreatedAt
		pageCursor = &lastCreatedAt
	}
}

func (s *Server) bulkReplayDeadLetterRunsForEnvironment(ctx context.Context, projectID string, limit int) ([]domain.JobRun, error) {
	cursor := (*time.Time)(nil)
	jobEnvCache := make(map[string]bool)
	runIDs := make([]string, 0, limit)

	for len(runIDs) < limit {
		page, err := s.store.ListDeadLetterRuns(ctx, projectID, limit+1, cursor)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}

		for _, run := range page {
			allowed, err := s.runMatchesEnvironment(ctx, run, jobEnvCache)
			if err != nil {
				return nil, err
			}
			if allowed {
				runIDs = append(runIDs, run.ID)
				if len(runIDs) >= limit {
					break
				}
			}
		}

		if len(page) < limit+1 {
			break
		}
		lastCreatedAt := page[len(page)-1].CreatedAt
		cursor = &lastCreatedAt
	}

	if len(runIDs) == 0 {
		return []domain.JobRun{}, nil
	}
	return s.store.BulkReplayDeadLetterRuns(ctx, runIDs, "", 0)
}
