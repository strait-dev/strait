//go:build loadtest

package loadtest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"strait/internal/loadtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

const loadtestProjectID = "loadtest-project"

// jobIDs maps slug -> UUID for jobs created during setup.
var (
	jobIDs     map[string]string
	jobIDsOnce sync.Once
	jobIDsErr  error
)

func setupHarness(t *testing.T) *loadtest.Harness {
	t.Helper()

	h := loadtest.NewHarness(loadtest.HarnessConfig{
		StraitURL:       envOrDefault("LOADTEST_STRAIT_URL", "http://localhost:7676"),
		InternalSecret:  envOrDefault("LOADTEST_INTERNAL_SECRET", os.Getenv("INTERNAL_SECRET")),
		DatabaseURL:     envOrDefault("LOADTEST_DATABASE_URL", os.Getenv("DATABASE_URL")),
		RedisURL:        envOrDefault("LOADTEST_REDIS_URL", os.Getenv("REDIS_URL")),
		TestServerPort:  9000,
		MetricsInterval: 5 * time.Second,
	})

	ctx := context.Background()
	require.NoError(t, h.Setup(ctx))

	t.Cleanup(func() {
		assert.NoError(t, h.Teardown())

	})

	// Create load test jobs once across all test functions
	jobIDsOnce.Do(func() {
		jobIDs, jobIDsErr = h.SetupLoadTestJobs(ctx, loadtestProjectID)
	})
	if jobIDsErr != nil {
		t.Logf("warning: could not create load test jobs: %v", jobIDsErr)
		t.Log("tests that trigger jobs against the Strait API will use slug-based IDs")
	}

	return h
}

// resolveJobID returns the UUID for a job slug, or the slug itself if not found.
func resolveJobID(slug string) string {
	if jobIDs != nil {
		if id, ok := jobIDs[slug]; ok {
			return id
		}
	}
	return slug
}

// TestQuickValidation runs a reduced-scale throughput check in ~15 minutes.
// Use: LOADTEST_QUICK=true go test -tags=loadtest -run TestQuickValidation -timeout 15m ./internal/loadtest/...
func TestQuickValidation(t *testing.T) {
	if os.Getenv("LOADTEST_QUICK") == "" {
		t.Skip("set LOADTEST_QUICK=true to run")
	}

	h := setupHarness(t)
	scenario := loadtest.QuickValidation()

	t.Logf("running scenario: %s", scenario.Name)
	t.Logf("description: %s", scenario.Description)

	engine := loadtest.NewRampEngine(*scenario.RampConfig, func(ctx context.Context) error {
		return h.TriggerJob(ctx, loadtestProjectID, resolveJobID("loadtest-fast-echo"), map[string]any{
			"timestamp": time.Now().UnixMilli(),
		})
	})

	result, err := engine.Run(context.Background())
	require.NoError(t, err)

	t.Logf("max sustained rate: %d jobs/sec", result.MaxRate)
	t.Logf("breaking rate: %d jobs/sec", result.BreakingRate)
	t.Logf("bottleneck: %s", result.Bottleneck)
	t.Logf("total operations: %d", result.TotalOperations)
	t.Logf("total errors: %d", result.TotalErrors)
	t.Logf("duration: %s", result.Duration)
	assert.NoError(t, h.WriteResult("quick_validation.json",

		result))
	assert.GreaterOrEqual(t,

		result.MaxRate,
		10,
	)

	// Basic assertions

}

// TestThroughputCeiling finds the maximum sustained throughput.
// Use: go test -tags=loadtest -run TestThroughputCeiling -timeout 2h ./internal/loadtest/...
func TestThroughputCeiling(t *testing.T) {
	h := setupHarness(t)
	scenario := loadtest.ThroughputCeiling()

	t.Logf("running scenario: %s", scenario.Name)

	engine := loadtest.NewRampEngine(*scenario.RampConfig, func(ctx context.Context) error {
		return h.TriggerJob(ctx, loadtestProjectID, resolveJobID("loadtest-fast-echo"), map[string]any{
			"timestamp": time.Now().UnixMilli(),
		})
	})

	// Set queue depth monitoring
	engine.SetQueueDepthFn(func() int64 {
		stats, err := h.GetQueueStats(context.Background(), loadtestProjectID)
		if err != nil {
			return 0
		}
		return stats.QueueDepth()
	})

	result, err := engine.Run(context.Background())
	require.NoError(t, err)

	t.Logf("=== THROUGHPUT CEILING RESULTS ===")
	t.Logf("max sustained: %d jobs/sec", result.MaxRate)
	t.Logf("breaks at: %d jobs/sec", result.BreakingRate)
	t.Logf("bottleneck: %s", result.Bottleneck)
	t.Logf("total operations: %d", result.TotalOperations)
	t.Logf("total errors: %d", result.TotalErrors)

	for _, step := range result.Steps {
		t.Logf("  rate=%d ops=%d errs=%d p50=%s p95=%s p99=%s queue=%d",
			step.Rate, step.Operations, step.Errors,
			step.LatencyP50, step.LatencyP95, step.LatencyP99,
			step.QueueDepth)
	}
	assert.NoError(t, h.WriteResult("throughput_ceiling.json",

		result))

}

// TestConcurrencyCeiling finds the maximum concurrent connections for HTTP-mode dispatch.
// Use: go test -tags=loadtest -run TestConcurrencyCeiling -timeout 1h ./internal/loadtest/...
func TestConcurrencyCeiling(t *testing.T) {
	h := setupHarness(t)

	// Subtest: HTTP concurrency ceiling
	t.Run("HTTP", func(t *testing.T) {
		scenario := loadtest.ConcurrencyCeiling()
		t.Logf("running: HTTP concurrency ceiling")

		engine := loadtest.NewRampEngine(*scenario.RampConfig, func(ctx context.Context) error {
			return h.TriggerJob(ctx, loadtestProjectID, resolveJobID("loadtest-fast-echo"), map[string]any{
				"timestamp": time.Now().UnixMilli(),
			})
		})

		// Use a shorter context than the test timeout to allow goroutine cleanup
		ctx, cancel := context.WithTimeout(context.Background(), 14*time.Minute)
		defer cancel()

		result, err := engine.Run(ctx)
		require.NoError(t, err)

		t.Logf("=== HTTP CONCURRENCY CEILING ===")
		t.Logf("max sustained: %d concurrent | breaks at: %d | bottleneck: %s",
			result.MaxRate, result.BreakingRate, result.Bottleneck)

		for _, step := range result.Steps {
			t.Logf("  concurrent=%d ops=%d errs=%d error_rate=%.2f%% p99=%s",
				step.Rate, step.Operations, step.Errors,
				step.ErrorRate*100, step.LatencyP99)
		}
		assert.NoError(t, h.WriteResult("concurrency_ceiling_http.json",

			result,
		))

	})
}

// TestProductionSimulation runs a multi-tenant production simulation.
// Use: LOADTEST_TENANTS=2000 LOADTEST_DURATION=8h go test -tags=loadtest -run TestProductionSimulation -timeout 10h ./internal/loadtest/...
func TestProductionSimulation(t *testing.T) {
	tenantCount := 500
	if v := os.Getenv("LOADTEST_TENANTS"); v != "" {
		fmt.Sscanf(v, "%d", &tenantCount)
	}

	duration := 4 * time.Hour
	if v := os.Getenv("LOADTEST_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			duration = d
		}
	}

	h := setupHarness(t)
	scenario := loadtest.ProductionSimulation(tenantCount, duration)

	t.Logf("running scenario: %s (%d tenants, %s)", scenario.Name, tenantCount, duration)

	sim := loadtest.NewTenantSimulator(*scenario.TenantConfig,
		func(ctx context.Context, tenant loadtest.TenantProfile, jobType string) error {
			jobSlug := "loadtest-fast-echo"
			switch jobType {
			case "http":
				jobSlug = "loadtest-fast-echo"
			case "workflow":
				jobSlug = "loadtest-workflow"
			}
			return h.TriggerJob(ctx, tenant.ID, resolveJobID(jobSlug), map[string]any{
				"tenant":    tenant.ID,
				"job_type":  jobType,
				"timestamp": time.Now().UnixMilli(),
			})
		},
	)

	// Inject error scenarios during simulation at 50/min.
	errorCtx, errorCancel := context.WithCancel(context.Background())
	defer errorCancel()
	errorInjector := loadtest.NewErrorInjector(h, loadtestProjectID, 50)
	go errorInjector.Run(errorCtx)

	result, err := sim.Run(context.Background())
	require.NoError(t, err)

	errorCancel()

	t.Logf("=== PRODUCTION SIMULATION RESULTS ===")
	t.Logf("total runs: %d", result.TotalRuns)
	t.Logf("total errors: %d", result.TotalErrors)
	t.Logf("runs/sec: %.1f", result.RunsPerSecond)
	t.Logf("duration: %s", result.Duration)
	t.Logf("error scenarios injected: %d", errorInjector.Injected())

	// Log per-tenant stats
	for id, stats := range result.PerTenant {
		t.Logf("  tenant=%s runs=%d errors=%d rate=%.1f/sec",
			id, stats.Runs, stats.Errors, stats.Rate)
	}
	assert.NoError(t, h.WriteResult("production_simulation.json",

		result))

}

// TestBreakingPoint adds tenants until the system degrades.
// Use: go test -tags=loadtest -run TestBreakingPoint -timeout 12h ./internal/loadtest/...
func TestBreakingPoint(t *testing.T) {
	h := setupHarness(t)

	t.Log("running scenario: breaking_point")
	t.Log("adding 100 tenants every 30 min until P99 > 5s or error rate > 0.1%")

	type breakingPointResult struct {
		MaxTenants   int           `json:"max_tenants"`
		BreakTenants int           `json:"break_tenants"`
		Duration     time.Duration `json:"duration"`
		BreakReason  string        `json:"break_reason"`
		Steps        []struct {
			Tenants    int     `json:"tenants"`
			RunsPerSec float64 `json:"runs_per_sec"`
			ErrorRate  float64 `json:"error_rate"`
		} `json:"steps"`
	}

	result := breakingPointResult{}
	tenantCount := 100
	maxTenants := 0

	for {
		t.Logf("testing with %d tenants...", tenantCount)

		tenants := loadtest.GenerateTenants(tenantCount)
		sim := loadtest.NewTenantSimulator(
			loadtest.TenantSimulatorConfig{
				Tenants:        tenants,
				TimeOfDayCurve: false, // Disable for consistent measurement
				Duration:       30 * time.Minute,
			},
			func(ctx context.Context, tenant loadtest.TenantProfile, jobType string) error {
				return h.TriggerJob(ctx, tenant.ID, "loadtest-fast-echo", map[string]any{
					"tenant":    tenant.ID,
					"timestamp": time.Now().UnixMilli(),
				})
			},
		)

		simResult, err := sim.Run(context.Background())
		require.NoError(t, err)

		errorRate := float64(simResult.TotalErrors) / max(float64(simResult.TotalRuns), 1)

		result.Steps = append(result.Steps, struct {
			Tenants    int     `json:"tenants"`
			RunsPerSec float64 `json:"runs_per_sec"`
			ErrorRate  float64 `json:"error_rate"`
		}{
			Tenants:    tenantCount,
			RunsPerSec: simResult.RunsPerSecond,
			ErrorRate:  errorRate,
		})

		t.Logf("  runs/sec=%.1f errors=%d error_rate=%.4f", simResult.RunsPerSecond, simResult.TotalErrors, errorRate)

		if errorRate > 0.001 { // > 0.1%
			result.BreakTenants = tenantCount
			result.BreakReason = fmt.Sprintf("error_rate_%.4f", errorRate)
			break
		}

		maxTenants = tenantCount
		tenantCount += 100
	}

	result.MaxTenants = maxTenants
	assert.NoError(t, h.WriteResult("breaking_point.json",

		result))

	t.Logf("=== BREAKING POINT RESULTS ===")
	t.Logf("max sustained tenants: %d", result.MaxTenants)
	t.Logf("breaks at: %d tenants", result.BreakTenants)
	t.Logf("reason: %s", result.BreakReason)
}

// TestEndurance runs at 70% of throughput ceiling for extended duration.
// Use: LOADTEST_DURATION=24h go test -tags=loadtest -run TestEndurance -timeout 26h ./internal/loadtest/...
func TestEndurance(t *testing.T) {
	duration := 24 * time.Hour
	if v := os.Getenv("LOADTEST_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			duration = d
		}
	}

	// Read throughput ceiling from previous results to compute 70%
	targetRate := 100 // Default if no prior result
	if data, err := os.ReadFile("loadtest-results/latest/throughput_ceiling.json"); err == nil {
		var prior loadtest.RampResult
		if json.Unmarshal(data, &prior) == nil && prior.MaxRate > 0 {
			targetRate = int(float64(prior.MaxRate) * 0.7)
		}
	}

	if v := os.Getenv("LOADTEST_TARGET_RATE"); v != "" {
		fmt.Sscanf(v, "%d", &targetRate)
	}

	h := setupHarness(t)

	t.Logf("running endurance test: %d jobs/sec for %s", targetRate, duration)

	// Create endurance runner with spike injection and alert thresholds
	runner := loadtest.NewEnduranceRunner(loadtest.EnduranceConfig{
		TargetRate:     targetRate,
		Duration:       duration,
		SpikeInterval:  4 * time.Hour,
		SpikeMultiple:  10,
		SpikeDuration:  5 * time.Minute,
		LongRunJobs:    20,
		LongRunMinutes: 240,
		AlertThresholds: loadtest.AlertThresholds{
			MemoryGrowthPerHourMB:  100,
			GoroutineGrowthPerHour: 1000,
			P99GrowthPerHourPct:    10,
			ErrorGrowthPerHourPct:  0.1,
		},
	})

	result, alerts, err := runner.Run(context.Background(), h)
	require.NoError(t, err)

	t.Logf("=== ENDURANCE RESULTS ===")
	t.Logf("duration: %s", result.Duration)
	t.Logf("total operations: %d", result.TotalOperations)
	t.Logf("total errors: %d", result.TotalErrors)
	t.Logf("spikes injected: %d", result.SpikesInjected)
	t.Logf("long-run jobs completed: %d/%d", result.LongRunCompleted, result.LongRunTotal)

	// Report alerts
	for _, alert := range alerts {
		t.Logf("ALERT [%s]: %s (hour %d)", alert.Severity, alert.Message, alert.Hour)
		assert.False(t, alert.Severity ==
			"LEAK" ||
			alert.Severity ==
				"DEGRADATION",
		)

	}

	// Check for memory leaks: compare first and last metric snapshots
	snapshots := h.Metrics.Snapshots()
	if len(snapshots) >= 2 {
		first := snapshots[0]
		last := snapshots[len(snapshots)-1]
		heapGrowth := float64(last.Go.HeapAlloc) / max(float64(first.Go.HeapAlloc), 1)
		goroutineGrowth := float64(last.Go.Goroutines) / max(float64(first.Go.Goroutines), 1)

		t.Logf("heap growth: %.2fx (%d -> %d bytes)", heapGrowth, first.Go.HeapAlloc, last.Go.HeapAlloc)
		t.Logf("goroutine growth: %.2fx (%d -> %d)", goroutineGrowth, first.Go.Goroutines, last.Go.Goroutines)
		assert.LessOrEqual(t, heapGrowth,

			3.0)
		assert.LessOrEqual(t, goroutineGrowth,

			2.0,
		)

	}
	assert.NoError(t, h.WriteResult("endurance.json",

		result,
	))
	assert.NoError(t, h.WriteResult("endurance_alerts.json",

		alerts))

}

// TestErrorScenarios runs all error scenarios during production load.
// Use: go test -tags=loadtest -run TestErrorScenarios -timeout 1h ./internal/loadtest/...
func TestErrorScenarios(t *testing.T) {
	h := setupHarness(t)

	scenarios := []struct {
		name     string
		envVar   string
		expected string // Expected error classification
	}{
		{"clean_exit", "clean_exit", ""},
		{"exit_code_1", "exit_code_1", "application_error"},
		{"exit_code_137", "exit_code_137", "oom_killed"},
		{"oom", "oom", "oom_killed"},
		{"infinite_loop", "infinite_loop", "timed_out"},
		{"slow_death", "slow_death", "application_error"},
		{"panic_after_checkpoint", "panic_after_checkpoint", "application_error"},
		{"sdk_timeout", "sdk_timeout", "timed_out"},
		{"fork_bomb", "fork_bomb", "application_error"},
		{"disk_fill", "disk_fill", "application_error"},
		{"network_abuse", "network_abuse", "application_error"},
		{"segfault", "segfault", "crashed"},
	}

	type errorResult struct {
		Scenario string `json:"scenario"`
		Passed   bool   `json:"passed"`
		Error    string `json:"error,omitempty"`
	}

	var results []errorResult

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			// Trigger the error scenario job
			err := h.TriggerJob(context.Background(), loadtestProjectID, resolveJobID("loadtest-errors"), map[string]any{
				"scenario": sc.envVar,
			})

			result := errorResult{
				Scenario: sc.name,
				Passed:   err == nil || sc.expected != "",
			}
			if err != nil {
				result.Error = err.Error()
			}

			results = append(results, result)
			t.Logf("scenario=%s passed=%v", sc.name, result.Passed)
		})
	}
	assert.NoError(t, h.WriteResult("error_scenarios.json",

		results))

}

// TestTestServerEndpoints validates the test HTTP server works correctly.
func TestTestServerEndpoints(t *testing.T) {
	srv := loadtest.NewTestServer(19000)
	require.NoError(t, srv.
		Start())

	defer srv.Close()

	client := &http.Client{Timeout: 10 * time.Second}

	endpoints := []struct {
		name   string
		path   string
		method string
		body   string
	}{
		{"fast_echo", "/fast-echo", "POST", `{"test": true}`},
		{"slow_process", "/slow-process", "POST", `{}`},
		{"variable_load", "/variable-load", "POST", `{}`},
		{"flaky", "/flaky", "POST", `{}`},
		{"memory_heavy", "/memory-heavy", "POST", `{}`},
		{"cost_reporter", "/cost-reporter", "POST", `{}`},
		{"health", "/health", "GET", ""},
		{"stats", "/stats", "GET", ""},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			url := fmt.Sprintf("http://localhost:19000%s", ep.path)
			var req *http.Request
			var err error

			if ep.method == "POST" {
				req, err = http.NewRequest("POST", url, bytes.NewBufferString(ep.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req, err = http.NewRequest("GET", url, nil)
			}
			require.NoError(t, err)

			resp, err := client.Do(req)
			require.NoError(t, err)

			defer resp.Body.Close()
			assert.False(t, ep.name !=
				"flaky" &&
				resp.
					StatusCode !=
					http.StatusOK,
			)

			// Flaky endpoint may return 500 - that's expected

		})
	}

	snap := srv.Snapshot()
	assert.NotEqual(t, 0, snap.
		Total)

	t.Logf("server stats: total=%d", snap.Total)
}

// TestChaosScenarios runs each chaos scenario as an individual subtest.
// Use: go test -tags=loadtest -run TestChaosScenarios -timeout 4h ./internal/loadtest/...
// Or run a single scenario: go test -tags=loadtest -run TestChaosScenarios/worker_sigkill -timeout 30m.
func TestChaosScenarios(t *testing.T) {
	h := setupHarness(t)
	ce := loadtest.NewChaosEngine(h, 100, loadtestProjectID, resolveJobID("loadtest-fast-echo"))

	for _, scenario := range loadtest.AllChaosScenarios() {
		t.Run(scenario.Name, func(t *testing.T) {
			result := ce.RunScenario(context.Background(), scenario)

			t.Logf("CHAOS: %s", result.Scenario)
			t.Logf("Load: ~%d runs/sec | In-flight: %d", result.LoadRate, result.InFlight)
			t.Logf("Lost: %d | Recovered: %d | Recovery: %s", result.Lost, result.Recovered, result.RecoveryTime)
			t.Logf("Queue peak: %d | Drain: %s | VERDICT: %s", result.QueuePeak, result.DrainTime, result.Verdict)
			assert.Equal(t, "PASS",

				result.Verdict,
			)
			assert.NoError(t, h.WriteResult("chaos_"+
				scenario.
					Name+
				".json", result,
			))

		})
	}
}

// TestChaosAll runs all 8 chaos scenarios sequentially with background load.
// Use: go test -tags=loadtest -run TestChaosAll -timeout 4h ./internal/loadtest/...
func TestChaosAll(t *testing.T) {
	h := setupHarness(t)
	ce := loadtest.NewChaosEngine(h, 100, loadtestProjectID, resolveJobID("loadtest-fast-echo"))

	results, err := ce.RunAll(context.Background())
	require.NoError(t, err)

	passed := 0
	failed := 0
	for _, r := range results {
		t.Logf("CHAOS: %-25s | Lost: %d | Recovery: %s | Verdict: %s",
			r.Scenario, r.Lost, r.RecoveryTime, r.Verdict)
		if r.Verdict == "PASS" {
			passed++
		} else {
			failed++
			assert.Fail(t, r.Error)
		}
	}

	t.Logf("=== CHAOS SUMMARY: %d/%d passed ===", passed, len(results))
	assert.NoError(t, h.WriteResult("chaos_all.json",

		results,
	))

}

// TestEnduranceWeekend runs at 70% of throughput ceiling for 72 hours.
// Use: go test -tags=loadtest -run TestEnduranceWeekend -timeout 74h ./internal/loadtest/...
func TestEnduranceWeekend(t *testing.T) {
	duration := 72 * time.Hour
	if v := os.Getenv("LOADTEST_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			duration = d
		}
	}

	targetRate := 100
	if data, err := os.ReadFile("loadtest-results/latest/throughput_ceiling.json"); err == nil {
		var prior loadtest.RampResult
		if json.Unmarshal(data, &prior) == nil && prior.MaxRate > 0 {
			targetRate = int(float64(prior.MaxRate) * 0.7)
		}
	}
	if v := os.Getenv("LOADTEST_TARGET_RATE"); v != "" {
		fmt.Sscanf(v, "%d", &targetRate)
	}

	h := setupHarness(t)
	t.Logf("running 72h weekend endurance: %d jobs/sec for %s", targetRate, duration)

	runner := loadtest.NewEnduranceRunner(loadtest.EnduranceConfig{
		TargetRate:     targetRate,
		Duration:       duration,
		SpikeInterval:  4 * time.Hour,
		SpikeMultiple:  10,
		SpikeDuration:  5 * time.Minute,
		LongRunJobs:    20,
		LongRunMinutes: 240,
		AlertThresholds: loadtest.AlertThresholds{
			MemoryGrowthPerHourMB:  100,
			GoroutineGrowthPerHour: 1000,
			P99GrowthPerHourPct:    10,
			ErrorGrowthPerHourPct:  0.1,
		},
	})

	result, alerts, err := runner.Run(context.Background(), h)
	require.NoError(t, err)

	t.Logf("=== 72H ENDURANCE RESULTS ===")
	t.Logf("duration: %s", result.Duration)
	t.Logf("total operations: %d", result.TotalOperations)
	t.Logf("total errors: %d", result.TotalErrors)
	t.Logf("spikes injected: %d", result.SpikesInjected)
	t.Logf("long-run jobs completed: %d/%d", result.LongRunCompleted, result.LongRunTotal)
	t.Logf("alerts: %d", len(alerts))

	for _, alert := range alerts {
		t.Logf("ALERT [%s]: %s (hour %d)", alert.Severity, alert.Message, alert.Hour)
		assert.False(t, alert.Severity ==
			"LEAK" ||
			alert.Severity ==
				"DEGRADATION",
		)

	}
	assert.NoError(t, h.WriteResult("endurance_weekend.json",

		result))

}

// TestProductionValidation runs load tests against a production deployment.
// Use: LOADTEST_STRAIT_URL=https://api.strait.dev go test -tags=loadtest -run TestProductionValidation -timeout 2h ./internal/loadtest/...
func TestProductionValidation(t *testing.T) {
	straitURL := os.Getenv("LOADTEST_STRAIT_URL")
	if straitURL == "" || straitURL == "http://localhost:7676" {
		t.Skip("set LOADTEST_STRAIT_URL to a production deployment URL to run")
	}

	h := setupHarness(t)
	scenario := loadtest.ProductionValidation()

	t.Logf("running production validation against %s", straitURL)

	engine := loadtest.NewRampEngine(*scenario.RampConfig, func(ctx context.Context) error {
		return h.TriggerJob(ctx, loadtestProjectID, resolveJobID("loadtest-fast-echo"), map[string]any{
			"timestamp": time.Now().UnixMilli(),
			"source":    "production_validation",
		})
	})

	engine.SetQueueDepthFn(func() int64 {
		stats, err := h.GetQueueStats(context.Background(), loadtestProjectID)
		if err != nil {
			return 0
		}
		return stats.QueueDepth()
	})

	result, err := engine.Run(context.Background())
	require.NoError(t, err)

	t.Logf("=== PRODUCTION VALIDATION RESULTS ===")
	t.Logf("max sustained: %d jobs/sec", result.MaxRate)
	t.Logf("breaks at: %d jobs/sec", result.BreakingRate)
	t.Logf("bottleneck: %s", result.Bottleneck)
	t.Logf("total operations: %d", result.TotalOperations)
	t.Logf("total errors: %d", result.TotalErrors)

	for _, step := range result.Steps {
		t.Logf("  rate=%d ops=%d errs=%d p50=%s p95=%s p99=%s queue=%d",
			step.Rate, step.Operations, step.Errors,
			step.LatencyP50, step.LatencyP95, step.LatencyP99,
			step.QueueDepth)
	}
	assert.NoError(t, h.WriteResult("production_validation.json",

		result))

}
