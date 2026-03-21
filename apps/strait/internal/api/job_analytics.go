package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func (s *Server) handleJobHistory(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.JobHistory")
	defer span.End()

	projectID := projectIDFromContext(ctx)
	jobID := chi.URLParam(r, "jobID")

	from, to, ok := parseCostTimeRange(w, r)
	if !ok {
		return
	}

	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		bucket = "day"
	}
	if bucket != "hour" && bucket != "day" {
		respondError(w, r, http.StatusBadRequest, "bucket must be 'hour' or 'day'")
		return
	}

	span.SetAttributes(
		attribute.String("job_id", jobID),
		attribute.String("from", from.Format(time.RFC3339)),
		attribute.String("to", to.Format(time.RFC3339)),
		attribute.String("bucket", bucket),
	)

	result, err := s.analytics().GetJobHistory(ctx, projectID, jobID, from, to, bucket)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get job history")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleJobComparison(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.JobComparison")
	defer span.End()

	projectID := projectIDFromContext(ctx)

	from, to, ok := parseCostTimeRange(w, r)
	if !ok {
		return
	}

	jobIDsStr := r.URL.Query().Get("job_ids")
	if jobIDsStr == "" {
		respondError(w, r, http.StatusBadRequest, "job_ids query parameter is required (comma-separated)")
		return
	}
	jobIDs := strings.Split(jobIDsStr, ",")
	if len(jobIDs) > 50 {
		respondError(w, r, http.StatusBadRequest, "job_ids must not exceed 50 entries")
		return
	}

	span.SetAttributes(
		attribute.String("from", from.Format(time.RFC3339)),
		attribute.String("to", to.Format(time.RFC3339)),
		attribute.Int("job_count", len(jobIDs)),
	)

	result, err := s.analytics().GetJobComparison(ctx, projectID, jobIDs, from, to)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get job comparison")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleJobReliability(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.JobReliability")
	defer span.End()

	projectID := projectIDFromContext(ctx)

	from, to, ok := parseCostTimeRange(w, r)
	if !ok {
		return
	}

	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 || parsed > 100 {
			respondError(w, r, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}

	span.SetAttributes(
		attribute.String("from", from.Format(time.RFC3339)),
		attribute.String("to", to.Format(time.RFC3339)),
		attribute.Int("limit", limit),
	)

	result, err := s.analytics().GetJobReliability(ctx, projectID, from, to, limit)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get job reliability")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleRunsByVersion(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.RunsByVersion")
	defer span.End()

	projectID := projectIDFromContext(ctx)

	from, to, ok := parseCostTimeRange(w, r)
	if !ok {
		return
	}

	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		respondError(w, r, http.StatusBadRequest, "job_id query parameter is required")
		return
	}

	span.SetAttributes(
		attribute.String("job_id", jobID),
		attribute.String("from", from.Format(time.RFC3339)),
		attribute.String("to", to.Format(time.RFC3339)),
	)

	result, err := s.analytics().GetRunsByVersion(ctx, projectID, jobID, from, to)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get runs by version")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleJobCostRanking(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.JobCostRanking")
	defer span.End()

	projectID := projectIDFromContext(ctx)

	from, to, ok := parseCostTimeRange(w, r)
	if !ok {
		return
	}

	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 || parsed > 100 {
			respondError(w, r, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}

	span.SetAttributes(
		attribute.String("from", from.Format(time.RFC3339)),
		attribute.String("to", to.Format(time.RFC3339)),
		attribute.Int("limit", limit),
	)

	result, err := s.analytics().GetJobCostRanking(ctx, projectID, from, to, limit)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get job cost ranking")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleTopFailingJobs(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("strait").Start(r.Context(), "api.TopFailingJobs")
	defer span.End()

	projectID := projectIDFromContext(ctx)

	from, to, ok := parseCostTimeRange(w, r)
	if !ok {
		return
	}

	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 || parsed > 100 {
			respondError(w, r, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}

	span.SetAttributes(
		attribute.String("from", from.Format(time.RFC3339)),
		attribute.String("to", to.Format(time.RFC3339)),
		attribute.Int("limit", limit),
	)

	result, err := s.analytics().GetTopFailingJobs(ctx, projectID, from, to, limit)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get top failing jobs")
		return
	}

	respondJSON(w, http.StatusOK, result)
}
