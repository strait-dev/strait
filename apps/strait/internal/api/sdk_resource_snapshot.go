package api

import (
	"context"
	"encoding/json"
	"fmt"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

type SDKResourceSnapshotRequest struct {
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryMB       float64 `json:"memory_mb"`
	MemoryLimitMB  float64 `json:"memory_limit_mb"`
	NetworkRxBytes int64   `json:"network_rx_bytes"`
	NetworkTxBytes int64   `json:"network_tx_bytes"`
}
type SDKResourceSnapshotInput struct {
	RunID string `path:"runID"`
	Body  SDKResourceSnapshotRequest
}
type SDKResourceSnapshotOutput struct{ Body *domain.RunResourceSnapshot }

func (s *Server) handleSDKResourceSnapshot(ctx context.Context, input *SDKResourceSnapshotInput) (*SDKResourceSnapshotOutput, error) {
	req := input.Body
	snapshot := &domain.RunResourceSnapshot{RunID: input.RunID, CPUPercent: req.CPUPercent, MemoryMB: req.MemoryMB, MemoryLimitMB: req.MemoryLimitMB, NetworkRxBytes: req.NetworkRxBytes, NetworkTxBytes: req.NetworkTxBytes}
	if err := s.store.CreateRunResourceSnapshot(ctx, snapshot); err != nil {
		return nil, huma.Error500InternalServerError("failed to create resource snapshot")
	}
	if req.MemoryLimitMB > 0 && req.MemoryMB > req.MemoryLimitMB*0.9 {
		data, _ := json.Marshal(map[string]any{"memory_mb": req.MemoryMB, "memory_limit_mb": req.MemoryLimitMB, "usage_percent": req.MemoryMB / req.MemoryLimitMB * 100})
		event := &domain.RunEvent{RunID: input.RunID, Type: domain.EventType("resource.oom_risk"), Level: "warn", Message: fmt.Sprintf("memory usage %.0fMB exceeds 90%% of limit %.0fMB", req.MemoryMB, req.MemoryLimitMB), Data: json.RawMessage(data)}
		if guardedStore, ok := s.store.(activeRunMutationStore); ok {
			_ = guardedStore.InsertEventForActiveRun(ctx, event, runTokenAttemptFromContext(ctx))
		} else {
			_ = s.store.InsertEvent(ctx, event)
		}
	}
	return &SDKResourceSnapshotOutput{Body: snapshot}, nil
}
