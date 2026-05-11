package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

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
	quota, err := s.quotaCache.Get(ctx, run.ProjectID)
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
	mem := &domain.JobMemory{JobID: run.JobID, ProjectID: run.ProjectID, MemoryKey: key, Value: req.Value, SizeBytes: len(req.Value), TTLExpiresAt: ttlExpiresAt}
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		err = guardedStore.UpsertJobMemoryWithQuotaForActiveRun(ctx, input.RunID, mem, maxPerKey, maxPerJob, runTokenAttemptFromContext(ctx))
	} else {
		err = s.store.UpsertJobMemoryWithQuota(ctx, mem, maxPerKey, maxPerJob)
	}
	if err != nil {
		switch {
		case errors.Is(err, store.ErrRunConflict), errors.Is(err, store.ErrRunNotFound):
			if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
				return nil, sdkErr
			}
			return nil, huma.Error409Conflict("run is not active for this SDK token")
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
	var mem *domain.JobMemory
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		mem, err = guardedStore.GetJobMemoryForActiveRun(ctx, input.RunID, run.JobID, input.Key, runTokenAttemptFromContext(ctx))
	} else {
		mem, err = s.store.GetJobMemory(ctx, run.JobID, input.Key)
	}
	if err != nil {
		if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
			return nil, sdkErr
		}
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
	var items []domain.JobMemory
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		items, err = guardedStore.ListJobMemoryForActiveRun(ctx, input.RunID, run.JobID, runTokenAttemptFromContext(ctx))
	} else {
		items, err = s.store.ListJobMemory(ctx, run.JobID)
	}
	if err != nil {
		if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
			return nil, sdkErr
		}
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
	var deleteErr error
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		deleteErr = guardedStore.DeleteJobMemoryForActiveRun(ctx, input.RunID, run.JobID, input.Key, runTokenAttemptFromContext(ctx))
	} else {
		deleteErr = s.store.DeleteJobMemory(ctx, run.JobID, input.Key)
	}
	if deleteErr != nil {
		if sdkErr := s.guardedSDKMutationError(ctx, deleteErr); sdkErr != nil {
			return nil, sdkErr
		}
		return nil, huma.Error500InternalServerError("failed to delete job memory")
	}
	return nil, nil
}
