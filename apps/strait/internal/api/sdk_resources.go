package api

import (
	"context"
	"encoding/json"
	"fmt"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

type SDKResourcesRequest struct {
	MemoryMB      float64 `json:"memory_mb"`
	MemoryPercent float64 `json:"memory_percent"`
	CPUPercent    float64 `json:"cpu_percent"`
}
type SDKResourcesInput struct {
	RunID string `path:"runID"`
	Body  SDKResourcesRequest
}
type SDKResourcesOutput struct{ Body map[string]string }

func (s *Server) handleSDKResources(ctx context.Context, input *SDKResourcesInput) (*SDKResourcesOutput, error) {
	req := input.Body
	if req.MemoryMB < 0 {
		return nil, huma.Error400BadRequest("memory_mb must be non-negative")
	}
	if req.MemoryPercent < 0 || req.MemoryPercent > 100 {
		return nil, huma.Error400BadRequest("memory_percent must be between 0 and 100")
	}
	if req.CPUPercent < 0 || req.CPUPercent > 100 {
		return nil, huma.Error400BadRequest("cpu_percent must be between 0 and 100")
	}
	data, _ := marshalSDKResourceSampleData(req.MemoryMB, req.MemoryPercent, req.CPUPercent)
	level := "info"
	message := fmt.Sprintf("memory: %.1fMB (%.1f%%), cpu: %.1f%%", req.MemoryMB, req.MemoryPercent, req.CPUPercent)
	if req.MemoryPercent >= 90 {
		level = "error"
		message = fmt.Sprintf("memory pressure critical: %.1fMB (%.1f%%)", req.MemoryMB, req.MemoryPercent)
	} else if req.MemoryPercent >= 80 {
		level = "warn"
		message = fmt.Sprintf("memory pressure warning: %.1fMB (%.1f%%)", req.MemoryMB, req.MemoryPercent)
	}
	event := &domain.RunEvent{RunID: input.RunID, Type: domain.EventType("resource_sample"), Level: level, Message: message, Data: json.RawMessage(data)}
	var err error
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		err = guardedStore.InsertEventForActiveRun(ctx, event, runTokenAttemptFromContext(ctx))
	} else {
		err = s.store.InsertEvent(ctx, event)
	}
	if err != nil {
		if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
			return nil, sdkErr
		}
		return nil, huma.Error500InternalServerError("failed to store resource sample")
	}
	return &SDKResourcesOutput{Body: map[string]string{"status": "ok"}}, nil
}
