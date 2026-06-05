package worker

import (
	"context"
	"fmt"

	"strait/internal/domain"
	"strait/internal/store"
)

func defaultExecutionPolicy(job *domain.Job) executionPolicy {
	return executionPolicy{
		maxAttempts:      job.MaxAttempts,
		timeoutSecs:      job.TimeoutSecs,
		retryBackoff:     domain.RetryBackoffExponential,
		retryInitialSecs: 1,
		retryMaxSecs:     3600,
	}
}

func (e *Executor) resolveExecutionPolicy(ctx context.Context, run *domain.JobRun, fallback executionPolicy) (executionPolicy, error) {
	if run.WorkflowStepRunID == "" {
		return fallback, nil
	}

	stepRun, err := e.store.GetWorkflowStepRun(ctx, run.WorkflowStepRunID)
	if err != nil || stepRun == nil {
		if err != nil {
			return fallback, err
		}
		return fallback, fmt.Errorf("%w: %s", store.ErrWorkflowStepRunNotFound, run.WorkflowStepRunID)
	}

	runVersion, err := e.getWorkflowRunVersion(ctx, stepRun.WorkflowRunID)
	if err != nil {
		return fallback, err
	}
	if runVersion.WorkflowID == "" {
		return fallback, nil
	}

	steps, err := e.getWorkflowStepsForVersion(ctx, runVersion.WorkflowID, runVersion.Version)
	if err != nil {
		return fallback, err
	}

	for _, step := range steps {
		if step.StepRef != stepRun.StepRef {
			continue
		}

		if step.RetryMaxAttempts > 0 {
			fallback.maxAttempts = step.RetryMaxAttempts
		}
		if step.RetryBackoff != "" {
			fallback.retryBackoff = step.RetryBackoff
		}
		if step.RetryInitialDelaySecs > 0 {
			fallback.retryInitialSecs = step.RetryInitialDelaySecs
		}
		if step.RetryMaxDelaySecs > 0 {
			fallback.retryMaxSecs = step.RetryMaxDelaySecs
		}
		if step.TimeoutSecsOverride > 0 {
			fallback.timeoutSecs = step.TimeoutSecsOverride
		}
		return fallback, nil
	}

	return fallback, nil
}

func (e *Executor) getWorkflowRunVersion(ctx context.Context, workflowRunID string) (workflowRunVersion, error) {
	loader := func(loadCtx context.Context, key string) (workflowRunVersion, error) {
		wfRun, err := e.store.GetWorkflowRun(loadCtx, key)
		if err != nil || wfRun == nil {
			if err != nil {
				return workflowRunVersion{}, err
			}
			return workflowRunVersion{}, nil
		}
		return workflowRunVersion{WorkflowID: wfRun.WorkflowID, Version: wfRun.WorkflowVersion}, nil
	}
	if e.runVersionCache == nil {
		return loader(ctx, workflowRunID)
	}
	return e.runVersionCache.Load(ctx, workflowRunID, loader)
}

func (e *Executor) getWorkflowStepsForVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
	key := workflowStepsVersionKey{WorkflowID: workflowID, Version: version}
	loader := func(loadCtx context.Context, loadKey workflowStepsVersionKey) ([]domain.WorkflowStep, error) {
		steps, err := e.store.ListStepsByWorkflowVersion(loadCtx, loadKey.WorkflowID, loadKey.Version)
		if err != nil {
			return nil, err
		}
		return domain.CloneWorkflowSteps(steps), nil
	}
	if e.stepsVersionCache == nil {
		return loader(ctx, key)
	}
	return e.stepsVersionCache.Load(ctx, key, loader)
}
