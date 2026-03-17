package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleSDKResources(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	var req struct {
		MemoryMB      float64 `json:"memory_mb"`
		MemoryPercent float64 `json:"memory_percent"`
		CPUPercent    float64 `json:"cpu_percent"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate: non-negative values.
	if req.MemoryMB < 0 {
		respondError(w, r, http.StatusBadRequest, "memory_mb must be non-negative")
		return
	}
	if req.MemoryPercent < 0 || req.MemoryPercent > 100 {
		respondError(w, r, http.StatusBadRequest, "memory_percent must be between 0 and 100")
		return
	}
	if req.CPUPercent < 0 || req.CPUPercent > 100 {
		respondError(w, r, http.StatusBadRequest, "cpu_percent must be between 0 and 100")
		return
	}

	data, _ := json.Marshal(map[string]any{
		"memory_mb":      req.MemoryMB,
		"memory_percent": req.MemoryPercent,
		"cpu_percent":    req.CPUPercent,
	})

	level := "info"
	message := fmt.Sprintf("memory: %.1fMB (%.1f%%), cpu: %.1f%%", req.MemoryMB, req.MemoryPercent, req.CPUPercent)
	if req.MemoryPercent >= 90 {
		level = "error"
		message = fmt.Sprintf("memory pressure critical: %.1fMB (%.1f%%)", req.MemoryMB, req.MemoryPercent)
	} else if req.MemoryPercent >= 80 {
		level = "warn"
		message = fmt.Sprintf("memory pressure warning: %.1fMB (%.1f%%)", req.MemoryMB, req.MemoryPercent)
	}

	event := &domain.RunEvent{
		RunID:   runID,
		Type:    domain.EventType("resource_sample"),
		Level:   level,
		Message: message,
		Data:    json.RawMessage(data),
	}
	if err := s.store.InsertEvent(r.Context(), event); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to store resource sample")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}
