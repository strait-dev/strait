package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

type SDKStreamChunkRequest struct {
	Chunk    string `json:"chunk"`
	StreamID string `json:"stream_id,omitempty"`
	Done     bool   `json:"done,omitempty"`
}
type SDKStreamChunkInput struct {
	RunID string `path:"runID"`
	Body  SDKStreamChunkRequest
}
type SDKStreamChunkOutput struct{ Body map[string]string }

func (s *Server) handleSDKStreamChunk(ctx context.Context, input *SDKStreamChunkInput) (*SDKStreamChunkOutput, error) {
	if s.pubsub == nil {
		return &SDKStreamChunkOutput{Body: map[string]string{"status": "ok"}}, nil
	}
	if err := s.ensureSDKRunActive(ctx, input.RunID); err != nil {
		return nil, err
	}
	streamID := input.Body.StreamID
	if streamID == "" {
		streamID = "default"
	}
	payload, err := marshalSDKStreamChunkPayload(input.Body.Chunk, streamID, input.Body.Done, time.Now().UTC())
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to marshal chunk")
	}
	if err := s.pubsub.Publish(ctx, "run_stream:"+input.RunID, payload); err != nil {
		slog.Warn("failed to publish stream chunk", "run_id", input.RunID, "error", err)
	}
	return &SDKStreamChunkOutput{Body: map[string]string{"status": "ok"}}, nil
}
