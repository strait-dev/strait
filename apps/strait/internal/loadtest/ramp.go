//go:build loadtest

package loadtest

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sourcegraph/conc"
)

// RampMode determines how the ramp engine increases load.
type RampMode int

const (
	// RampThroughput increases the rate of operations per second.
	RampThroughput RampMode = iota
	// RampConcurrency increases the number of concurrent operations.
	RampConcurrency
)

// StopCondition defines when a ramp test should stop.
type StopCondition struct {
	MaxQueueDepth int64         // Stop if queue depth exceeds this
	MaxLatencyP99 time.Duration // Stop if P99 latency exceeds this
	MaxErrorRate  float64       // Stop if error rate exceeds this (0-1)
	MaxDuration   time.Duration // Maximum total duration
}

// RampConfig configures the ramp engine.
type RampConfig struct {
	Mode          RampMode
	StartRate     int           // Starting jobs/sec (throughput) or concurrent count
	StepSize      int           // Increase per step
	StepInterval  time.Duration // Time between steps
	StopCondition StopCondition
}

// RampResult captures the outcome of a ramp test.
type RampResult struct {
	Mode            RampMode            `json:"mode"`
	MaxRate         int                 `json:"max_rate"`
	BreakingRate    int                 `json:"breaking_rate"`
	Bottleneck      string              `json:"bottleneck"`
	Duration        time.Duration       `json:"duration"`
	TotalOperations int64               `json:"total_operations"`
	TotalErrors     int64               `json:"total_errors"`
	Steps           []RampStepResult    `json:"steps"`
}

// RampStepResult captures metrics for a single ramp step.
type RampStepResult struct {
	Rate           int           `json:"rate"`
	Duration       time.Duration `json:"duration"`
	Operations     int64         `json:"operations"`
	Errors         int64         `json:"errors"`
	ErrorRate      float64       `json:"error_rate"`
	LatencyP50     time.Duration `json:"latency_p50"`
	LatencyP95     time.Duration `json:"latency_p95"`
	LatencyP99     time.Duration `json:"latency_p99"`
	QueueDepth     int64         `json:"queue_depth"`
	StoppedEarly   bool          `json:"stopped_early,omitempty"`
	StopReason     string        `json:"stop_reason,omitempty"`
}

// RampEngine executes load ramp tests.
type RampEngine struct {
	config    RampConfig
	operation func(ctx context.Context) error // The operation to execute
	queueDepthFn func() int64                 // Returns current queue depth
}

// NewRampEngine creates a ramp engine with the given configuration.
func NewRampEngine(cfg RampConfig, operation func(ctx context.Context) error) *RampEngine {
	return &RampEngine{
		config:    cfg,
		operation: operation,
	}
}

// SetQueueDepthFn sets the function that returns current queue depth.
func (re *RampEngine) SetQueueDepthFn(fn func() int64) {
	re.queueDepthFn = fn
}

// Run executes the ramp test and returns results.
func (re *RampEngine) Run(ctx context.Context) (*RampResult, error) {
	result := &RampResult{
		Mode: re.config.Mode,
	}

	start := time.Now()
	currentRate := re.config.StartRate
	maxSustained := 0

	for {
		if re.config.StopCondition.MaxDuration > 0 && time.Since(start) > re.config.StopCondition.MaxDuration {
			result.Bottleneck = "max_duration"
			break
		}

		if ctx.Err() != nil {
			result.Bottleneck = "cancelled"
			break
		}

		stepResult := re.runStep(ctx, currentRate)
		result.Steps = append(result.Steps, stepResult)
		result.TotalOperations += stepResult.Operations
		result.TotalErrors += stepResult.Errors

		// Check stop conditions
		if stopReason := re.checkStopConditions(stepResult); stopReason != "" {
			stepResult.StoppedEarly = true
			stepResult.StopReason = stopReason
			result.BreakingRate = currentRate
			result.Bottleneck = stopReason
			break
		}

		maxSustained = currentRate
		currentRate += re.config.StepSize
	}

	result.MaxRate = maxSustained
	result.Duration = time.Since(start)

	if result.BreakingRate == 0 {
		result.BreakingRate = currentRate
	}

	return result, nil
}

func (re *RampEngine) runStep(ctx context.Context, rate int) RampStepResult {
	stepCtx, cancel := context.WithTimeout(ctx, re.config.StepInterval)
	defer cancel()

	var (
		ops    atomic.Int64
		errs   atomic.Int64
	)

	// Track latencies
	latencies := newLatencyTracker()

	// Use a WaitGroup to track all goroutines spawned during the step
	var wg conc.WaitGroup

	switch re.config.Mode {
	case RampThroughput:
		re.runThroughputStep(stepCtx, rate, &ops, &errs, latencies, &wg)
	case RampConcurrency:
		re.runConcurrencyStep(stepCtx, rate, &ops, &errs, latencies, &wg)
	}

	// Wait for step duration to complete
	<-stepCtx.Done()
	// Wait for in-flight operations to finish (with a brief timeout)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}

	totalOps := ops.Load()
	totalErrs := errs.Load()
	totalAttempts := totalOps + totalErrs
	var errorRate float64
	if totalAttempts > 0 {
		errorRate = float64(totalErrs) / float64(totalAttempts)
	}

	var queueDepth int64
	if re.queueDepthFn != nil {
		queueDepth = re.queueDepthFn()
	}

	return RampStepResult{
		Rate:       rate,
		Duration:   re.config.StepInterval,
		Operations: totalOps,
		Errors:     totalErrs,
		ErrorRate:  errorRate,
		LatencyP50: latencies.percentile(50),
		LatencyP95: latencies.percentile(95),
		LatencyP99: latencies.percentile(99),
		QueueDepth: queueDepth,
	}
}

func (re *RampEngine) runThroughputStep(
	ctx context.Context,
	rate int,
	ops, errs *atomic.Int64,
	latencies *latencyTracker,
	wg *conc.WaitGroup,
) {
	// Send 'rate' operations per second for the step duration.
	// Each operation gets its own context so step boundary cancellation
	// does not count in-flight requests as errors.
	ticker := time.NewTicker(time.Second / time.Duration(max(rate, 1)))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			wg.Go(func() {
				opCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				start := time.Now()
				if err := re.operation(opCtx); err != nil {
					errs.Add(1)
				} else {
					ops.Add(1)
				}
				latencies.record(time.Since(start))
			})
		}
	}
}

func (re *RampEngine) runConcurrencyStep(
	ctx context.Context,
	concurrent int,
	ops, errs *atomic.Int64,
	latencies *latencyTracker,
	wg *conc.WaitGroup,
) {
	// Run 'concurrent' workers in parallel for the step duration.
	// Each operation gets its own context so step boundary cancellation
	// does not count in-flight requests as errors.
	for range concurrent {
		wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					opCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					start := time.Now()
					if err := re.operation(opCtx); err != nil {
						errs.Add(1)
					} else {
						ops.Add(1)
					}
					latencies.record(time.Since(start))
					cancel()
				}
			}
		})
	}

	// Wait for step duration (context will cancel)
	<-ctx.Done()
}

func (re *RampEngine) checkStopConditions(step RampStepResult) string {
	sc := re.config.StopCondition

	if sc.MaxQueueDepth > 0 && step.QueueDepth > sc.MaxQueueDepth {
		return fmt.Sprintf("queue_depth_%d", step.QueueDepth)
	}
	if sc.MaxLatencyP99 > 0 && step.LatencyP99 > sc.MaxLatencyP99 {
		return fmt.Sprintf("latency_p99_%s", step.LatencyP99)
	}
	if sc.MaxErrorRate > 0 && step.ErrorRate > sc.MaxErrorRate {
		return fmt.Sprintf("error_rate_%.2f", step.ErrorRate)
	}
	return ""
}

const reservoirSize = 10000

// latencyTracker collects latency measurements using reservoir sampling
// to bound memory usage while maintaining statistical accuracy.
type latencyTracker struct {
	mu      sync.Mutex
	samples []time.Duration
	count   int64
}

func newLatencyTracker() *latencyTracker {
	return &latencyTracker{
		samples: make([]time.Duration, 0, reservoirSize),
	}
}

func (lt *latencyTracker) record(d time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.count++
	if len(lt.samples) < reservoirSize {
		lt.samples = append(lt.samples, d)
	} else {
		// Reservoir sampling: replace a random element with probability reservoirSize/count
		j := rand.Int64N(lt.count)
		if j < reservoirSize {
			lt.samples[j] = d
		}
	}
}

func (lt *latencyTracker) percentile(p float64) time.Duration {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	n := len(lt.samples)
	if n == 0 {
		return 0
	}

	// Copy and sort
	sorted := make([]time.Duration, n)
	copy(sorted, lt.samples)
	slices.Sort(sorted)

	idx := int(math.Ceil(p/100*float64(n))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}
