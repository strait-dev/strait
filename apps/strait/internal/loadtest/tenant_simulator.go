//go:build loadtest

package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sourcegraph/conc"
)

// TenantPlan represents the plan tier a tenant is on.
type TenantPlan string

const (
	PlanStarter TenantPlan = "starter"
	PlanPro     TenantPlan = "pro"
)

// TenantProfile defines the traffic characteristics of a simulated tenant.
type TenantProfile struct {
	ID              string     `json:"id"`
	Plan            TenantPlan `json:"plan"`
	RunsPerMinute   float64    `json:"runs_per_minute"`   // Base rate
	BurstProb       float64    `json:"burst_probability"` // Chance of 10x burst per minute
	HTTPPercent     float64    `json:"http_percent"`      // Fraction of HTTP-mode jobs
	ManagedPercent  float64    `json:"managed_percent"`   // Fraction of managed (Docker) jobs
	WorkflowPercent float64    `json:"workflow_percent"`  // Fraction of workflow triggers
}

// TenantSimulatorConfig configures the multi-tenant simulator.
type TenantSimulatorConfig struct {
	Tenants        []TenantProfile
	TimeOfDayCurve bool          // Apply realistic time-of-day curve
	Duration       time.Duration // Total simulation duration
	PeakHourUTC    int           // Hour of peak traffic (default: 14)
	TroughHourUTC  int           // Hour of minimum traffic (default: 4)
}

// TenantSimulator generates realistic multi-tenant traffic.
type TenantSimulator struct {
	config    TenantSimulatorConfig
	triggerFn func(ctx context.Context, tenant TenantProfile, jobType string) error

	totalRuns atomic.Int64
	totalErrs atomic.Int64
	perTenant sync.Map // tenant ID -> *tenantStats
}

type tenantStats struct {
	runs   atomic.Int64
	errors atomic.Int64
}

// TenantSimulatorResult captures the outcome of a simulation.
type TenantSimulatorResult struct {
	Duration      time.Duration             `json:"duration"`
	TotalRuns     int64                     `json:"total_runs"`
	TotalErrors   int64                     `json:"total_errors"`
	RunsPerSecond float64                   `json:"runs_per_second"`
	PerTenant     map[string]TenantRunStats `json:"per_tenant"`
}

// TenantRunStats captures per-tenant statistics.
type TenantRunStats struct {
	Runs   int64   `json:"runs"`
	Errors int64   `json:"errors"`
	Rate   float64 `json:"runs_per_second"`
}

// NewTenantSimulator creates a tenant simulator.
func NewTenantSimulator(
	cfg TenantSimulatorConfig,
	triggerFn func(ctx context.Context, tenant TenantProfile, jobType string) error,
) *TenantSimulator {
	if cfg.PeakHourUTC == 0 {
		cfg.PeakHourUTC = 14
	}
	if cfg.TroughHourUTC == 0 {
		cfg.TroughHourUTC = 4
	}
	return &TenantSimulator{
		config:    cfg,
		triggerFn: triggerFn,
	}
}

// Run executes the multi-tenant simulation.
func (ts *TenantSimulator) Run(ctx context.Context) (*TenantSimulatorResult, error) {
	ctx, cancel := context.WithTimeout(ctx, ts.config.Duration)
	defer cancel()

	var wg conc.WaitGroup

	for _, tenant := range ts.config.Tenants {
		wg.Go(func() {
			ts.simulateTenant(ctx, tenant)
		})
	}

	wg.Wait()

	return ts.buildResult(), nil
}

// CurrentRate returns the aggregate runs/sec across all tenants right now.
func (ts *TenantSimulator) CurrentRate() float64 {
	total := ts.totalRuns.Load()
	return float64(total)
}

// TotalRuns returns the total number of triggered runs.
func (ts *TenantSimulator) TotalRuns() int64 {
	return ts.totalRuns.Load()
}

func (ts *TenantSimulator) simulateTenant(ctx context.Context, tenant TenantProfile) {
	stats := &tenantStats{}
	ts.perTenant.Store(tenant.ID, stats)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Compute effective rate with time-of-day curve
		effectiveRate := tenant.RunsPerMinute
		if ts.config.TimeOfDayCurve {
			effectiveRate *= ts.timeOfDayMultiplier()
		}

		// Random burst: 5% chance of 10x per minute check
		if tenant.BurstProb > 0 && rand.Float64() < tenant.BurstProb { //nolint:gosec // non-cryptographic use for load test simulation
			effectiveRate *= 10
		}

		// Poisson-distributed inter-arrival time
		if effectiveRate <= 0 {
			time.Sleep(time.Second)
			continue
		}
		lambda := effectiveRate / 60.0 // Convert to per-second
		interArrival := min(poissonInterval(lambda), 10*time.Second)

		select {
		case <-ctx.Done():
			return
		case <-time.After(interArrival):
		}

		// Pick job type based on tenant profile
		jobType := ts.pickJobType(tenant)

		// Trigger in background to not block the timing loop
		go func() {
			if err := ts.triggerFn(ctx, tenant, jobType); err != nil {
				ts.totalErrs.Add(1)
				stats.errors.Add(1)
				return
			}
			ts.totalRuns.Add(1)
			stats.runs.Add(1)
		}()
	}
}

func (ts *TenantSimulator) pickJobType(tenant TenantProfile) string {
	r := rand.Float64() //nolint:gosec // non-cryptographic use for load test simulation
	if r < tenant.HTTPPercent {
		return "http"
	}
	if r < tenant.HTTPPercent+tenant.ManagedPercent {
		return "managed"
	}
	return "workflow"
}

// timeOfDayMultiplier returns a multiplier (0.2 - 1.0) based on a sinusoidal
// time-of-day curve peaking at PeakHourUTC and troughing at TroughHourUTC.
func (ts *TenantSimulator) timeOfDayMultiplier() float64 {
	hour := time.Now().UTC().Hour()
	peak := ts.config.PeakHourUTC

	// Distance from peak in hours (wrapping around 24h)
	dist := math.Abs(float64(hour - peak))
	if dist > 12 {
		dist = 24 - dist
	}

	// Cosine curve: 1.0 at peak, 0.2 at trough (12h away)
	return 0.6 + 0.4*math.Cos(dist*math.Pi/12)
}

func (ts *TenantSimulator) buildResult() *TenantSimulatorResult {
	result := &TenantSimulatorResult{
		Duration:    ts.config.Duration,
		TotalRuns:   ts.totalRuns.Load(),
		TotalErrors: ts.totalErrs.Load(),
		PerTenant:   make(map[string]TenantRunStats),
	}

	durationSecs := ts.config.Duration.Seconds()
	if durationSecs > 0 {
		result.RunsPerSecond = float64(result.TotalRuns) / durationSecs
	}

	ts.perTenant.Range(func(key, value any) bool {
		id := key.(string)
		stats := value.(*tenantStats)
		runs := stats.runs.Load()
		result.PerTenant[id] = TenantRunStats{
			Runs:   runs,
			Errors: stats.errors.Load(),
			Rate:   float64(runs) / max(durationSecs, 1),
		}
		return true
	})

	return result
}

// poissonInterval generates an exponentially distributed inter-arrival time
// for a Poisson process with the given rate (events per second).
func poissonInterval(lambda float64) time.Duration {
	if lambda <= 0 {
		return time.Second
	}
	// Exponential distribution: -ln(U) / lambda
	u := rand.Float64() //nolint:gosec // non-cryptographic use for Poisson interval generation
	if u == 0 {
		u = 1e-10
	}
	interval := -math.Log(u) / lambda
	return time.Duration(interval * float64(time.Second))
}

// GenerateTenants creates a set of tenant profiles with mixed plans.
func GenerateTenants(count int) []TenantProfile {
	tenants := make([]TenantProfile, count)
	for i := range tenants {
		plan := PlanStarter
		runsPerMin := 1.0 + rand.Float64()*5 //nolint:gosec // non-cryptographic use for load test simulation

		// 20% are pro tenants with higher traffic
		if rand.Float64() < 0.2 { //nolint:gosec // non-cryptographic use for load test simulation
			plan = PlanPro
			runsPerMin = 5.0 + rand.Float64()*20 //nolint:gosec // non-cryptographic use for load test simulation
		}

		tenants[i] = TenantProfile{
			ID:              fmt.Sprintf("tenant-%04d", i),
			Plan:            plan,
			RunsPerMinute:   runsPerMin,
			BurstProb:       0.05, // 5% chance of burst
			HTTPPercent:     0.6,  // 60% HTTP
			ManagedPercent:  0.3,  // 30% managed/Docker
			WorkflowPercent: 0.1,  // 10% workflow
		}
	}
	return tenants
}

// MarshalResult serializes the result as JSON.
func (r *TenantSimulatorResult) MarshalResult() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
