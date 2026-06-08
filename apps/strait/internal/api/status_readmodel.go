package api

import (
	"context"
	"maps"
	"time"

	"github.com/redis/go-redis/v9"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
)

type runStatusVersionStore interface {
	GetRunWithCacheVersion(context.Context, string) (*domain.JobRun, int64, error)
}

type workflowRunStatusVersionStore interface {
	GetWorkflowRunWithCacheVersion(context.Context, string) (*domain.WorkflowRun, int64, error)
}

func cloneJobRunForStatusCache(run *domain.JobRun) *domain.JobRun {
	if run == nil {
		return nil
	}
	cp := *run
	cp.Tags = cloneStringMap(run.Tags)
	cp.Metadata = cloneStringMap(run.Metadata)
	cp.Payload = append([]byte(nil), run.Payload...)
	cp.Result = append([]byte(nil), run.Result...)
	return &cp
}

func cloneWorkflowRunForStatusCache(run *domain.WorkflowRun) *domain.WorkflowRun {
	if run == nil {
		return nil
	}
	cp := *run
	cp.Tags = cloneStringMap(run.Tags)
	cp.Payload = append([]byte(nil), run.Payload...)
	cp.TraceContext = cloneStringMap(run.TraceContext)
	return &cp
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

func (s *Server) getRunFromStatusReadModel(ctx context.Context, id string) (*domain.JobRun, int64, bool) {
	if s.runStatusReadModel == nil {
		return nil, 0, false
	}
	got, err := s.runStatusReadModel.Get(ctx, id)
	if err != nil || got.Value == nil {
		return nil, 0, false
	}
	return got.Value, got.Version, true
}

func (s *Server) getWorkflowRunFromStatusReadModel(ctx context.Context, id string) (*domain.WorkflowRun, int64, bool) {
	if s.workflowRunStatusReadModel == nil {
		return nil, 0, false
	}
	got, err := s.workflowRunStatusReadModel.Get(ctx, id)
	if err != nil || got.Value == nil {
		return nil, 0, false
	}
	return got.Value, got.Version, true
}

func (s *Server) getRunWithStatusReadModel(ctx context.Context, id string) (*domain.JobRun, error) {
	if run, cachedVersion, ok := s.getRunFromStatusReadModel(ctx, id); ok {
		if run.Status.IsTerminal() {
			return run, nil
		}
		fresh, version, loadErr := s.loadRunForStatusReadModel(ctx, id)
		if loadErr == nil {
			if fresh != nil && (version > cachedVersion || fresh.Status.IsTerminal()) {
				if s.runStatusReadModel != nil {
					_, _ = s.runStatusReadModel.CompareAndSet(ctx, id, fresh, version)
				}
				return fresh, nil
			}
		}
		return run, nil
	}
	run, version, err := s.loadRunForStatusReadModel(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.runStatusReadModel != nil {
		_ = s.runStatusReadModel.SetIfColdVersion(ctx, id, run, version)
	}
	return run, nil
}

func (s *Server) getWorkflowRunWithStatusReadModel(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	if run, cachedVersion, ok := s.getWorkflowRunFromStatusReadModel(ctx, id); ok {
		if run.Status.IsTerminal() {
			return run, nil
		}
		fresh, version, loadErr := s.loadWorkflowRunForStatusReadModel(ctx, id)
		if loadErr == nil {
			if fresh != nil && (version > cachedVersion || fresh.Status.IsTerminal()) {
				if s.workflowRunStatusReadModel != nil {
					_, _ = s.workflowRunStatusReadModel.CompareAndSet(ctx, id, fresh, version)
				}
				return fresh, nil
			}
		}
		return run, nil
	}
	run, version, err := s.loadWorkflowRunForStatusReadModel(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.workflowRunStatusReadModel != nil {
		_ = s.workflowRunStatusReadModel.SetIfColdVersion(ctx, id, run, version)
	}
	return run, nil
}

func (s *Server) loadRunForStatusReadModel(ctx context.Context, id string) (*domain.JobRun, int64, error) {
	if store, ok := s.store.(runStatusVersionStore); ok {
		run, version, err := store.GetRunWithCacheVersion(ctx, id)
		if err != nil {
			return nil, 0, err
		}
		return run, statusReadModelVersion(version), nil
	}
	run, err := s.store.GetRun(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	if run == nil {
		return nil, 1, nil
	}
	return run, statusReadModelVersion(run.CacheVersion), nil
}

func (s *Server) loadWorkflowRunForStatusReadModel(ctx context.Context, id string) (*domain.WorkflowRun, int64, error) {
	if store, ok := s.store.(workflowRunStatusVersionStore); ok {
		run, version, err := store.GetWorkflowRunWithCacheVersion(ctx, id)
		if err != nil {
			return nil, 0, err
		}
		return run, statusReadModelVersion(version), nil
	}
	run, err := s.store.GetWorkflowRun(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	if run == nil {
		return nil, 1, nil
	}
	return run, statusReadModelVersion(run.CacheVersion), nil
}

func statusReadModelVersion(version int64) int64 {
	if version <= 0 {
		return 1
	}
	return version
}

type statusReadModels struct {
	run         *straitcache.ReadModel[*domain.JobRun]
	workflowRun *straitcache.ReadModel[*domain.WorkflowRun]
}

func newStatusReadModels(client redis.Cmdable, ttl time.Duration) statusReadModels {
	if client == nil || ttl <= 0 {
		return statusReadModels{}
	}
	return statusReadModels{
		run: straitcache.NewReadModel[*domain.JobRun](straitcache.ReadModelConfig[*domain.JobRun]{
			Client:    client,
			Namespace: "status_job_run",
			TTL:       ttl,
			Clone:     cloneJobRunForStatusCache,
			Sanitize:  cloneJobRunForStatusCache,
		}),
		workflowRun: straitcache.NewReadModel[*domain.WorkflowRun](straitcache.ReadModelConfig[*domain.WorkflowRun]{
			Client:    client,
			Namespace: "status_workflow_run",
			TTL:       ttl,
			Clone:     cloneWorkflowRunForStatusCache,
			Sanitize:  cloneWorkflowRunForStatusCache,
		}),
	}
}
