package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"time"

	"strait/internal/compute"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sourcegraph/conc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// resolveJobForRun loads the job configuration for a run, applying version
// policy rules. For "pin" (default), returns the enqueue-time version. For
// "latest", upgrades to the current version. For "minor", upgrades only if
// the current version is marked backwards_compatible.
func (e *Executor) resolveJobForRun(ctx context.Context, run *domain.JobRun) (*domain.Job, error) {
	// Check job cache first.
	var current *domain.Job
	if e.jobCache != nil {
		if cached, err := e.jobCache.Get(ctx, run.JobID); err == nil {
			current = cached
		}
	}

	if current == nil {
		var err error
		current, err = e.store.GetJob(ctx, run.JobID)
		if err != nil {
			return nil, fmt.Errorf("load current job: %w", err)
		}
		if e.jobCache != nil {
			_ = e.jobCache.Set(ctx, run.JobID, current)
		}
	}

	// If the run is already at the current version, no policy check needed.
	if current.Version == run.JobVersion {
		return current, nil
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
		return current, nil

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
			return current, nil
		}
		e.logger.Info("version policy: minor upgrade skipped (not backwards compatible)",
			"run_id", run.ID,
			"from_version", run.JobVersion,
			"current_version", current.Version,
		)
		// Fall through to load the enqueue-time version.

	case domain.VersionPolicyPin, "":
		// Pin: use the enqueue-time version. Fall through.
	}

	// Load the versioned snapshot.
	return e.store.GetJobAtVersion(ctx, run.JobID, run.JobVersion)
}

func (e *Executor) execute(ctx context.Context, run *domain.JobRun) {
	ec := &ExecutionContext{
		Run:   run,
		Start: time.Now(),
	}

	handler := e.executeInner
	if len(e.middlewares) > 0 {
		handler = Chain(e.middlewares...)(handler)
	}
	handler(ctx, ec)
}

func (e *Executor) executeInner(ctx context.Context, ec *ExecutionContext) {
	run := ec.Run
	executeStart := ec.Start

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
		return
	}
	ec.Job = job

	policy := executionPolicy{
		maxAttempts:      job.MaxAttempts,
		timeoutSecs:      job.TimeoutSecs,
		retryBackoff:     domain.RetryBackoffExponential,
		retryInitialSecs: 1,
		retryMaxSecs:     3600,
	}
	resolved, policyErr := e.resolveExecutionPolicy(ctx, run, policy)
	if policyErr != nil {
		e.logger.Error("failed to resolve execution policy", "run_id", run.ID, "error", policyErr)
		e.handleSystemFailure(ctx, run, "resolve execution policy")
		return
	}
	policy = resolved

	// Billing enforcement: daily and concurrent run limits apply to ALL dispatch modes.
	// Managed-only limits (managed run cap, spending) are checked in managedDispatch.
	if e.billingEnforcer != nil {
		// Check if the project is suspended due to a plan downgrade.
		if err := e.billingEnforcer.CheckProjectSuspended(ctx, job.ProjectID); err != nil {
			e.logger.Warn("project suspended",
				"run_id", run.ID, "project_id", job.ProjectID, "error", err)
			e.handleSystemFailure(ctx, run, err.Error())
			return
		}

		orgID, orgErr := e.billingEnforcer.GetProjectOrgID(ctx, job.ProjectID)
		if orgErr != nil {
			e.logger.Warn("failed to resolve org for billing check",
				"run_id", run.ID, "error", orgErr, "fail_open", true)
		}
		if orgID != "" {
			if err := e.billingEnforcer.CheckDailyRunLimit(ctx, orgID); err != nil {
				e.logger.Warn("org daily run limit exceeded",
					"run_id", run.ID, "org_id", orgID, "error", err)
				e.handleSystemFailure(ctx, run, err.Error())
				return
			}
			if err := e.billingEnforcer.CheckConcurrentRunLimit(ctx, orgID); err != nil {
				e.logger.Warn("org concurrent run limit exceeded",
					"run_id", run.ID, "org_id", orgID, "error", err)
				e.billingEnforcer.DecrDailyRunCount(ctx, orgID)
				e.handleSystemFailure(ctx, run, err.Error())
				return
			}
			decrCtx := context.WithoutCancel(ctx)
			defer e.billingEnforcer.DecrConcurrentRunCount(decrCtx, orgID)
		}
	}

	// Route based on execution mode.
	switch job.ExecutionMode {
	case domain.ExecutionModeManaged:
		e.managedDispatch(ctx, run, job)
		return
	case domain.ExecutionModeHTTP, "":
		// Fall through to HTTP dispatch.
	default:
		e.logger.Error("unknown execution_mode", "run_id", run.ID, "job_id", run.JobID, "execution_mode", job.ExecutionMode)
		e.handleSystemFailure(ctx, run, fmt.Sprintf("unknown execution_mode: %s", job.ExecutionMode))
		return
	}

	// Environment endpoint override: if the job has an environment_id,
	// resolve its variables and check for ENDPOINT_URL override.
	if job.EnvironmentID != "" {
		envVars, envErr := e.store.GetResolvedEnvironmentVariables(ctx, job.EnvironmentID)
		if envErr != nil {
			e.logger.Warn("failed to resolve environment variables", "run_id", run.ID, "environment_id", job.EnvironmentID, "error", envErr)
		} else if override, ok := envVars["ENDPOINT_URL"]; ok && override != "" {
			if err := validateEndpointURL(override); err != nil {
				e.logger.Warn("environment ENDPOINT_URL failed SSRF validation",
					"run_id", run.ID,
					"environment_id", job.EnvironmentID,
					"error", err,
				)
			} else {
				e.logger.Info("overriding endpoint URL from environment",
					"run_id", run.ID,
					"environment_id", job.EnvironmentID,
				)
				job.EndpointURL = override
			}
		}
	}
	// Run circuit breaker, health check, and adaptive timeout queries in parallel.
	// All three depend on job.EndpointURL (which env var resolution may have overridden above).
	var (
		circuitAllowed bool
		circuitRetryAt *time.Time
		circuitErr     error
		healthScore    *domain.EndpointHealthScore
		healthAllowed  bool
		healthErr      error
		adaptiveStats  *store.JobHealthStats
	)

	var prefetchWG conc.WaitGroup
	prefetchWG.Go(func() {
		circuitAllowed, circuitRetryAt, circuitErr = e.store.CanDispatchEndpoint(ctx, job.EndpointURL, time.Now().UTC())
	})
	prefetchWG.Go(func() {
		healthScore, healthAllowed, healthErr = e.healthScorer.CheckHealth(ctx, job.EndpointURL)
	})
	if policy.timeoutSecs > 0 {
		prefetchWG.Go(func() {
			adaptiveStats, _ = e.store.GetJobHealthStats(ctx, job.ID, time.Now().Add(-24*time.Hour))
		})
	}
	prefetchWG.Wait()

	if circuitErr != nil {
		e.logger.Error(
			"circuit breaker check failed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"endpoint", job.EndpointURL,
			"error", circuitErr,
		)
		e.handleSystemFailure(ctx, run, "circuit breaker unavailable")
		return
	}

	if !circuitAllowed {
		e.snoozeRun(ctx, run, "endpoint circuit breaker open", circuitRetryAt)
		return
	}

	// Health score check: block unhealthy endpoints, throttle degraded ones.
	if healthErr != nil {
		e.logger.Warn(
			"health score check failed, proceeding with dispatch",
			"run_id", run.ID,
			"endpoint", job.EndpointURL,
			"error", healthErr,
		)
	} else if !healthAllowed {
		healthRetryAt := NextRetryAt(run.Attempt)
		e.logger.Info(
			"endpoint unhealthy, snoozing run",
			"run_id", run.ID,
			"endpoint", job.EndpointURL,
			"health_score", healthScore.HealthScore,
		)
		e.snoozeRun(ctx, run, "endpoint health score below threshold", &healthRetryAt)
		return
	}

	// Apply health-based concurrency throttling for degraded endpoints.
	effectiveConcurrency := job.MaxConcurrency
	if healthScore != nil {
		effectiveConcurrency = ThrottledConcurrency(healthScore, job.MaxConcurrency)
	}

	acquired := e.tryAcquireBulkheadSlot(job.ID, effectiveConcurrency)
	if !acquired {
		bulkheadRetryAt := NextRetryAt(run.Attempt)
		e.snoozeRun(ctx, run, "job bulkhead at capacity", &bulkheadRetryAt)
		return
	}
	defer e.releaseBulkheadSlot(job.ID, job.MaxConcurrency)

	err = e.store.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
		"started_at": time.Now(),
	})
	if err != nil {
		e.logger.Error(
			"failed to transition to executing",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	run.Status = domain.StatusExecuting
	e.publishEvent(ctx, run, map[string]any{"from": "dequeued", "to": "executing"})

	e.heartbeat.Register(run.ID)
	defer e.heartbeat.Deregister(run.ID)

	timeout := time.Duration(policy.timeoutSecs) * time.Second
	if adaptiveStats != nil && adaptiveStats.P95DurationSecs > 0 {
		adaptiveTimeout := time.Duration(adaptiveStats.P95DurationSecs * 1.5 * float64(time.Second))
		if adaptiveTimeout > timeout {
			timeout = adaptiveTimeout
			e.logger.Debug("using adaptive timeout", "job_id", job.ID, "p95_secs", adaptiveStats.P95DurationSecs, "timeout", timeout)
		}
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, execTrace, err := e.tracedDispatch(execCtx, job, run)
	if execTrace != nil {
		execTrace.TotalMs = durationMillisecondsAtLeastOne(time.Since(executeStart))
		queueWait := max(time.Duration(0), executeStart.Sub(run.CreatedAt))
		execTrace.QueueWaitMs = durationMillisecondsAtLeastOne(queueWait)
		if run.StartedAt != nil {
			dequeue := max(time.Duration(0), executeStart.Sub(*run.StartedAt))
			execTrace.DequeueMs = durationMillisecondsAtLeastOne(dequeue)
		}
	}
	if err != nil {
		if job.FallbackEndpointURL != "" {
			errClass := classifyError(err)
			if shouldUseFallbackForClass(errClass) {
				fallbackResult, fallbackErr := e.dispatchToEndpoint(execCtx, job.FallbackEndpointURL, run, nil)
				if fallbackErr == nil {
					e.handleSuccess(ctx, run, job, fallbackResult, execTrace)
					return
				}
				err = errors.Join(
					fmt.Errorf("primary dispatch failed: %w", err),
					fmt.Errorf("fallback dispatch failed: %w", fallbackErr),
				)
			}
		}

		if execCtx.Err() == context.DeadlineExceeded {
			e.handleTimeout(ctx, run, job, policy, execTrace)
		} else {
			e.handleFailure(ctx, run, job, policy, err, execTrace)
		}
		return
	}

	e.handleSuccess(ctx, run, job, result, execTrace)
}

// managedDispatch dispatches a job run to a container runtime (Fly Machines, Docker).
func (e *Executor) managedDispatch(ctx context.Context, run *domain.JobRun, job *domain.Job) {
	dispatchStart := time.Now()

	// 1. Guard: runtime must be configured.
	if e.containerRuntime == nil {
		e.logger.Error("managed execution not available: COMPUTE_RUNTIME not configured",
			"run_id", run.ID,
			"job_id", run.JobID,
		)
		e.handleSystemFailure(ctx, run, "managed execution not available: COMPUTE_RUNTIME not configured")
		e.recordManagedMetric(ctx, "system_failed", dispatchStart)
		return
	}

	// 2. Managed-specific billing enforcement (cloud only).
	// Daily + concurrent limits are already checked in executeInner (shared path).
	// Here we only check managed run cap (free tier) and spending limit (compute credits).
	if e.billingEnforcer != nil {
		orgID, orgErr := e.billingEnforcer.GetProjectOrgID(ctx, job.ProjectID)
		if orgErr != nil {
			e.logger.Warn("failed to resolve org for managed billing check",
				"run_id", run.ID, "error", orgErr, "fail_open", true)
		}
		if orgID != "" {
			if err := e.billingEnforcer.CheckManagedRunLimit(ctx, orgID); err != nil {
				e.logger.Warn("org managed run limit exceeded",
					"run_id", run.ID, "org_id", orgID, "error", err)
				e.handleSystemFailure(ctx, run, err.Error())
				e.recordManagedMetric(ctx, "org_limit_exceeded", dispatchStart)
				return
			}
			if err := e.billingEnforcer.CheckSpendingLimit(ctx, orgID); err != nil {
				e.logger.Warn("org spending limit exceeded",
					"run_id", run.ID, "org_id", orgID, "error", err)
				e.billingEnforcer.DecrManagedRunCount(ctx, orgID)
				e.handleSystemFailure(ctx, run, err.Error())
				e.recordManagedMetric(ctx, "org_limit_exceeded", dispatchStart)
				return
			}
			if err := e.billingEnforcer.CheckProjectBudgetLimit(ctx, job.ProjectID); err != nil {
				e.logger.Warn("project budget limit exceeded",
					"run_id", run.ID, "project_id", job.ProjectID, "org_id", orgID, "error", err)
				e.billingEnforcer.DecrManagedRunCount(ctx, orgID)
				e.handleSystemFailure(ctx, run, err.Error())
				e.recordManagedMetric(ctx, "org_limit_exceeded", dispatchStart)
				return
			}
		}
	}

	// 3. Semaphore: limit concurrent managed machines.
	if e.managedSemaphore != nil {
		if err := e.managedSemaphore.Acquire(ctx, 1); err != nil {
			// Context cancelled during acquire → backpressure snooze.
			retryAt := NextRetryAt(run.Attempt)
			e.snoozeRun(ctx, run, "managed semaphore full", &retryAt)
			e.recordManagedMetric(ctx, "infra_retry", dispatchStart)
			return
		}
		defer e.managedSemaphore.Release(1) // immediately after Acquire to prevent leak on panic
	}

	if e.metrics != nil {
		e.metrics.ManagedMachinesActive.Add(ctx, 1)
		defer e.metrics.ManagedMachinesActive.Add(ctx, -1)
	}

	// 4. Budget check.
	quota, quotaErr := e.store.GetProjectQuota(ctx, job.ProjectID)
	if quotaErr != nil {
		e.logger.Warn("failed to load project quota for budget check", "run_id", run.ID, "error", quotaErr)
		// Non-fatal: proceed without budget enforcement.
	}
	if quota != nil && quota.ComputeDailyCostLimitMicrousd > 0 {
		tz := quota.Timezone
		if tz == "" {
			tz = "UTC"
		}
		dailyCost, costErr := e.store.SumDailyComputeCost(ctx, job.ProjectID, tz)
		if costErr != nil {
			e.logger.Warn("failed to sum daily compute cost", "run_id", run.ID, "error", costErr)
		} else {
			estimated, _ := compute.EstimateCost(string(job.MachinePreset), job.TimeoutSecs)

			// Soft-limit warning at threshold percentage.
			threshold := quota.ComputeDailyCostLimitMicrousd * int64(domain.ComputeBudgetAlertThresholdPct) / 100
			if dailyCost+estimated > threshold && dailyCost < threshold {
				e.emitBudgetWarning(ctx, run, job, dailyCost, estimated, quota.ComputeDailyCostLimitMicrousd)
			}

			if dailyCost+estimated > quota.ComputeDailyCostLimitMicrousd {
				e.logger.Warn("compute budget exceeded",
					"run_id", run.ID,
					"project_id", job.ProjectID,
					"daily_cost", dailyCost,
					"estimated", estimated,
					"limit", quota.ComputeDailyCostLimitMicrousd,
				)
				e.handleSystemFailure(ctx, run, "compute budget exceeded")
				e.recordManagedMetric(ctx, "budget_exceeded", dispatchStart)
				return
			}
		}
	}

	// 5. Transition: dequeued → executing.
	err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
		"started_at": time.Now(),
	})
	if err != nil {
		e.logger.Error("failed to transition to executing", "run_id", run.ID, "error", err)
		return
	}
	run.Status = domain.StatusExecuting
	e.publishEvent(ctx, run, map[string]any{"from": "dequeued", "to": "executing"})

	// 6. Register heartbeat.
	e.heartbeat.Register(run.ID)
	defer e.heartbeat.Deregister(run.ID)

	// 7. Build environment variables.
	env := map[string]string{
		"STRAIT_RUN_ID":   run.ID,
		"STRAIT_JOB_SLUG": job.Slug,
		"STRAIT_ATTEMPT":  strconv.Itoa(run.Attempt),
		"STRAIT_API_URL":  e.externalAPIURL,
	}

	// JWT token for SDK callbacks.
	if e.jwtSigningKey != "" {
		expiresAt := time.Now().Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
		if run.ExpiresAt != nil {
			expiresAt = *run.ExpiresAt
		}
		claims := jwt.RegisteredClaims{
			Subject:   run.ID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		if signed, signErr := tok.SignedString([]byte(e.jwtSigningKey)); signErr == nil {
			env["STRAIT_SDK_TOKEN"] = signed
		}
	}

	// Payload: inline ≤64KB, fetch mode for larger.
	const maxInlinePayload = 64 * 1024
	if len(run.Payload) > 0 {
		if len(run.Payload) <= maxInlinePayload {
			env["STRAIT_PAYLOAD"] = string(run.Payload)
		} else {
			env["STRAIT_PAYLOAD_MODE"] = "fetch"
		}
	}

	// Secrets injection.
	secrets, secretsErr := e.store.ListJobSecretsByJob(ctx, job.ID, "production")
	if secretsErr != nil {
		e.logger.Warn("failed to load secrets for managed run", "run_id", run.ID, "error", secretsErr)
	}
	for _, secret := range secrets {
		key := "STRAIT_SECRET_" + strings.ToUpper(secret.SecretKey)
		env[key] = secret.EncryptedValue
	}

	// Checkpoint injection for retried runs.
	if run.Attempt > 1 {
		cp, cpErr := e.store.GetLatestCheckpoint(ctx, run.ID)
		if cpErr == nil && cp != nil {
			data, _ := json.Marshal(cp.State)
			if len(data) <= maxInlinePayload {
				env["STRAIT_LAST_CHECKPOINT"] = string(data)
				env["STRAIT_CHECKPOINT_AT"] = cp.CreatedAt.Format(time.RFC3339)
			}
		}
		if run.Error != "" {
			env["STRAIT_PREVIOUS_ERROR"] = run.Error
		}
	}

	// Inject memory limit for in-container resource monitoring.
	if presetInfo, pErr := compute.PresetFromName(string(job.MachinePreset)); pErr == nil {
		env["STRAIT_MEMORY_LIMIT_MB"] = strconv.Itoa(presetInfo.MemoryMB)
	}

	// 8. Resolve region: job config > project default > run metadata hint > executor default.
	region := job.Region
	if region == "" && quota != nil && quota.DefaultRegion != "" && compute.IsValidRegion(quota.DefaultRegion) {
		region = quota.DefaultRegion
	}
	if region == "" {
		if hint, ok := run.Metadata["_region_hint"]; ok && hint != "" {
			if validated := compute.NearestFlyRegion(hint); validated != "" {
				region = validated
			}
		}
	}
	if region == "" {
		region = e.defaultFlyRegion
	}

	// 9. Create the container (non-blocking).
	preset := string(job.MachinePreset)
	if override, ok := run.Metadata["_preset_override"]; ok && override != "" {
		if domain.MachinePreset(override).IsValid() {
			preset = override
		}
	}
	// Auto-upgrade from historical OOM data (only if no explicit override).
	if _, hasOverride := run.Metadata["_preset_override"]; !hasOverride {
		rec, recErr := e.store.GetPresetRecommendation(ctx, job.ID)
		if recErr == nil && rec != nil {
			recIdx := compute.PresetIndex(rec.RecommendedPreset)
			curIdx := compute.PresetIndex(preset)
			if recIdx > curIdx {
				e.logger.Info("auto-upgrading preset from OOM history",
					"run_id", run.ID, "job_id", job.ID,
					"from", preset, "to", rec.RecommendedPreset,
					"oom_count", rec.OOMCount)
				preset = rec.RecommendedPreset
			}
		}
	}
	runReq := compute.RunRequest{
		ImageURI:      job.ImageURI,
		MachinePreset: preset,
		Region:        region,
		Env:           env,
		Labels: map[string]string{
			"run_id":     run.ID,
			"job_id":     job.ID,
			"project_id": job.ProjectID,
		},
		TimeoutSecs: job.TimeoutSecs,
		Reusable:    e.machinePool != nil || job.ExecutionMode == domain.ExecutionModeManaged,
	}

	var machineID string
	var createErr error
	var dispatchSource string // "pool", "pause_reuse", or "cold_start"

	// Try warm pool first (acquire stopped machine and Start with new env).
	if e.machinePool != nil {
		if pooledID, ok := e.machinePool.Acquire(job.ImageURI, region); ok {
			env["STRAIT_CLEAN_START"] = "true"
			if startErr := e.containerRuntime.Start(ctx, pooledID, env); startErr != nil {
				e.logger.Warn("pooled machine start failed, falling back to create",
					"machine_id", pooledID, "run_id", run.ID, "error", startErr)
				delete(env, "STRAIT_CLEAN_START")
			} else {
				machineID = pooledID
				dispatchSource = "pool"
			}
		}
	}
	// Try reusing a paused machine (machine_id preserved from pause).
	if machineID == "" && run.MachineID != "" {
		env["STRAIT_CLEAN_START"] = "true"
		if startErr := e.containerRuntime.Start(ctx, run.MachineID, env); startErr != nil {
			e.logger.Warn("paused machine start failed, creating new",
				"machine_id", run.MachineID, "run_id", run.ID, "error", startErr)
			delete(env, "STRAIT_CLEAN_START")
		} else {
			machineID = run.MachineID
			dispatchSource = "pause_reuse"
		}
	}
	if machineID == "" {
		machineID, createErr = e.containerRuntime.Create(ctx, runReq)
		dispatchSource = "cold_start"

		// Multi-region failover: on 503, try preferred_regions first, then geo-proximate fallbacks.
		if createErr != nil && job.Region == "" {
			var re *compute.RuntimeError
			if errors.As(createErr, &re) && re.StatusCode == 503 {
				fallbacks := job.PreferredRegions
				if len(fallbacks) == 0 {
					fallbacks = compute.RegionFallbackChain(region)
				}
				for _, fbRegion := range fallbacks {
					e.logger.Info("attempting region failover",
						"run_id", run.ID, "from", region, "to", fbRegion)
					runReq.Region = fbRegion
					machineID, createErr = e.containerRuntime.Create(ctx, runReq)
					if createErr == nil {
						region = fbRegion // Update region for metadata tracking.
						break
					}
					var fbRE *compute.RuntimeError
					if !errors.As(createErr, &fbRE) || fbRE.StatusCode != 503 {
						break // Only fallback on 503.
					}
				}
			}
		}
	}
	if createErr != nil {
		// Fly-specific error classification for observability.
		var re *compute.RuntimeError
		if errors.As(createErr, &re) && re.StatusCode > 0 {
			retryable, fatal, backoffSecs := compute.ClassifyFlyError(re.StatusCode)
			e.logger.Warn("fly error classified",
				"run_id", run.ID, "status_code", re.StatusCode,
				"retryable", retryable, "fatal", fatal, "backoff_secs", backoffSecs,
			)
		}

		if compute.IsRetryable(createErr) {
			backoff := compute.BackoffHint(createErr)
			retryAt := time.Now().Add(backoff)
			e.snoozeRunFromExecuting(ctx, run, "infra retry: "+createErr.Error(), &retryAt)
			e.recordManagedMetric(ctx, "infra_retry", dispatchStart)
			return
		}
		e.handleSystemFailure(ctx, run, "container create error: "+createErr.Error())
		e.recordManagedMetric(ctx, "system_failed", dispatchStart)
		return
	}

	// Record dispatch source metrics.
	e.recordManagedMetric(ctx, dispatchSource, dispatchStart)
	e.logger.Info("managed dispatch resolved machine",
		"run_id", run.ID,
		"machine_id", machineID,
		"source", dispatchSource,
	)

	// Store machine_id on the run so cancellation can stop it.
	if machineID != "" {
		if setErr := e.store.SetRunMachineID(ctx, run.ID, machineID); setErr != nil {
			e.logger.Warn("failed to store machine_id on run", "run_id", run.ID, "error", setErr)
		}
		run.MachineID = machineID

		// Race check: if cancel arrived between Create and now, stop the machine.
		currentRun, readErr := e.store.GetRun(ctx, run.ID)
		if readErr == nil && currentRun.Status == domain.StatusCanceled {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer stopCancel()
			if stopErr := e.containerRuntime.Stop(stopCtx, machineID); stopErr != nil {
				e.logger.Warn("failed to stop canceled machine, destroying",
					"machine_id", machineID, "error", stopErr)
				_ = e.containerRuntime.Destroy(stopCtx, machineID)
			}
			e.recordManagedMetric(ctx, "canceled_race", dispatchStart)
			return
		}
	}

	// 10. Wait for container exit.
	result, runErr := e.containerRuntime.Wait(ctx, machineID, job.TimeoutSecs)

	if runErr != nil {
		// Stop the machine before snoozing — avoid orphaned running containers.
		if machineID != "" {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if stopErr := e.containerRuntime.Stop(stopCtx, machineID); stopErr != nil {
				_ = e.containerRuntime.Destroy(stopCtx, machineID)
			}
			stopCancel()
		}

		if compute.IsRetryable(runErr) {
			backoff := compute.BackoffHint(runErr)
			retryAt := time.Now().Add(backoff)
			e.snoozeRunFromExecuting(ctx, run, "infra retry: "+runErr.Error(), &retryAt)
			e.recordManagedMetric(ctx, "infra_retry", dispatchStart)
			return
		}
		e.handleSystemFailure(ctx, run, "container runtime error: "+runErr.Error())
		e.recordManagedMetric(ctx, "system_failed", dispatchStart)
		return
	}

	// Guard: Wait() returned nil result (shouldn't happen, but prevents nil-deref).
	if result == nil {
		e.handleSystemFailure(ctx, run, "container wait returned nil result")
		e.recordManagedMetric(ctx, "system_failed", dispatchStart)
		return
	}

	// 11. Record compute usage.
	e.recordComputeUsage(ctx, run, job, result)

	// 12. Re-read run status from DB (SDK race check).
	currentRun, readErr := e.store.GetRun(ctx, run.ID)
	if readErr != nil {
		e.logger.Error("failed to re-read run after container exit", "run_id", run.ID, "error", readErr)
		e.recordManagedMetric(ctx, "system_failed", dispatchStart)
		return
	}

	// If the SDK already moved the run to a terminal state, we're done.
	if currentRun.Status.IsTerminal() {
		e.logger.Info("managed run already terminal (SDK race)",
			"run_id", run.ID, "status", currentRun.Status, "exit_code", result.ExitCode)
		if e.machinePool != nil && result.ExitCode == 0 {
			e.machinePool.Release(job.ImageURI, region, machineID)
		}
		e.recordManagedMetric(ctx, "success", dispatchStart)
		return
	}

	// 13. Container exited: interpret exit code.
	if result.ExitCode == 0 {
		// Exit 0 but SDK didn't report complete → system failure.
		e.handleSystemFailure(ctx, run, "container exited 0 but SDK did not report completion")
		e.recordManagedMetric(ctx, "system_failed", dispatchStart)
		return
	}

	// Non-zero exit → classify signal, capture crash logs, and fail.
	classification := compute.ClassifyExitCode(result.ExitCode)

	// Belt-and-suspenders: fetch logs if Wait() didn't populate them.
	if result.Logs == "" && machineID != "" {
		logCtx, logCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if logs, logErr := e.containerRuntime.GetLogs(logCtx, machineID, 100); logErr == nil && logs != "" {
			result.Logs = logs
		}
		logCancel()
	}

	// Determine crash event type.
	eventType := domain.EventType("container_crash_log")
	if classification.IsOOM {
		eventType = domain.EventType("container_oom")
	}

	// Build enriched crash event data.
	crashData := map[string]any{
		"exit_code":   result.ExitCode,
		"error_class": classification.ErrorClass,
		"preset":      string(job.MachinePreset),
	}
	if presetInfo, pErr := compute.PresetFromName(string(job.MachinePreset)); pErr == nil {
		crashData["memory_mb"] = presetInfo.MemoryMB
	}
	if result.ExitSignal != "" {
		crashData["exit_signal"] = result.ExitSignal
	} else if classification.Signal != "" {
		crashData["exit_signal"] = classification.Signal
	}
	if result.OOMKilled || classification.IsOOM {
		crashData["oom_killed"] = true
	}
	if result.Logs != "" {
		crashData["logs"] = result.Logs
	}
	// Include last checkpoint time if available.
	if run.Attempt > 1 {
		cp, cpErr := e.store.GetLatestCheckpoint(ctx, run.ID)
		if cpErr == nil && cp != nil {
			crashData["last_checkpoint_at"] = cp.CreatedAt.Format(time.RFC3339)
		}
	}

	crashDataJSON, _ := json.Marshal(crashData)
	crashEvent := &domain.RunEvent{
		RunID:   run.ID,
		Type:    eventType,
		Level:   "error",
		Message: classification.HumanMessage,
		Data:    json.RawMessage(crashDataJSON),
	}
	if insertErr := e.store.InsertEvent(ctx, crashEvent); insertErr != nil {
		e.logger.Warn("failed to store crash event", "run_id", run.ID, "error", insertErr)
	}

	e.handleManagedFailure(ctx, run, job, classification)
	e.recordManagedMetric(ctx, "failure", dispatchStart)
}

// snoozeRunFromExecuting transitions a run back to queued from executing state (for managed infra retries).
func (e *Executor) snoozeRunFromExecuting(ctx context.Context, run *domain.JobRun, reason string, retryAt *time.Time) {
	snoozeCount := 0
	if run.Metadata != nil {
		if raw, ok := run.Metadata["snooze_count"]; ok {
			if parsed, err := strconv.Atoi(raw); err == nil {
				snoozeCount = parsed
			}
		}
	}
	snoozeCount++

	if e.maxSnoozeCount > 0 && snoozeCount > e.maxSnoozeCount {
		e.logger.Warn("max snooze count exceeded during managed dispatch",
			"run_id", run.ID, "snooze_count", snoozeCount)
		e.handleSystemFailure(ctx, run, fmt.Sprintf("max snooze count (%d) exceeded: %s", e.maxSnoozeCount, reason))
		return
	}

	fields := map[string]any{
		"error":         reason,
		"error_class":   "transient",
		"started_at":    nil,
		"finished_at":   nil,
		"next_retry_at": retryAt,
		"metadata":      map[string]string{"snooze_count": strconv.Itoa(snoozeCount)},
	}
	if err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, fields); err != nil {
		e.logger.Error("failed to snooze managed run", "run_id", run.ID, "error", err)
		return
	}

	e.emit(ctx, RunLifecycleEvent{
		Type: EventSnoozed, Run: run,
		FromStatus: domain.StatusExecuting, ToStatus: domain.StatusQueued,
		Attempt: run.Attempt,
	})
}

// handleManagedFailure handles a non-zero exit from a managed container.
// Retries per max_attempts policy; moves to dead_letter when exhausted.
func (e *Executor) handleManagedFailure(ctx context.Context, run *domain.JobRun, job *domain.Job, classification compute.ExitClassification) {
	currentPreset := string(job.MachinePreset)
	if override, ok := run.Metadata["_preset_override"]; ok && override != "" {
		currentPreset = override
	}

	// Record OOM event for historical learning.
	if classification.IsOOM {
		if err := e.store.RecordOOMEvent(ctx, job.ID, currentPreset); err != nil {
			e.logger.Warn("failed to record OOM event", "job_id", job.ID, "error", err)
		}
	}

	// OOM at max preset → dead_letter immediately.
	if classification.IsOOM && compute.IsMaxPreset(currentPreset) {
		memMB := compute.PresetMemoryMB(currentPreset)
		errMsg := fmt.Sprintf("OOM on largest preset %s (%dMB); cannot upgrade further", currentPreset, memMB)
		now := time.Now()
		run.Status = domain.StatusDeadLetter
		if err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusDeadLetter, map[string]any{
			"finished_at": now,
			"error":       errMsg,
			"error_class": classification.ErrorClass,
		}); err != nil {
			e.logger.Error("failed to mark managed run dead_letter", "run_id", run.ID, "error", err)
			return
		}
		e.emit(ctx, RunLifecycleEvent{
			Type: EventDeadLettered, Run: run, Job: job,
			FromStatus: domain.StatusExecuting, ToStatus: domain.StatusDeadLetter,
			Attempt: run.Attempt,
		})
		e.notifyWorkflowCallback(ctx, run)
		return
	}

	if run.Attempt < job.MaxAttempts {
		retryAt := NextRetryAt(run.Attempt)
		fields := map[string]any{
			"attempt":       run.Attempt + 1,
			"next_retry_at": retryAt,
			"error":         classification.HumanMessage,
			"error_class":   classification.ErrorClass,
			"started_at":    nil,
			"finished_at":   nil,
		}

		// OOM → upgrade to next preset for the retry.
		if classification.IsOOM {
			if nextPreset, ok := compute.NextPreset(currentPreset); ok {
				nextMemMB := compute.PresetMemoryMB(nextPreset)
				curMemMB := compute.PresetMemoryMB(currentPreset)
				fields["metadata"] = map[string]string{
					"_preset_override":   nextPreset,
					"_oom_upgraded_from": currentPreset,
				}
				fields["error"] = fmt.Sprintf("OOM on %s (%dMB), retrying on %s (%dMB)", currentPreset, curMemMB, nextPreset, nextMemMB)
			}
		}

		if err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, fields); err != nil {
			e.logger.Error("failed to re-enqueue managed run", "run_id", run.ID, "error", err)
			return
		}
		e.emit(ctx, RunLifecycleEvent{
			Type: EventRetried, Run: run, Job: job,
			FromStatus: domain.StatusExecuting, ToStatus: domain.StatusQueued,
			Attempt: run.Attempt + 1,
		})
		return
	}

	now := time.Now()
	run.Status = domain.StatusDeadLetter
	if err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusDeadLetter, map[string]any{
		"finished_at": now,
		"error":       classification.HumanMessage,
		"error_class": classification.ErrorClass,
	}); err != nil {
		e.logger.Error("failed to mark managed run dead_letter", "run_id", run.ID, "error", err)
		return
	}
	e.emit(ctx, RunLifecycleEvent{
		Type: EventDeadLettered, Run: run, Job: job,
		FromStatus: domain.StatusExecuting, ToStatus: domain.StatusDeadLetter,
		Attempt: run.Attempt,
	})
	e.notifyWorkflowCallback(ctx, run)
}

// recordComputeUsage records wall-clock time and cost for a managed run.
func (e *Executor) recordComputeUsage(ctx context.Context, run *domain.JobRun, job *domain.Job, result *compute.RunResult) {
	var durationSecs float64
	if result.StartedAt != nil && result.FinishedAt != nil {
		durationSecs = result.FinishedAt.Sub(*result.StartedAt).Seconds()
	}
	cost, _ := compute.CalculateCost(string(job.MachinePreset), durationSecs)

	usage := &domain.RunComputeUsage{
		RunID:         run.ID,
		ProjectID:     job.ProjectID,
		JobID:         job.ID,
		MachinePreset: string(job.MachinePreset),
		MachineID:     result.MachineID,
		DurationSecs:  durationSecs,
		CostMicrousd:  cost,
		StartedAt:     result.StartedAt,
		FinishedAt:    result.FinishedAt,
	}
	if err := e.store.CreateRunComputeUsage(ctx, usage); err != nil {
		e.logger.Warn("failed to record compute usage", "run_id", run.ID, "error", err)
	}
}

// emitBudgetWarning inserts a run event warning that compute budget is nearing the limit.
func (e *Executor) emitBudgetWarning(ctx context.Context, run *domain.JobRun, job *domain.Job, dailyCost, estimated, limit int64) {
	pct := float64(dailyCost+estimated) * 100 / float64(limit)
	data, _ := json.Marshal(map[string]any{
		"project_id":          job.ProjectID,
		"daily_cost_microusd": dailyCost,
		"estimated_microusd":  estimated,
		"limit_microusd":      limit,
		"percentage":          pct,
	})
	event := &domain.RunEvent{
		RunID:   run.ID,
		Type:    domain.EventType("budget_warning"),
		Level:   "warn",
		Message: fmt.Sprintf("compute budget at %.0f%% of daily limit", pct),
		Data:    json.RawMessage(data),
	}
	if err := e.store.InsertEvent(ctx, event); err != nil {
		e.logger.Warn("failed to insert budget warning event", "run_id", run.ID, "error", err)
	}
}

// recordManagedMetric increments managed dispatch counters.
func (e *Executor) recordManagedMetric(ctx context.Context, status string, start time.Time) {
	if e.metrics == nil {
		return
	}
	e.metrics.ManagedDispatchTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String("status", status)))
	e.metrics.ManagedDispatchDuration.Record(ctx, time.Since(start).Seconds())
}

func (e *Executor) tracedDispatch(ctx context.Context, job *domain.Job, run *domain.JobRun) (json.RawMessage, *domain.ExecutionTrace, error) {
	dispatchStart := time.Now()
	var connectStart time.Time
	var connectDone time.Time
	var gotFirstByte time.Time

	trace := &httptrace.ClientTrace{
		ConnectStart:         func(string, string) { connectStart = time.Now() },
		ConnectDone:          func(string, string, error) { connectDone = time.Now() },
		GotFirstResponseByte: func() { gotFirstByte = time.Now() },
	}

	tracedCtx := httptrace.WithClientTrace(ctx, trace)

	// Fetch secrets and checkpoint in parallel.
	var (
		secrets    []domain.JobSecret
		secretsErr error
		cp         *domain.RunCheckpoint
	)

	var dispatchWG conc.WaitGroup
	dispatchWG.Go(func() {
		secrets, secretsErr = e.store.ListJobSecretsByJob(tracedCtx, job.ID, "production")
	})
	if run.Attempt > 1 {
		dispatchWG.Go(func() {
			cp, _ = e.store.GetLatestCheckpoint(tracedCtx, run.ID)
		})
	}
	dispatchWG.Wait()

	if secretsErr != nil {
		return nil, nil, fmt.Errorf("failed to load secrets for job %s: %w", job.ID, secretsErr)
	}

	extraHeaders := make(map[string]string)
	for _, secret := range secrets {
		extraHeaders[fmt.Sprintf("X-Secret-%s", secret.SecretKey)] = secret.EncryptedValue
	}

	// Generate a JWT run token so the endpoint's SDK can call back to Strait.
	if e.jwtSigningKey != "" {
		expiresAt := time.Now().Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
		if run.ExpiresAt != nil {
			expiresAt = *run.ExpiresAt
		}
		claims := jwt.RegisteredClaims{
			Subject:   run.ID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		if signed, signErr := tok.SignedString([]byte(e.jwtSigningKey)); signErr == nil {
			extraHeaders["X-Run-Token"] = signed
		}
	}

	if run.Attempt > 1 {
		if cp != nil {
			data, _ := json.Marshal(cp.State)
			if len(data) <= 65536 {
				extraHeaders["X-Last-Checkpoint"] = string(data)
				extraHeaders["X-Checkpoint-At"] = cp.CreatedAt.Format(time.RFC3339)
			}
		}
		if run.Error != "" {
			extraHeaders["X-Previous-Error"] = run.Error
		}
	}

	result, err := e.dispatchToEndpoint(tracedCtx, job.EndpointURL, run, extraHeaders)
	gotLastByte := time.Now()

	execTrace := &domain.ExecutionTrace{}
	if !connectStart.IsZero() && !connectDone.IsZero() {
		execTrace.ConnectMs = durationMillisecondsAtLeastOne(connectDone.Sub(connectStart))
	}
	if !gotFirstByte.IsZero() {
		base := dispatchStart
		if !connectDone.IsZero() {
			base = connectDone
		}
		execTrace.TtfbMs = durationMillisecondsAtLeastOne(gotFirstByte.Sub(base))
	}
	if !gotFirstByte.IsZero() {
		execTrace.TransferMs = durationMillisecondsAtLeastOne(gotLastByte.Sub(gotFirstByte))
	}
	execTrace.DispatchMs = execTrace.ConnectMs + execTrace.TtfbMs + execTrace.TransferMs

	return result, execTrace, err
}

func (e *Executor) dispatch(ctx context.Context, job *domain.Job, run *domain.JobRun) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.Dispatch")
	defer span.End()
	start := time.Now()
	defer func() {
		if e.metrics != nil {
			e.metrics.DispatchDuration.Record(ctx, time.Since(start).Seconds())
		}
	}()

	extraHeaders := make(map[string]string)
	secrets, err := e.store.ListJobSecretsByJob(ctx, job.ID, "production")
	if err != nil {
		return fmt.Errorf("failed to load secrets for job %s: %w", job.ID, err)
	}
	for _, secret := range secrets {
		extraHeaders[fmt.Sprintf("X-Secret-%s", secret.SecretKey)] = secret.EncryptedValue
	}

	// Generate a JWT run token so the endpoint's SDK can call back to Strait.
	if e.jwtSigningKey != "" {
		expiresAt := time.Now().Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
		if run.ExpiresAt != nil {
			expiresAt = *run.ExpiresAt
		}
		claims := jwt.RegisteredClaims{
			Subject:   run.ID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		if signed, signErr := tok.SignedString([]byte(e.jwtSigningKey)); signErr == nil {
			extraHeaders["X-Run-Token"] = signed
		}
	}

	if run.Attempt > 1 {
		cp, cpErr := e.store.GetLatestCheckpoint(ctx, run.ID)
		if cpErr == nil && cp != nil {
			data, _ := json.Marshal(cp.State)
			if len(data) <= 65536 {
				extraHeaders["X-Last-Checkpoint"] = string(data)
				extraHeaders["X-Checkpoint-At"] = cp.CreatedAt.Format(time.RFC3339)
			}
		}
		if run.Error != "" {
			extraHeaders["X-Previous-Error"] = run.Error
		}
	}

	_, err = e.dispatchToEndpoint(ctx, job.EndpointURL, run, extraHeaders)
	return err
}

func (e *Executor) dispatchToEndpoint(ctx context.Context, endpointURL string, run *domain.JobRun, extraHeaders map[string]string) (json.RawMessage, error) {

	var body io.Reader
	if len(run.Payload) > 0 {
		body = bytes.NewReader(run.Payload)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Run-ID", run.ID)
	req.Header.Set("X-Job-ID", run.JobID)
	req.Header.Set("X-Attempt", fmt.Sprintf("%d", run.Attempt))

	// Inject W3C trace context headers from run metadata.
	if tp, ok := run.Metadata["_trace_parent"]; ok && tp != "" {
		req.Header.Set("Traceparent", tp)
		if ts, ok := run.Metadata["_trace_state"]; ok && ts != "" {
			req.Header.Set("Tracestate", ts)
		}
	}

	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		if e.metrics != nil {
			e.metrics.DispatchErrors.Add(ctx, 1)
		}
		return nil, fmt.Errorf("http dispatch: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &domain.EndpointError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	if len(respBody) > 0 {
		return json.RawMessage(respBody), nil
	}

	return nil, nil
}

func (e *Executor) snoozeRun(ctx context.Context, run *domain.JobRun, reason string, retryAt *time.Time) {
	snoozeCount := 0
	if run.Metadata != nil {
		if raw, ok := run.Metadata["snooze_count"]; ok {
			if parsed, err := strconv.Atoi(raw); err == nil {
				snoozeCount = parsed
			}
		}
	}
	snoozeCount++

	if e.maxSnoozeCount > 0 && snoozeCount > e.maxSnoozeCount {
		e.logger.Warn("max snooze count exceeded, marking system_failed",
			"run_id", run.ID, "job_id", run.JobID, "snooze_count", snoozeCount)
		e.handleSystemFailure(ctx, run, fmt.Sprintf("max snooze count (%d) exceeded: %s", e.maxSnoozeCount, reason))
		return
	}

	fields := map[string]any{
		"error":         reason,
		"error_class":   "transient",
		"started_at":    nil,
		"finished_at":   nil,
		"next_retry_at": retryAt,
		"metadata":      map[string]string{"snooze_count": strconv.Itoa(snoozeCount)},
	}
	if err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, fields); err != nil {
		e.logger.Error("failed to snooze run", "run_id", run.ID, "job_id", run.JobID, "error", err)
		return
	}

	e.emit(ctx, RunLifecycleEvent{
		Type: EventSnoozed, Run: run,
		FromStatus: domain.StatusDequeued, ToStatus: domain.StatusQueued,
		Attempt: run.Attempt,
	})
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
		return fallback, nil
	}

	wfRun, err := e.store.GetWorkflowRun(ctx, stepRun.WorkflowRunID)
	if err != nil || wfRun == nil {
		if err != nil {
			return fallback, err
		}
		return fallback, nil
	}

	steps, err := e.store.ListStepsByWorkflowVersion(ctx, wfRun.WorkflowID, wfRun.WorkflowVersion)
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
