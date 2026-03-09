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

// dispatchSandbox executes a sandbox job via the Forge gRPC service.
// It streams execution events into the run events store and returns
// the final result.
func (e *Executor) dispatchSandbox(ctx context.Context, job *domain.Job, run *domain.JobRun) (json.RawMessage, *domain.ExecutionTrace, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "executor.DispatchSandbox")
	defer span.End()

	if e.sandboxClient == nil {
		return nil, nil, fmt.Errorf("sandbox client not configured")
	}

	dispatchStart := time.Now()

	req := &sandbox.ExecuteRequest{
		RunID:    run.ID,
		Language: job.SandboxLanguage,
		Code:     job.SandboxCode,
		Payload:  run.Payload,
		Timeout:  time.Duration(job.TimeoutSecs) * time.Second,
		MemoryMB: 256,
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
	var eventCount int

	err := e.sandboxClient.ExecuteStream(ctx, req, func(event *sandboxv1.ExecutionEvent) error {
		eventCount++

		if event.Log != nil {
			e.logger.Debug("sandbox log",
				"run_id", run.ID,
				"level", event.Log.Level,
				"message", event.Log.Message,
			)
			e.publishEvent(ctx, run, map[string]any{
				"type":    "sandbox_log",
				"level":   event.Log.Level,
				"message": event.Log.Message,
			})
		}

		if event.Checkpoint != nil {
			e.logger.Debug("sandbox checkpoint",
				"run_id", run.ID,
				"sequence", event.Checkpoint.Sequence,
			)
		}

		if event.ToolCall != nil {
			e.logger.Debug("sandbox tool call",
				"run_id", run.ID,
				"tool", event.ToolCall.ToolName,
				"status", event.ToolCall.Status,
				"duration_ms", event.ToolCall.DurationMs,
			)
		}

		if event.Result != nil {
			finalResult = event.Result
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

	execTrace.DispatchMs = finalResult.DurationMs
	if execTrace.DispatchMs == 0 {
		execTrace.DispatchMs = durationMillisecondsAtLeastOne(time.Since(dispatchStart))
	}

	if !finalResult.Success {
		return nil, execTrace, &domain.EndpointError{
			StatusCode: 500,
			Body:       finalResult.Error,
		}
	}

	if len(finalResult.Result) > 0 {
		return json.RawMessage(finalResult.Result), execTrace, nil
	}

	return nil, execTrace, nil
}
