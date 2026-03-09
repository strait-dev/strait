package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"strait/internal/domain"
	"strait/internal/sandbox"
	sandboxv1 "strait/internal/sandbox/v1"

	"go.opentelemetry.io/otel"
)

// defaultSandboxMemoryMB is the default memory limit for sandbox executions.
const defaultSandboxMemoryMB = 256

var errSandboxNotConfigured = fmt.Errorf("sandbox client not configured")

// dispatchSandbox executes a sandbox job via the Forge gRPC service.
// It streams execution events into the run events store and returns
// the final result.
func (e *Executor) dispatchSandbox(ctx context.Context, job *domain.Job, run *domain.JobRun) (json.RawMessage, *domain.ExecutionTrace, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.DispatchSandbox")
	defer span.End()

	if e.sandboxClient == nil {
		return nil, nil, errSandboxNotConfigured
	}

	dispatchStart := time.Now()

	req := &sandbox.ExecuteRequest{
		RunID:    run.ID,
		Language: job.SandboxLanguage,
		Code:     job.SandboxCode,
		Payload:  run.Payload,
		Timeout:  time.Duration(job.TimeoutSecs) * time.Second,
		MemoryMB: defaultSandboxMemoryMB,
	}

	// Resolve environment variables if present
	if job.EnvironmentID != "" {
		envVars, err := e.store.GetResolvedEnvironmentVariables(ctx, job.EnvironmentID)
		if err != nil {
			e.logger.Warn("failed to resolve sandbox env vars",
				"run_id", run.ID,
				"environment_id", job.EnvironmentID,
				"error", err,
			)
		} else {
			req.Env = envVars
		}
	}

	// Stream events and collect result
	var finalResult *sandboxv1.ExecutionResult

	err := e.sandboxClient.ExecuteStream(ctx, req, func(event *sandboxv1.ExecutionEvent) error {
		if logEntry := event.GetLog(); logEntry != nil {
			e.logger.Debug("sandbox log",
				"run_id", run.ID,
				"level", logEntry.GetLevel(),
				"message", logEntry.GetMessage(),
			)
			e.publishEvent(ctx, run, map[string]any{
				"type":    "sandbox_log",
				"level":   logEntry.GetLevel(),
				"message": logEntry.GetMessage(),
			})
		}

		if cp := event.GetCheckpoint(); cp != nil {
			e.logger.Debug("sandbox checkpoint",
				"run_id", run.ID,
				"sequence", cp.GetSequence(),
			)
		}

		if tc := event.GetToolCall(); tc != nil {
			e.logger.Debug("sandbox tool call",
				"run_id", run.ID,
				"tool", tc.GetToolName(),
				"status", tc.GetStatus(),
				"duration_ms", tc.GetDurationMs(),
			)
		}

		if r := event.GetResult(); r != nil {
			finalResult = r
		}

		return nil
	})

	execTrace := &domain.ExecutionTrace{
		DispatchMs: durationMillisecondsAtLeastOne(time.Since(dispatchStart)),
	}

	if err != nil {
		return nil, execTrace, fmt.Errorf("sandbox execution: %w", err)
	}

	if finalResult == nil {
		return nil, execTrace, fmt.Errorf("sandbox execution completed without result")
	}

	execTrace.DispatchMs = finalResult.GetDurationMs()
	if execTrace.DispatchMs == 0 {
		execTrace.DispatchMs = durationMillisecondsAtLeastOne(time.Since(dispatchStart))
	}

	if !finalResult.GetSuccess() {
		return nil, execTrace, &domain.EndpointError{
			StatusCode: 500,
			Body:       finalResult.GetError(),
		}
	}

	if len(finalResult.GetResult()) > 0 {
		return json.RawMessage(finalResult.GetResult()), execTrace, nil
	}

	return nil, execTrace, nil
}
