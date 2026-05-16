//go:build loadtest

package loadtest

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sourcegraph/conc"
)

// EnduranceConfig configures the endurance test runner.
type EnduranceConfig struct {
	TargetRate      int           // Steady-state jobs/sec
	Duration        time.Duration // Total test duration
	SpikeInterval   time.Duration // How often to inject 10x spikes
	SpikeMultiple   int           // Spike multiplier (default 10)
	SpikeDuration   time.Duration // How long each spike lasts
	LongRunJobs     int           // Number of long-running jobs to maintain
	LongRunMinutes  int           // Duration of each long-run job in minutes
	AlertThresholds AlertThresholds
}

// AlertThresholds define automatic leak/degradation detection.
type AlertThresholds struct {
	MemoryGrowthPerHourMB  float64 // MB growth per hour before alerting
	GoroutineGrowthPerHour float64 // Goroutine count growth per hour
	P99GrowthPerHourPct    float64 // P99 latency percent growth per hour
	ErrorGrowthPerHourPct  float64 // Error rate percent growth per hour
}

// EnduranceResult captures endurance test outcomes.
type EnduranceResult struct {
	RampResult
	SpikesInjected   int `json:"spikes_injected"`
	LongRunTotal     int `json:"long_run_total"`
	LongRunCompleted int `json:"long_run_completed"`
	LongRunFailed    int `json:"long_run_failed"`
}

const loadGeneratorMaxInFlight = 1024

// Alert represents a detected issue during endurance testing.
type Alert struct {
	Severity string    `json:"severity"` // LEAK, DEGRADATION, WARNING
	Message  string    `json:"message"`
	Hour     int       `json:"hour"`
	Time     time.Time `json:"time"`
}

// EnduranceRunner executes endurance tests with spike injection and monitoring.
type EnduranceRunner struct {
	config EnduranceConfig
}

// NewEnduranceRunner creates an endurance test runner.
func NewEnduranceRunner(cfg EnduranceConfig) *EnduranceRunner {
	if cfg.SpikeMultiple == 0 {
		cfg.SpikeMultiple = 10
	}
	if cfg.SpikeInterval == 0 {
		cfg.SpikeInterval = 4 * time.Hour
	}
	if cfg.SpikeDuration == 0 {
		cfg.SpikeDuration = 5 * time.Minute
	}
	return &EnduranceRunner{config: cfg}
}

// Run executes the endurance test. Returns results, alerts, and any fatal error.
func (er *EnduranceRunner) Run(ctx context.Context, h *Harness) (*EnduranceResult, []Alert, error) {
	ctx, cancel := context.WithTimeout(ctx, er.config.Duration)
	defer cancel()

	result := &EnduranceResult{
		LongRunTotal: er.config.LongRunJobs,
	}
	var alerts []Alert
	var alertsMu sync.Mutex

	addAlert := func(a Alert) {
		alertsMu.Lock()
		alerts = append(alerts, a)
		alertsMu.Unlock()
	}

	// Track operations
	var ops, errs atomic.Int64
	var spikeActive atomic.Bool
	fastEchoJobID := h.ResolveJobID("loadtest-fast-echo")
	slowProcessJobID := h.ResolveJobID("loadtest-slow-process")
	var triggerWG conc.WaitGroup
	triggerSlots := make(chan struct{}, boundedLoadGeneratorCapacity(er.config.TargetRate*er.config.SpikeMultiple))

	// Baseline load goroutine
	var wg conc.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			rate := er.config.TargetRate
			if spikeActive.Load() {
				rate *= er.config.SpikeMultiple
			}

			interval := time.Second / time.Duration(max(rate, 1))
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}

			startTrackedLoadtestTrigger(ctx, &triggerWG, triggerSlots, func(ctx context.Context) error {
				return h.TriggerJob(ctx, "loadtest-project", fastEchoJobID, map[string]any{
					"timestamp": time.Now().UnixMilli(),
				})
			}, func() {
				ops.Add(1)
			}, func() {
				errs.Add(1)
			})
		}
	})

	// Spike injection goroutine
	if er.config.SpikeInterval > 0 {
		wg.Go(func() {
			ticker := time.NewTicker(er.config.SpikeInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					spikeActive.Store(true)
					result.SpikesInjected++
					if !sleepWithContext(ctx, er.config.SpikeDuration) {
						spikeActive.Store(false)
						return
					}
					spikeActive.Store(false)
					// Allow 10 min recovery
					if !sleepWithContext(ctx, 10*time.Minute) {
						return
					}
				}
			}
		})
	}

	// Long-running job goroutines
	var longRunCompleted, longRunFailed atomic.Int32
	for range er.config.LongRunJobs {
		wg.Go(func() {
			err := h.TriggerJob(ctx, "loadtest-project", slowProcessJobID, map[string]any{
				"work_duration": er.config.LongRunMinutes * 60,
				"timestamp":     time.Now().UnixMilli(),
			})
			if err != nil {
				longRunFailed.Add(1)
			} else {
				longRunCompleted.Add(1)
			}
		})
	}

	// Hourly alert check goroutine
	wg.Go(func() {
		hourTicker := time.NewTicker(1 * time.Hour)
		defer hourTicker.Stop()

		hour := 0
		var prevSnapshot *MetricsSnapshot
		for {
			select {
			case <-ctx.Done():
				return
			case <-hourTicker.C:
				hour++
				snapshots := h.Metrics.Snapshots()
				if len(snapshots) == 0 {
					continue
				}
				current := snapshots[len(snapshots)-1]

				if prevSnapshot != nil {
					newAlerts := er.checkThresholds(hour, prevSnapshot, &current)
					for _, a := range newAlerts {
						addAlert(a)
					}
				}
				prevSnapshot = &current
			}
		}
	})

	// Wait for completion
	wg.Wait()
	triggerWG.Wait()

	result.TotalOperations = ops.Load()
	result.TotalErrors = errs.Load()
	result.Duration = er.config.Duration
	result.LongRunCompleted = int(longRunCompleted.Load())
	result.LongRunFailed = int(longRunFailed.Load())

	return result, alerts, nil
}

func (er *EnduranceRunner) checkThresholds(hour int, prev, curr *MetricsSnapshot) []Alert {
	var alerts []Alert
	th := er.config.AlertThresholds

	// Memory growth
	memDeltaMB := float64(curr.Go.HeapAlloc-prev.Go.HeapAlloc) / (1024 * 1024)
	if th.MemoryGrowthPerHourMB > 0 && memDeltaMB > th.MemoryGrowthPerHourMB {
		alerts = append(alerts, Alert{
			Severity: "LEAK",
			Message:  fmt.Sprintf("memory grew %.0fMB in hour %d (threshold: %.0fMB)", memDeltaMB, hour, th.MemoryGrowthPerHourMB),
			Hour:     hour,
			Time:     time.Now(),
		})
	}

	// Goroutine growth
	goroutineDelta := float64(curr.Go.Goroutines - prev.Go.Goroutines)
	if th.GoroutineGrowthPerHour > 0 && goroutineDelta > th.GoroutineGrowthPerHour {
		alerts = append(alerts, Alert{
			Severity: "LEAK",
			Message:  fmt.Sprintf("goroutines grew by %.0f in hour %d (threshold: %.0f)", goroutineDelta, hour, th.GoroutineGrowthPerHour),
			Hour:     hour,
			Time:     time.Now(),
		})
	}

	// P99 latency growth (using Postgres wait duration as a proxy for request latency).
	// When DB wait times grow, end-to-end P99 latency degrades proportionally.
	if th.P99GrowthPerHourPct > 0 && prev.Postgres != nil && curr.Postgres != nil {
		prevWait := float64(prev.Postgres.WaitDurationMs)
		currWait := float64(curr.Postgres.WaitDurationMs)
		if prevWait > 0 {
			growthPct := ((currWait - prevWait) / prevWait) * 100
			if growthPct > th.P99GrowthPerHourPct {
				alerts = append(alerts, Alert{
					Severity: "DEGRADATION",
					Message:  fmt.Sprintf("P99 latency proxy (DB wait) grew %.1f%% in hour %d (threshold: %.1f%%)", growthPct, hour, th.P99GrowthPerHourPct),
					Hour:     hour,
					Time:     time.Now(),
				})
			}
		}
	}

	// Error rate growth
	if th.ErrorGrowthPerHourPct > 0 && prev.App != nil && curr.App != nil {
		prevRate := prev.App.ErrorRate
		currRate := curr.App.ErrorRate
		if prevRate > 0 {
			growthPct := ((currRate - prevRate) / prevRate) * 100
			if growthPct > th.ErrorGrowthPerHourPct {
				alerts = append(alerts, Alert{
					Severity: "DEGRADATION",
					Message:  fmt.Sprintf("error rate grew %.1f%% in hour %d (%.2f -> %.2f/sec, threshold: %.1f%%)", growthPct, hour, prevRate, currRate, th.ErrorGrowthPerHourPct),
					Hour:     hour,
					Time:     time.Now(),
				})
			}
		} else if currRate > 0 {
			// Errors appeared where there were none before
			alerts = append(alerts, Alert{
				Severity: "WARNING",
				Message:  fmt.Sprintf("error rate appeared in hour %d (0 -> %.2f/sec)", hour, currRate),
				Hour:     hour,
				Time:     time.Now(),
			})
		}
	}

	return alerts
}

// ErrorInjector injects error scenario jobs at a configurable rate during load tests.
type ErrorInjector struct {
	harness   *Harness
	projectID string
	perMinute int
	injected  atomic.Int64
}

// NewErrorInjector creates an error injector that triggers error scenario jobs.
func NewErrorInjector(h *Harness, projectID string, perMinute int) *ErrorInjector {
	return &ErrorInjector{
		harness:   h,
		projectID: projectID,
		perMinute: perMinute,
	}
}

// Run starts injecting error scenarios until the context is cancelled.
func (ei *ErrorInjector) Run(ctx context.Context) {
	scenarios := []string{
		"clean_exit", "exit_code_1", "oom", "infinite_loop",
		"slow_death", "panic_after_checkpoint", "segfault",
		"fork_bomb", "disk_fill", "sdk_timeout",
		"exit_code_137", "network_abuse",
	}

	interval := time.Minute / time.Duration(max(ei.perMinute, 1))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	errorJobID := ei.harness.ResolveJobID("loadtest-errors")
	var triggerWG conc.WaitGroup
	triggerSlots := make(chan struct{}, boundedLoadGeneratorCapacity(ei.perMinute))
	defer triggerWG.Wait()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scenario := scenarios[rand.IntN(len(scenarios))] //nolint:gosec // non-cryptographic use for load test randomization
			startTrackedLoadtestTrigger(ctx, &triggerWG, triggerSlots, func(ctx context.Context) error {
				return ei.harness.TriggerJob(ctx, ei.projectID, errorJobID, map[string]any{
					"scenario": scenario,
				})
			}, func() {
				ei.injected.Add(1)
			}, nil)
		}
	}
}

// Injected returns the total number of error scenarios injected.
func (ei *ErrorInjector) Injected() int64 {
	return ei.injected.Load()
}

func boundedLoadGeneratorCapacity(rate int) int {
	if rate <= 0 {
		return 1
	}
	return min(rate, loadGeneratorMaxInFlight)
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func startTrackedLoadtestTrigger(
	ctx context.Context,
	wg *conc.WaitGroup,
	slots chan struct{},
	trigger func(context.Context) error,
	onSuccess func(),
	onError func(),
) bool {
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return false
	}
	wg.Go(func() {
		defer func() { <-slots }()
		if err := trigger(ctx); err != nil {
			if onError != nil {
				onError()
			}
			return
		}
		if onSuccess != nil {
			onSuccess()
		}
	})
	return true
}
