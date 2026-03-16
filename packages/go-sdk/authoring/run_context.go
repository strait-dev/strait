// Package authoring provides the job and workflow definition DSL for the
// Strait Go SDK.
package authoring

import (
	"context"
	"log/slog"
)

// RunContextState provides KV state store operations for a run.
type RunContextState struct {
	Get    func(key string) (any, error)
	Set    func(key string, value any) error
	Delete func(key string) error
	List   func() ([]map[string]any, error)
}

// RunContext is the context object passed to a job's run handler.
type RunContext struct {
	// RunID is the unique identifier for this run.
	RunID string
	// Attempt is the current attempt number (1-based).
	Attempt int
	// Ctx is the context for the run, supporting cancellation.
	Ctx context.Context
	// Logger provides structured logging for the run.
	Logger *slog.Logger
	// Checkpoint saves intermediate state for crash recovery.
	Checkpoint func(state map[string]any) error
	// ReportProgress reports execution progress (0.0 to 1.0).
	ReportProgress func(percent float64, message ...string) error
	// Heartbeat signals that the run is still alive.
	Heartbeat func() error
	// ReportUsage reports LLM token usage and cost.
	ReportUsage func(usage UsageReport) error
	// LogToolCall logs a tool invocation.
	LogToolCall func(toolCall ToolCallReport) error
	// SaveOutput saves a named output value.
	SaveOutput func(key string, value map[string]any, schema ...map[string]any) error
	// State provides KV state store operations.
	State *RunContextState
	// StreamChunk sends a streaming chunk.
	StreamChunk func(chunk string, opts ...StreamChunkOption) error
	// WaitForEvent pauses until an external event is received.
	WaitForEvent func(eventKey string, opts ...WaitForEventOption) (map[string]any, error)
	// Spawn creates a child run.
	Spawn func(opts SpawnOptions) (map[string]any, error)
	// Continue continues the run with an optional payload.
	Continue func(payload ...map[string]any) (map[string]any, error)
	// Annotate adds annotations to the run.
	Annotate func(annotations map[string]string) error
	// Complete marks the run as complete.
	Complete func(result ...map[string]any) error
	// Fail marks the run as failed.
	Fail func(errMsg string) error
}

// UsageReport captures LLM token usage and cost.
type UsageReport struct {
	Provider         string
	Model            string
	PromptTokens     *int
	CompletionTokens *int
	TotalTokens      *int
	CostMicrousd     *int
}

// ToolCallReport captures a tool invocation.
type ToolCallReport struct {
	ToolName   string
	Input      map[string]any
	Output     map[string]any
	DurationMs *int
	Status     string
}

// SpawnOptions configures a child run.
type SpawnOptions struct {
	JobSlug   string
	ProjectID string
	Payload   map[string]any
	Priority  *int
}

// StreamChunkOption configures stream chunk options.
type StreamChunkOption struct {
	StreamID string
	Done     bool
}

// WaitForEventOption configures wait for event options.
type WaitForEventOption struct {
	TimeoutSecs *int
	NotifyUrl   string
}
