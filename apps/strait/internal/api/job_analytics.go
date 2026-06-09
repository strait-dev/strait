package api

import (
	"context"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type JobHistoryInput struct {
	JobID  string `path:"jobID"`
	From   string `query:"from"`
	To     string `query:"to"`
	Bucket string `query:"bucket"`
}
type JobHistoryOutput struct{ Body any }

func (s *Server) handleJobHistory(ctx context.Context, input *JobHistoryInput) (*JobHistoryOutput, error) {
	if err := requireProjectWideScope(ctx, "job analytics"); err != nil {
		return nil, err
	}
	ctx, span := otel.Tracer("strait").Start(ctx, "api.JobHistory")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	bucket, err := normalizeAnalyticsBucket(input.Bucket)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("job_id", input.JobID), attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)), attribute.String("bucket", bucket))
	result, rErr := s.analytics().GetJobHistory(ctx, projectID, input.JobID, from, to, bucket)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get job history")
	}
	return &JobHistoryOutput{Body: result}, nil
}

type JobComparisonInput struct {
	From   string `query:"from"`
	To     string `query:"to"`
	JobIDs string `query:"job_ids"`
}
type JobComparisonOutput struct{ Body any }

func (s *Server) handleJobComparison(ctx context.Context, input *JobComparisonInput) (*JobComparisonOutput, error) {
	if err := requireProjectWideScope(ctx, "job analytics"); err != nil {
		return nil, err
	}
	ctx, span := otel.Tracer("strait").Start(ctx, "api.JobComparison")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	if input.JobIDs == "" {
		return nil, huma.Error400BadRequest("job_ids query parameter is required (comma-separated)")
	}
	jobIDs := strings.Split(input.JobIDs, ",")
	if len(jobIDs) > 50 {
		return nil, huma.Error400BadRequest("job_ids must not exceed 50 entries")
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)), attribute.Int("job_count", len(jobIDs)))
	result, rErr := s.analytics().GetJobComparison(ctx, projectID, jobIDs, from, to)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get job comparison")
	}
	return &JobComparisonOutput{Body: result}, nil
}

type JobReliabilityInput struct {
	From  string `query:"from"`
	To    string `query:"to"`
	Limit int    `query:"limit"`
}
type JobReliabilityOutput struct{ Body any }

func (s *Server) handleJobReliability(ctx context.Context, input *JobReliabilityInput) (*JobReliabilityOutput, error) {
	if err := requireProjectWideScope(ctx, "job analytics"); err != nil {
		return nil, err
	}
	ctx, span := otel.Tracer("strait").Start(ctx, "api.JobReliability")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	limit := input.Limit
	if limit == 0 {
		limit = 10
	}
	if limit < 1 || limit > 100 {
		return nil, huma.Error400BadRequest("limit must be between 1 and 100")
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)), attribute.Int("limit", limit))
	result, rErr := s.analytics().GetJobReliability(ctx, projectID, from, to, limit)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get job reliability")
	}
	return &JobReliabilityOutput{Body: result}, nil
}

type RunsByVersionInput struct {
	From  string `query:"from"`
	To    string `query:"to"`
	JobID string `query:"job_id"`
}
type RunsByVersionOutput struct{ Body any }

func (s *Server) handleRunsByVersion(ctx context.Context, input *RunsByVersionInput) (*RunsByVersionOutput, error) {
	if err := requireProjectWideScope(ctx, "job analytics"); err != nil {
		return nil, err
	}
	ctx, span := otel.Tracer("strait").Start(ctx, "api.RunsByVersion")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	if input.JobID == "" {
		return nil, huma.Error400BadRequest("job_id query parameter is required")
	}
	span.SetAttributes(attribute.String("job_id", input.JobID), attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)))
	result, rErr := s.analytics().GetRunsByVersion(ctx, projectID, input.JobID, from, to)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get runs by version")
	}
	return &RunsByVersionOutput{Body: result}, nil
}

type JobCostRankingInput struct {
	From  string `query:"from"`
	To    string `query:"to"`
	Limit int    `query:"limit"`
}
type JobCostRankingOutput struct{ Body any }

func (s *Server) handleJobCostRanking(ctx context.Context, input *JobCostRankingInput) (*JobCostRankingOutput, error) {
	if err := requireProjectWideScope(ctx, "job analytics"); err != nil {
		return nil, err
	}
	ctx, span := otel.Tracer("strait").Start(ctx, "api.JobCostRanking")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	limit := input.Limit
	if limit == 0 {
		limit = 10
	}
	if limit < 1 || limit > 100 {
		return nil, huma.Error400BadRequest("limit must be between 1 and 100")
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)), attribute.Int("limit", limit))
	result, rErr := s.analytics().GetJobCostRanking(ctx, projectID, from, to, limit)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get job cost ranking")
	}
	return &JobCostRankingOutput{Body: result}, nil
}

type TopFailingJobsInput struct {
	From  string `query:"from"`
	To    string `query:"to"`
	Limit int    `query:"limit"`
}
type TopFailingJobsOutput struct{ Body any }

func (s *Server) handleTopFailingJobs(ctx context.Context, input *TopFailingJobsInput) (*TopFailingJobsOutput, error) {
	if err := requireProjectWideScope(ctx, "job analytics"); err != nil {
		return nil, err
	}
	ctx, span := otel.Tracer("strait").Start(ctx, "api.TopFailingJobs")
	defer span.End()
	projectID := projectIDFromContext(ctx)
	from, to, err := parseCostTimeRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	limit := input.Limit
	if limit == 0 {
		limit = 10
	}
	if limit < 1 || limit > 100 {
		return nil, huma.Error400BadRequest("limit must be between 1 and 100")
	}
	span.SetAttributes(attribute.String("from", from.Format(time.RFC3339)), attribute.String("to", to.Format(time.RFC3339)), attribute.Int("limit", limit))
	result, rErr := s.analytics().GetTopFailingJobs(ctx, projectID, from, to, limit)
	if rErr != nil {
		return nil, huma.Error500InternalServerError("failed to get top failing jobs")
	}
	return &TopFailingJobsOutput{Body: result}, nil
}
