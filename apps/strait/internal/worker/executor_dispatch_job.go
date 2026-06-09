package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

const workflowStepVisibilityRetryDelay = 250 * time.Millisecond

// resolveJobForRun loads the job configuration for a run, applying version
// policy rules. For "pin" (default), returns the enqueue-time version. For
// "latest", upgrades to the current version. For "minor", upgrades only if
// the current version is marked backwards_compatible.
func (e *Executor) resolveJobForRun(ctx context.Context, run *domain.JobRun) (*domain.Job, error) {
	var current *domain.Job
	bypassCache := false
	if e.jobCache != nil {
		if cached, err := e.jobCache.Get(ctx, run.JobID); err == nil {
			current = cloneJob(cached)
			if versionPolicyRequiresCurrentJob(current.VersionPolicy) {
				current = nil
				bypassCache = true
			}
		}
	}

	if current == nil {
		loadCurrent := func(loadCtx context.Context, jobID string) (*domain.Job, error) {
			job, gerr := e.store.GetJob(loadCtx, jobID)
			if gerr != nil {
				return nil, gerr
			}
			return cloneJob(job), nil
		}
		var err error
		if e.jobCache != nil && !bypassCache {
			current, err = e.jobCache.Load(ctx, run.JobID, loadCurrent)
		} else {
			current, err = loadCurrent(ctx, run.JobID)
			if err == nil && e.jobCache != nil {
				_ = e.jobCache.Set(ctx, run.JobID, current)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("load current job: %w", err)
		}
		current = cloneJob(current)
	}

	if current.Version == run.JobVersion {
		return cloneJob(current), nil
	}

	switch current.VersionPolicy {
	case domain.VersionPolicyLatest:
		e.logger.Info("version policy upgrade",
			"run_id", run.ID,
			"policy", "latest",
			"from_version", run.JobVersion,
			"to_version", current.Version,
		)
		run.JobVersion = current.Version
		run.JobVersionID = current.VersionID
		return cloneJob(current), nil

	case domain.VersionPolicyMinor:
		if current.BackwardsCompatible {
			e.logger.Info("version policy upgrade",
				"run_id", run.ID,
				"policy", "minor",
				"from_version", run.JobVersion,
				"to_version", current.Version,
			)
			run.JobVersion = current.Version
			run.JobVersionID = current.VersionID
			return cloneJob(current), nil
		}
		e.logger.Info("version policy: minor upgrade skipped (not backwards compatible)",
			"run_id", run.ID,
			"from_version", run.JobVersion,
			"current_version", current.Version,
		)
		// Fall through to load the enqueue-time version.

	case domain.VersionPolicyPin, "":
	}

	loadVersion := func(loadCtx context.Context, key jobVersionKey) (*domain.Job, error) {
		job, err := e.store.GetJobAtVersion(loadCtx, key.JobID, key.Version)
		if err != nil {
			return nil, err
		}
		return cloneJob(job), nil
	}
	if e.jobVersionCache != nil {
		return e.jobVersionCache.Load(ctx, jobVersionKey{JobID: run.JobID, Version: run.JobVersion}, loadVersion)
	}
	return loadVersion(ctx, jobVersionKey{JobID: run.JobID, Version: run.JobVersion})
}

func versionPolicyRequiresCurrentJob(policy domain.VersionPolicy) bool {
	return policy == domain.VersionPolicyLatest || policy == domain.VersionPolicyMinor
}

func (e *Executor) resolveDispatchJobAndPolicy(ctx context.Context, run *domain.JobRun) (*domain.Job, executionPolicy, bool) {
	job, err := e.resolveJobForRun(ctx, run)
	if err != nil || job == nil {
		e.logger.Error(
			"job lookup failed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"job_version", run.JobVersion,
			"error", err,
		)
		e.handleSystemFailure(ctx, run, "job not found")
		return nil, executionPolicy{}, false
	}

	policy, err := e.resolveExecutionPolicy(ctx, run, defaultExecutionPolicy(job))
	if err != nil {
		if errors.Is(err, store.ErrWorkflowStepRunNotFound) {
			retryAt := time.Now().Add(workflowStepVisibilityRetryDelay)
			e.logger.Warn("workflow step run not visible yet; requeueing run",
				"run_id", run.ID,
				"workflow_step_run_id", run.WorkflowStepRunID,
				"retry_at", retryAt,
			)
			e.snoozeRun(ctx, run, "workflow step run not visible yet", &retryAt)
			return nil, executionPolicy{}, false
		}
		e.logger.Error("failed to resolve execution policy", "run_id", run.ID, "error", err)
		e.handleSystemFailureWithJob(ctx, run, job, "resolve execution policy")
		return nil, executionPolicy{}, false
	}
	return job, policy, true
}
