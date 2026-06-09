package cdc

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
)

type CacheReadModelHandlers struct {
	JobRuns          Handler
	WorkflowRuns     Handler
	WorkflowStepRuns Handler
}

func NewCacheReadModelHandlers(client redis.Cmdable, ttl time.Duration, logger *slog.Logger) CacheReadModelHandlers {
	if !cacheReadModelConfigUsable(client, ttl) {
		return CacheReadModelHandlers{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return CacheReadModelHandlers{
		JobRuns: &cacheReadModelHandler[*domain.JobRun]{
			table: "job_runs",
			model: straitcache.NewReadModel[*domain.JobRun](straitcache.ReadModelConfig[*domain.JobRun]{
				Client:    client,
				Namespace: "status_job_run",
				TTL:       ttl,
			}),
			decode: decodeJobRunStatusRecord,
			logger: logger,
		},
		WorkflowRuns: &cacheReadModelHandler[*domain.WorkflowRun]{
			table: "workflow_runs",
			model: straitcache.NewReadModel[*domain.WorkflowRun](straitcache.ReadModelConfig[*domain.WorkflowRun]{
				Client:    client,
				Namespace: "status_workflow_run",
				TTL:       ttl,
			}),
			decode: decodeWorkflowRunStatusRecord,
			logger: logger,
		},
		WorkflowStepRuns: &cacheReadModelHandler[*domain.WorkflowStepRun]{
			table: "workflow_step_runs",
			model: straitcache.NewReadModel[*domain.WorkflowStepRun](straitcache.ReadModelConfig[*domain.WorkflowStepRun]{
				Client:    client,
				Namespace: "status_workflow_step_run",
				TTL:       ttl,
			}),
			decode: decodeWorkflowStepRunStatusRecord,
			logger: logger,
		},
	}
}

func cacheReadModelConfigUsable(client redis.Cmdable, ttl time.Duration) bool {
	return client != nil && ttl > 0
}

type cacheReadModelHandler[V any] struct {
	table  string
	model  *straitcache.ReadModel[V]
	decode func(json.RawMessage) (string, V, int64, error)
	logger *slog.Logger
}

func (h *cacheReadModelHandler[V]) Table() string { return h.table }

func (h *cacheReadModelHandler[V]) Handle(ctx context.Context, msg Message) error {
	id, value, version, err := h.decode(msg.Record)
	if err != nil {
		h.logger.Warn("cdc cache read model ignored malformed record", "table", h.table, "error", err)
		return nil
	}
	if id == "" {
		return nil
	}
	if version <= 0 {
		version = 1
	}
	if msg.Action == ActionDelete {
		if _, err := h.model.DeleteVersion(ctx, id, version); err != nil {
			h.logger.Warn("cdc cache read model delete failed", "table", h.table, "id", id, "error", err)
		}
		return nil
	}
	ok, err := h.model.CompareAndSet(ctx, id, value, version)
	if err != nil {
		h.logger.Warn("cdc cache read model write failed", "table", h.table, "id", id, "version", version, "error", err)
		return nil
	}
	if !ok {
		h.logger.Debug("cdc cache read model rejected stale update", "table", h.table, "id", id, "version", version)
	}
	return nil
}

func decodeJobRunStatusRecord(raw json.RawMessage) (string, *domain.JobRun, int64, error) {
	var envelope struct {
		domain.JobRun
		CacheVersion int64 `json:"cache_version"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", nil, 0, err
	}
	return envelope.ID, &envelope.JobRun, envelope.CacheVersion, nil
}

func decodeWorkflowRunStatusRecord(raw json.RawMessage) (string, *domain.WorkflowRun, int64, error) {
	var envelope struct {
		domain.WorkflowRun
		CacheVersion int64 `json:"cache_version"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", nil, 0, err
	}
	return envelope.ID, &envelope.WorkflowRun, envelope.CacheVersion, nil
}

func decodeWorkflowStepRunStatusRecord(raw json.RawMessage) (string, *domain.WorkflowStepRun, int64, error) {
	var envelope struct {
		domain.WorkflowStepRun
		CacheVersion int64 `json:"cache_version"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", nil, 0, err
	}
	return envelope.ID, &envelope.WorkflowStepRun, envelope.CacheVersion, nil
}
