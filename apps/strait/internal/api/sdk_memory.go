package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

// resolveAgentIDForDO returns the agent ID if this run's job uses DO memory backend.
// Returns empty string if DO is not configured or the job is not an agent.
func (s *Server) resolveAgentIDForDO(ctx context.Context, jobID string) string {
	if s.doMemoryClient == nil || s.config == nil || s.config.AgentMemoryBackend != "durable_objects" {
		return ""
	}
	agent, err := s.store.GetAgentByJobID(ctx, jobID)
	if err != nil || agent == nil {
		return ""
	}
	return agent.ID
}

type SDKSetMemoryRequest struct {
	Value   json.RawMessage `json:"value" validate:"required"`
	TTLSecs *int            `json:"ttl_secs,omitempty"`
}
type SDKSetMemoryInput struct {
	RunID string `path:"runID"`
	Key   string `path:"key"`
	Body  SDKSetMemoryRequest
}
type SDKSetMemoryOutput struct{ Body *domain.JobMemory }

func (s *Server) handleSDKSetMemory(ctx context.Context, input *SDKSetMemoryInput) (*SDKSetMemoryOutput, error) {
	key := input.Key
	if len(key) > 256 {
		return nil, huma.Error400BadRequest("memory key must be 256 characters or fewer")
	}
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	run, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get run")
	}
	quota, err := s.store.GetProjectQuota(ctx, run.ProjectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get project quota")
	}
	maxPerKey := 1048576
	maxPerJob := 10485760
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
	// Route to Durable Objects if configured for this agent.
	if agentID := s.resolveAgentIDForDO(ctx, run.JobID); agentID != "" {
		ttlSecs := 0
		if req.TTLSecs != nil {
			ttlSecs = *req.TTLSecs
		}
		entry, doErr := s.doMemoryClient.Set(ctx, agentID, key, req.Value, ttlSecs, maxPerKey, maxPerJob)
		if doErr != nil {
			slog.Warn("DO memory set failed, falling back to Postgres", "agent_id", agentID, "error", doErr)
		} else {
			return &SDKSetMemoryOutput{Body: &domain.JobMemory{
				JobID: run.JobID, ProjectID: run.ProjectID, MemoryKey: key,
				Value: entry.Value, SizeBytes: entry.SizeBytes,
			}}, nil
		}
	}

	mem := &domain.JobMemory{JobID: run.JobID, ProjectID: run.ProjectID, MemoryKey: key, Value: req.Value, SizeBytes: len(req.Value), TTLExpiresAt: ttlExpiresAt}
	if err := s.store.UpsertJobMemoryWithQuota(ctx, mem, maxPerKey, maxPerJob); err != nil {
		switch {
		case errors.Is(err, store.ErrJobMemoryPerKeyLimitExceeded):
			return nil, huma.Error400BadRequest("value exceeds per-key memory limit")
		case errors.Is(err, store.ErrJobMemoryPerJobLimitExceeded):
			return nil, huma.Error400BadRequest("value exceeds per-job memory limit")
		default:
			return nil, huma.Error500InternalServerError("failed to upsert job memory")
		}
	}
	return &SDKSetMemoryOutput{Body: mem}, nil
}

type SDKGetMemoryInput struct {
	RunID string `path:"runID"`
	Key   string `path:"key"`
}
type SDKGetMemoryOutput struct{ Body *domain.JobMemory }

func (s *Server) handleSDKGetMemory(ctx context.Context, input *SDKGetMemoryInput) (*SDKGetMemoryOutput, error) {
	run, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get run")
	}
	if run == nil {
		return nil, huma.Error404NotFound("run not found")
	}
	if agentID := s.resolveAgentIDForDO(ctx, run.JobID); agentID != "" {
		entry, doErr := s.doMemoryClient.Get(ctx, agentID, input.Key)
		if doErr == nil {
			if entry == nil {
				return nil, huma.Error404NotFound("memory key not found")
			}
			return &SDKGetMemoryOutput{Body: &domain.JobMemory{
				JobID: run.JobID, ProjectID: run.ProjectID, MemoryKey: input.Key,
				Value: entry.Value, SizeBytes: entry.SizeBytes,
			}}, nil
		}
		slog.Warn("DO memory get failed, falling back to Postgres", "agent_id", agentID, "error", doErr)
	}

	mem, err := s.store.GetJobMemory(ctx, run.JobID, input.Key)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get job memory")
	}
	if mem == nil {
		return nil, huma.Error404NotFound("memory key not found")
	}
	return &SDKGetMemoryOutput{Body: mem}, nil
}

type SDKListMemoryOutput struct{ Body any }

func (s *Server) handleSDKListMemory(ctx context.Context, input *SDKRunIDInput) (*SDKListMemoryOutput, error) {
	run, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get run")
	}
	if agentID := s.resolveAgentIDForDO(ctx, run.JobID); agentID != "" {
		entries, doErr := s.doMemoryClient.List(ctx, agentID)
		if doErr == nil {
			return &SDKListMemoryOutput{Body: entries}, nil
		}
		slog.Warn("DO memory list failed, falling back to Postgres", "agent_id", agentID, "error", doErr)
	}

	items, err := s.store.ListJobMemory(ctx, run.JobID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list job memory")
	}
	return &SDKListMemoryOutput{Body: items}, nil
}

type SDKDeleteMemoryInput struct {
	RunID string `path:"runID"`
	Key   string `path:"key"`
}

func (s *Server) handleSDKDeleteMemory(ctx context.Context, input *SDKDeleteMemoryInput) (*struct{}, error) {
	run, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get run")
	}
	if agentID := s.resolveAgentIDForDO(ctx, run.JobID); agentID != "" {
		if doErr := s.doMemoryClient.Delete(ctx, agentID, input.Key); doErr == nil {
			return nil, nil
		}
		slog.Warn("DO memory delete failed, falling back to Postgres", "agent_id", agentID, "error", err)
	}

	if err := s.store.DeleteJobMemory(ctx, run.JobID, input.Key); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete job memory")
	}
	return nil, nil
}
