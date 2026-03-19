package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleSDKSetMemory(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")
	key := chi.URLParam(r, "key")

	if len(key) > 256 {
		respondError(w, r, http.StatusBadRequest, "memory key must be 256 characters or fewer")
		return
	}

	var req struct {
		Value   json.RawMessage `json:"value" validate:"required"`
		TTLSecs *int            `json:"ttl_secs,omitempty"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}

	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}

	sizeBytes := len(req.Value)

	// Check per-key quota.
	quota, err := s.store.GetProjectQuota(r.Context(), run.ProjectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get project quota")
		return
	}
	maxPerKey := 1048576  // 1MB default
	maxPerJob := 10485760 // 10MB default
	if quota != nil {
		if quota.MaxMemoryPerKeyBytes > 0 {
			maxPerKey = quota.MaxMemoryPerKeyBytes
		}
		if quota.MaxMemoryPerJobBytes > 0 {
			maxPerJob = quota.MaxMemoryPerJobBytes
		}
	}

	var ttlExpiresAt *time.Time
	if req.TTLSecs != nil && *req.TTLSecs > 0 {
		t := time.Now().Add(time.Duration(*req.TTLSecs) * time.Second)
		ttlExpiresAt = &t
	}

	mem := &domain.JobMemory{
		JobID:        run.JobID,
		ProjectID:    run.ProjectID,
		MemoryKey:    key,
		Value:        req.Value,
		SizeBytes:    sizeBytes,
		TTLExpiresAt: ttlExpiresAt,
	}

	if err := s.store.UpsertJobMemoryWithQuota(r.Context(), mem, maxPerKey, maxPerJob); err != nil {
		switch {
		case errors.Is(err, store.ErrJobMemoryPerKeyLimitExceeded):
			respondError(w, r, http.StatusBadRequest, "value exceeds per-key memory limit")
		case errors.Is(err, store.ErrJobMemoryPerJobLimitExceeded):
			respondError(w, r, http.StatusBadRequest, "value exceeds per-job memory limit")
		default:
			respondError(w, r, http.StatusInternalServerError, "failed to upsert job memory")
		}
		return
	}

	respondJSON(w, http.StatusCreated, mem)
}

func (s *Server) handleSDKGetMemory(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")
	key := chi.URLParam(r, "key")

	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}
	if run == nil {
		respondError(w, r, http.StatusNotFound, "run not found")
		return
	}

	mem, err := s.store.GetJobMemory(r.Context(), run.JobID, key)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get job memory")
		return
	}
	if mem == nil {
		respondError(w, r, http.StatusNotFound, "memory key not found")
		return
	}

	respondJSON(w, http.StatusOK, mem)
}

func (s *Server) handleSDKListMemory(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}

	items, err := s.store.ListJobMemory(r.Context(), run.JobID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list job memory")
		return
	}

	respondJSON(w, http.StatusOK, items)
}

func (s *Server) handleSDKDeleteMemory(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")
	key := chi.URLParam(r, "key")

	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}

	if err := s.store.DeleteJobMemory(r.Context(), run.JobID, key); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to delete job memory")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
