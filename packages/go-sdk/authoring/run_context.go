// Package authoring provides the job and workflow definition DSL for the
// Strait Go SDK.
package authoring

import (
	"context"
	"log/slog"
)

// RunContext is the context object passed to a job's run handler.
//
// The SDK defines this interface but does not provide a default implementation.
// Your executor/worker must supply concrete implementations when calling the
// run handler.
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
	ReportProgress func(percent float64) error
	// Heartbeat signals that the run is still alive.
	Heartbeat func() error
}
