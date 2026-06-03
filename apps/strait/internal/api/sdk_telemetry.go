package api

import (
	"context"
	"encoding/json"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type SDKOutputRequest struct {
	OutputKey string          `json:"output_key" validate:"required"`
	Schema    json.RawMessage `json:"schema,omitempty"`
	Value     json.RawMessage `json:"value" validate:"required"`
}
type SDKOutputInput struct {
	RunID string `path:"runID"`
	Body  SDKOutputRequest
}
type SDKOutputOutput struct{ Body *domain.RunOutput }

func (s *Server) handleSDKOutput(ctx context.Context, input *SDKOutputInput) (*SDKOutputOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := validatePayloadAgainstSchema(req.Value, req.Schema); err != nil {
		return nil, huma.Error400BadRequest("output schema validation failed: " + err.Error())
	}
	output := &domain.RunOutput{ID: uuid.Must(uuid.NewV7()).String(), RunID: runID, OutputKey: req.OutputKey, Schema: req.Schema, Value: req.Value}
	var err error
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		err = guardedStore.UpsertRunOutputForActiveRun(ctx, output, runTokenAttemptFromContext(ctx))
	} else {
		err = s.store.UpsertRunOutput(ctx, output)
	}
	if err != nil {
		if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
			return nil, sdkErr
		}
		return nil, huma.Error500InternalServerError("failed to upsert run output")
	}
	return &SDKOutputOutput{Body: output}, nil
}
