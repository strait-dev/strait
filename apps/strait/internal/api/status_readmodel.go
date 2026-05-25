package api

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
)

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
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *Server) getRunFromStatusReadModel(ctx context.Context, id string) (*domain.JobRun, bool) {
	if s.runStatusReadModel == nil {
		return nil, false
	}
	got, err := s.runStatusReadModel.Get(ctx, id)
	if err != nil || got.Value == nil {
		return nil, false
	}
	return got.Value, true
}

func (s *Server) getWorkflowRunFromStatusReadModel(ctx context.Context, id string) (*domain.WorkflowRun, bool) {
	if s.workflowRunStatusReadModel == nil {
		return nil, false
	}
	got, err := s.workflowRunStatusReadModel.Get(ctx, id)
	if err != nil || got.Value == nil {
		return nil, false
	}
	return got.Value, true
}

func (s *Server) getRunWithStatusReadModel(ctx context.Context, id string) (*domain.JobRun, error) {
	if run, ok := s.getRunFromStatusReadModel(ctx, id); ok {
		return run, nil
	}
	run, err := s.store.GetRun(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.runStatusReadModel != nil {
		_ = s.runStatusReadModel.SetIfCold(ctx, id, run)
	}
	return run, nil
}

func (s *Server) getWorkflowRunWithStatusReadModel(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	if run, ok := s.getWorkflowRunFromStatusReadModel(ctx, id); ok {
		return run, nil
	}
	run, err := s.store.GetWorkflowRun(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.workflowRunStatusReadModel != nil {
		_ = s.workflowRunStatusReadModel.SetIfCold(ctx, id, run)
	}
	return run, nil
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
