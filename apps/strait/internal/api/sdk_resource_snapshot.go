package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleSDKResourceSnapshot(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	var req struct {
		CPUPercent     float64 `json:"cpu_percent"`
		MemoryMB       float64 `json:"memory_mb"`
		MemoryLimitMB  float64 `json:"memory_limit_mb"`
		NetworkRxBytes int64   `json:"network_rx_bytes"`
		NetworkTxBytes int64   `json:"network_tx_bytes"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	snapshot := &domain.RunResourceSnapshot{
		RunID:          runID,
		CPUPercent:     req.CPUPercent,
		MemoryMB:       req.MemoryMB,
		MemoryLimitMB:  req.MemoryLimitMB,
		NetworkRxBytes: req.NetworkRxBytes,
		NetworkTxBytes: req.NetworkTxBytes,
	}

	if err := s.store.CreateRunResourceSnapshot(r.Context(), snapshot); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create resource snapshot")
		return
	}

	if req.MemoryLimitMB > 0 && req.MemoryMB > req.MemoryLimitMB*0.9 {
		data, _ := json.Marshal(map[string]any{
			"memory_mb":       req.MemoryMB,
			"memory_limit_mb": req.MemoryLimitMB,
			"usage_percent":   req.MemoryMB / req.MemoryLimitMB * 100,
		})
		_ = s.store.InsertEvent(r.Context(), &domain.RunEvent{
			RunID:   runID,
			Type:    domain.EventType("resource.oom_risk"),
			Level:   "warn",
			Message: fmt.Sprintf("memory usage %.0fMB exceeds 90%% of limit %.0fMB", req.MemoryMB, req.MemoryLimitMB),
			Data:    json.RawMessage(data),
		})
	}

	respondJSON(w, http.StatusCreated, snapshot)
}
