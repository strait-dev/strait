package api

import (
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

	respondJSON(w, http.StatusCreated, snapshot)
}
