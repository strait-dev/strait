package composition

import (
	"context"
	"fmt"
	"math"
	"time"

	strait "github.com/strait-dev/go-sdk"
)

// WaitForRunOptions configures polling behavior for WaitForRun.
type WaitForRunOptions struct {
	// TimeoutMs is the maximum wait time before timing out. Default 60000.
	TimeoutMs int
	// InitialDelayMs is the initial polling delay. Default 500.
	InitialDelayMs int
	// MaxDelayMs is the maximum polling delay. Default 10000.
	MaxDelayMs int
	// Factor is the exponential backoff multiplier. Default 1.5.
	Factor float64
	// IsTerminal overrides the default terminal status check.
	IsTerminal func(status string) bool
}

var defaultTerminalStatuses = map[string]bool{
	"completed":     true,
	"failed":        true,
	"timed_out":     true,
	"crashed":       true,
	"system_failed": true,
	"canceled":      true,
	"expired":       true,
	"dead_letter":   true,
}

// WaitForRun polls getRun until the run reaches a terminal status or times out.
func WaitForRun[T any](
	ctx context.Context,
	getRun func(ctx context.Context, runID string) (T, error),
	getStatus func(T) string,
	runID string,
	opts *WaitForRunOptions,
) (T, error) {
	var o WaitForRunOptions
	if opts != nil {
		o = *opts
	}
	if o.TimeoutMs <= 0 {
		o.TimeoutMs = 60000
	}
	if o.InitialDelayMs <= 0 {
		o.InitialDelayMs = 500
	}
	if o.MaxDelayMs <= 0 {
		o.MaxDelayMs = 10000
	}
	if o.Factor <= 0 {
		o.Factor = 1.5
	}

	var zero T
	delayMs := o.InitialDelayMs
	startedAt := time.Now()

	for {
		if err := ctx.Err(); err != nil {
			return zero, err
		}

		run, err := getRun(ctx, runID)
		if err != nil {
			return zero, err
		}

		status := getStatus(run)
		isTerminal := false
		if o.IsTerminal != nil {
			isTerminal = o.IsTerminal(status)
		} else {
			isTerminal = defaultTerminalStatuses[status]
		}

		if isTerminal {
			return run, nil
		}

		elapsed := time.Since(startedAt).Milliseconds()
		if elapsed > int64(o.TimeoutMs) {
			return zero, &strait.TimeoutError{
				Message:   fmt.Sprintf("waitForRun timed out after %dms for run %s", o.TimeoutMs, runID),
				RunID:     runID,
				ElapsedMs: elapsed,
			}
		}

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(time.Duration(delayMs) * time.Millisecond):
		}

		delayMs = int(math.Min(float64(o.MaxDelayMs), math.Max(1, math.Round(float64(delayMs)*o.Factor))))
	}
}
