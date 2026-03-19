package api

import (
	"net/http"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleSDKIteration(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	var req struct {
		Iteration   int    `json:"iteration" validate:"required,min=1"`
		Description string `json:"description,omitempty"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	// Iteration count limit check.
	run, runErr := s.store.GetRun(r.Context(), runID)
	if runErr == nil && run != nil {
		job, jobErr := s.store.GetJob(r.Context(), run.JobID)
		quota, qErr := s.store.GetProjectQuota(r.Context(), run.ProjectID)
		var quotaLimit int
		if qErr == nil && quota != nil {
			quotaLimit = quota.MaxIterationsPerRun
		}
		var jobLimit int
		if jobErr == nil && job != nil {
			jobLimit = job.MaxIterationsPerRun
		}
		iterLimit := resolveGuardrailInt(quotaLimit, jobLimit)
		if iterLimit > 0 {
			count, cErr := s.store.CountRunIterations(r.Context(), runID)
			if cErr == nil && count >= iterLimit {
				respondJSON(w, http.StatusTooManyRequests, map[string]any{
					"error": "iteration_limit_exceeded", "current": count, "limit": iterLimit,
				})
				return
			}
		}
	}

	iter := &domain.RunIteration{
		ID:          uuid.Must(uuid.NewV7()).String(),
		RunID:       runID,
		Iteration:   req.Iteration,
		Description: req.Description,
	}
	if err := s.store.CreateRunIteration(r.Context(), iter); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create run iteration")
		return
	}

	respondJSON(w, http.StatusCreated, iter)
}

// resolveGuardrailInt64 returns the job-level limit if set, otherwise the quota-level limit.
func resolveGuardrailInt64(quotaLimit, jobLimit int64) int64 {
	if jobLimit > 0 {
		return jobLimit
	}
	return quotaLimit
}

// resolveGuardrailInt returns the job-level limit if set, otherwise the quota-level limit.
func resolveGuardrailInt(quotaLimit, jobLimit int) int {
	if jobLimit > 0 {
		return jobLimit
	}
	return quotaLimit
}
