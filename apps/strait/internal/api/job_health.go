package api

import (
	"context"
	"errors"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/store"
)

// JobHealthResponse wraps health stats with the time window.
type JobHealthResponse struct {
	JobID  string    `json:"job_id"`
	Window string    `json:"window"`
	Since  time.Time `json:"since"`
	*store.JobHealthStats
}

// GetJobHealthInput is the typed input for getting job health stats.
type GetJobHealthInput struct {
	JobID  string `path:"jobID"`
	Window string `query:"window"`
}

// GetJobHealthOutput is the typed output for getting job health stats.
type GetJobHealthOutput struct {
	Body JobHealthResponse
}

func (s *Server) handleGetJobHealth(ctx context.Context, input *GetJobHealthInput) (*GetJobHealthOutput, error) {
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}

	window := input.Window
	var since time.Time
	switch window {
	case "1h":
		since = time.Now().Add(-time.Hour)
	case "1d":
		since = time.Now().Add(-24 * time.Hour)
	case "30d":
		since = time.Now().Add(-30 * 24 * time.Hour)
	case "7d", "":
		window = "7d"
		since = time.Now().Add(-7 * 24 * time.Hour)
	default:
		return nil, huma.Error400BadRequest("invalid window: must be 1h, 1d, 7d, or 30d")
	}

	stats, err := s.store.GetJobHealthStats(ctx, input.JobID, since)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to compute health stats")
	}

	return &GetJobHealthOutput{Body: JobHealthResponse{
		JobID:          input.JobID,
		Window:         window,
		Since:          since,
		JobHealthStats: stats,
	}}, nil
}
