package worker

import (
	"context"

	"strait/internal/domain"
)

// applyEnvironmentEndpointOverride only swaps the URL when the override passes
// SSRF validation and the dispatch has no job secrets to redirect.
func (e *Executor) applyEnvironmentEndpointOverride(ctx context.Context, run *domain.JobRun, job *domain.Job) {
	if job.EnvironmentID == "" {
		return
	}
	envVars, err := e.store.GetResolvedEnvironmentVariables(ctx, job.ProjectID, job.EnvironmentID)
	if err != nil {
		e.logger.Warn("failed to resolve environment variables", "run_id", run.ID, "environment_id", job.EnvironmentID, "error", err)
		return
	}
	override := envVars["ENDPOINT_URL"]
	if override == "" {
		return
	}
	if err := validateEndpointURL(override); err != nil {
		e.logger.Warn("environment ENDPOINT_URL failed SSRF validation",
			"run_id", run.ID,
			"environment_id", job.EnvironmentID,
			"error", err,
		)
		return
	}
	secrets, err := e.dispatchSecrets(ctx, job)
	if err != nil {
		e.logger.Warn("environment ENDPOINT_URL ignored because dispatch secrets could not be checked",
			"run_id", run.ID,
			"environment_id", job.EnvironmentID,
			"error", err,
		)
		return
	}
	if len(secrets) > 0 {
		e.logger.Warn("environment ENDPOINT_URL ignored because job dispatch includes secrets",
			"run_id", run.ID,
			"environment_id", job.EnvironmentID,
		)
		return
	}
	e.logger.Info("overriding endpoint URL from environment",
		"run_id", run.ID,
		"environment_id", job.EnvironmentID,
	)
	job.EndpointURL = override
}
