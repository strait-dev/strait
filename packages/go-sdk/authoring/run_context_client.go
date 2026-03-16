package authoring

import (
	"context"
	"log/slog"
)

// RunContextClient is the interface for HTTP operations needed by RunContext.
type RunContextClient interface {
	CheckpointRun(ctx context.Context, runID string, body any) (map[string]any, error)
	HeartbeatRun(ctx context.Context, runID string) (map[string]any, error)
	ProgressRun(ctx context.Context, runID string, body any) (map[string]any, error)
	LogRun(ctx context.Context, runID string, body any) (map[string]any, error)
	UsageRun(ctx context.Context, runID string, body any) (map[string]any, error)
	ToolCallRun(ctx context.Context, runID string, body any) (map[string]any, error)
	OutputRun(ctx context.Context, runID string, body any) (map[string]any, error)
	WaitForEventRun(ctx context.Context, runID string, body any) (map[string]any, error)
	SpawnRun(ctx context.Context, runID string, body any) (map[string]any, error)
	ContinueRun(ctx context.Context, runID string, body any) (map[string]any, error)
	AnnotateRun(ctx context.Context, runID string, body any) (map[string]any, error)
	CompleteRun(ctx context.Context, runID string, body any) (map[string]any, error)
	FailRun(ctx context.Context, runID string, body any) (map[string]any, error)
	SetState(ctx context.Context, runID string, body any) (map[string]any, error)
	ListState(ctx context.Context, runID string) (map[string]any, error)
	GetState(ctx context.Context, runID string, key string) (map[string]any, error)
	DeleteState(ctx context.Context, runID string, key string) (map[string]any, error)
	StreamRun(ctx context.Context, runID string, body any) (map[string]any, error)
}

// RunContextOption configures RunContext creation.
type RunContextOption func(*runContextConfig)

type runContextConfig struct {
	attempt int
	ctx     context.Context
}

// WithAttempt sets the attempt number.
func WithAttempt(attempt int) RunContextOption {
	return func(c *runContextConfig) { c.attempt = attempt }
}

// WithContext sets the context.
func WithContext(ctx context.Context) RunContextOption {
	return func(c *runContextConfig) { c.ctx = ctx }
}

// CreateRunContext creates a RunContext wired to HTTP endpoints via the client.
func CreateRunContext(client RunContextClient, runID string, opts ...RunContextOption) RunContext {
	cfg := &runContextConfig{attempt: 1, ctx: context.Background()}
	for _, o := range opts {
		o(cfg)
	}

	logger := slog.Default().With("run_id", runID)

	return RunContext{
		RunID:   runID,
		Attempt: cfg.attempt,
		Ctx:     cfg.ctx,
		Logger:  logger,

		Checkpoint: func(state map[string]any) error {
			_, err := client.CheckpointRun(cfg.ctx, runID, map[string]any{"state": state, "source": "sdk"})
			return err
		},

		ReportProgress: func(percent float64, message ...string) error {
			body := map[string]any{"percent": percent}
			if len(message) > 0 {
				body["message"] = message[0]
			}
			_, err := client.ProgressRun(cfg.ctx, runID, body)
			return err
		},

		Heartbeat: func() error {
			_, err := client.HeartbeatRun(cfg.ctx, runID)
			return err
		},

		ReportUsage: func(usage UsageReport) error {
			body := map[string]any{"provider": usage.Provider, "model": usage.Model}
			if usage.PromptTokens != nil {
				body["prompt_tokens"] = *usage.PromptTokens
			}
			if usage.CompletionTokens != nil {
				body["completion_tokens"] = *usage.CompletionTokens
			}
			if usage.TotalTokens != nil {
				body["total_tokens"] = *usage.TotalTokens
			}
			if usage.CostMicrousd != nil {
				body["cost_microusd"] = *usage.CostMicrousd
			}
			_, err := client.UsageRun(cfg.ctx, runID, body)
			return err
		},

		LogToolCall: func(toolCall ToolCallReport) error {
			body := map[string]any{"tool_name": toolCall.ToolName}
			if toolCall.Input != nil {
				body["input"] = toolCall.Input
			}
			if toolCall.Output != nil {
				body["output"] = toolCall.Output
			}
			if toolCall.DurationMs != nil {
				body["duration_ms"] = *toolCall.DurationMs
			}
			if toolCall.Status != "" {
				body["status"] = toolCall.Status
			}
			_, err := client.ToolCallRun(cfg.ctx, runID, body)
			return err
		},

		SaveOutput: func(key string, value map[string]any, schema ...map[string]any) error {
			body := map[string]any{"key": key, "value": value}
			if len(schema) > 0 {
				body["schema"] = schema[0]
			}
			_, err := client.OutputRun(cfg.ctx, runID, body)
			return err
		},

		State: &RunContextState{
			Get: func(key string) (any, error) {
				result, err := client.GetState(cfg.ctx, runID, key)
				if err != nil {
					return nil, err
				}
				return result, nil
			},
			Set: func(key string, value any) error {
				_, err := client.SetState(cfg.ctx, runID, map[string]any{"key": key, "value": value})
				return err
			},
			Delete: func(key string) error {
				_, err := client.DeleteState(cfg.ctx, runID, key)
				return err
			},
			List: func() ([]map[string]any, error) {
				result, err := client.ListState(cfg.ctx, runID)
				if err != nil {
					return nil, err
				}
				if items, ok := result["data"].([]any); ok {
					var out []map[string]any
					for _, item := range items {
						if m, ok := item.(map[string]any); ok {
							out = append(out, m)
						}
					}
					return out, nil
				}
				return nil, nil
			},
		},

		StreamChunk: func(chunk string, opts ...StreamChunkOption) error {
			body := map[string]any{"chunk": chunk}
			if len(opts) > 0 {
				if opts[0].StreamID != "" {
					body["stream_id"] = opts[0].StreamID
				}
				if opts[0].Done {
					body["done"] = opts[0].Done
				}
			}
			_, err := client.StreamRun(cfg.ctx, runID, body)
			return err
		},

		WaitForEvent: func(eventKey string, opts ...WaitForEventOption) (map[string]any, error) {
			body := map[string]any{"event_key": eventKey}
			if len(opts) > 0 {
				if opts[0].TimeoutSecs != nil {
					body["timeout_secs"] = *opts[0].TimeoutSecs
				}
				if opts[0].NotifyUrl != "" {
					body["notify_url"] = opts[0].NotifyUrl
				}
			}
			return client.WaitForEventRun(cfg.ctx, runID, body)
		},

		Spawn: func(opts SpawnOptions) (map[string]any, error) {
			body := map[string]any{"job_slug": opts.JobSlug, "project_id": opts.ProjectID}
			if opts.Payload != nil {
				body["payload"] = opts.Payload
			}
			if opts.Priority != nil {
				body["priority"] = *opts.Priority
			}
			return client.SpawnRun(cfg.ctx, runID, body)
		},

		Continue: func(payload ...map[string]any) (map[string]any, error) {
			var body any
			if len(payload) > 0 {
				body = map[string]any{"payload": payload[0]}
			}
			return client.ContinueRun(cfg.ctx, runID, body)
		},

		Annotate: func(annotations map[string]string) error {
			_, err := client.AnnotateRun(cfg.ctx, runID, map[string]any{"annotations": annotations})
			return err
		},

		Complete: func(result ...map[string]any) error {
			var body any
			if len(result) > 0 {
				body = map[string]any{"result": result[0]}
			}
			_, err := client.CompleteRun(cfg.ctx, runID, body)
			return err
		},

		Fail: func(errMsg string) error {
			_, err := client.FailRun(cfg.ctx, runID, map[string]any{"error": errMsg})
			return err
		},
	}
}
