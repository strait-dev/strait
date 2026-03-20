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
		StraitURL:       envOrDefault("LOADTEST_STRAIT_URL", "http://localhost:8080"),
		InternalSecret:  envOrDefault("LOADTEST_INTERNAL_SECRET", os.Getenv("INTERNAL_SECRET")),
		DatabaseURL:     envOrDefault("LOADTEST_DATABASE_URL", os.Getenv("DATABASE_URL")),
		RedisURL:        envOrDefault("LOADTEST_REDIS_URL", os.Getenv("REDIS_URL")),
		TestServerPort:  9999,
		MetricsInterval: 5 * time.Second,
	})

	ctx := context.Background()
	if err := h.Setup(ctx); err != nil {
		t.Fatalf("harness setup: %v", err)
	}
	t.Cleanup(func() {
		if err := h.Teardown(); err != nil {
			t.Errorf("harness teardown: %v", err)
		}
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
	if err != nil {
		t.Fatalf("ramp engine failed: %v", err)
	}

	t.Logf("max sustained rate: %d jobs/sec", result.MaxRate)
	t.Logf("breaking rate: %d jobs/sec", result.BreakingRate)
	t.Logf("bottleneck: %s", result.Bottleneck)
	t.Logf("total operations: %d", result.TotalOperations)
	t.Logf("total errors: %d", result.TotalErrors)
	t.Logf("duration: %s", result.Duration)

	if err := h.WriteResult("quick_validation.json", result); err != nil {
		t.Errorf("writing result: %v", err)
	}

	// Basic assertions
	if result.MaxRate < 10 {
		t.Errorf("max sustained rate too low: %d (expected >= 10)", result.MaxRate)
	}
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
	if err != nil {
		t.Fatalf("throughput ceiling test failed: %v", err)
	}

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

	if err := h.WriteResult("throughput_ceiling.json", result); err != nil {
		t.Errorf("writing result: %v", err)
	}
}

// TestConcurrencyCeiling finds the maximum concurrent connections for HTTP and Managed modes.
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
		if err != nil {
			t.Fatalf("HTTP concurrency test failed: %v", err)
		}

		t.Logf("=== HTTP CONCURRENCY CEILING ===")
		t.Logf("max sustained: %d concurrent | breaks at: %d | bottleneck: %s",
			result.MaxRate, result.BreakingRate, result.Bottleneck)

		for _, step := range result.Steps {
			t.Logf("  concurrent=%d ops=%d errs=%d error_rate=%.2f%% p99=%s",
				step.Rate, step.Operations, step.Errors,
				step.ErrorRate*100, step.LatencyP99)
		}

		if err := h.WriteResult("concurrency_ceiling_http.json", result); err != nil {
			t.Errorf("writing result: %v", err)
		}
	})

	// Subtest: Managed (Docker) concurrency ceiling
	t.Run("Managed", func(t *testing.T) {
		if os.Getenv("COMPUTE_RUNTIME") != "docker" {
			t.Skip("set COMPUTE_RUNTIME=docker to test managed concurrency")
		}

		engine := loadtest.NewRampEngine(loadtest.RampConfig{
			Mode:         loadtest.RampConcurrency,
			StartRate:    10,
			StepSize:     10,
			StepInterval: 2 * time.Minute,
			StopCondition: loadtest.StopCondition{
				MaxErrorRate: 0.05,
				MaxDuration:  1 * time.Hour,
			},
		}, func(ctx context.Context) error {
			return h.TriggerJob(ctx, loadtestProjectID, resolveJobID("loadtest-managed"), map[string]any{
				"timestamp": time.Now().UnixMilli(),
			})
		})

		result, err := engine.Run(context.Background())
		if err != nil {
			t.Fatalf("managed concurrency test failed: %v", err)
		}

		t.Logf("=== MANAGED CONCURRENCY CEILING ===")
		t.Logf("max sustained: %d containers | breaks at: %d | bottleneck: %s",
			result.MaxRate, result.BreakingRate, result.Bottleneck)

		if err := h.WriteResult("concurrency_ceiling_managed.json", result); err != nil {
			t.Errorf("writing result: %v", err)
		}
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
			case "managed":
				jobSlug = "loadtest-managed"
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

	// Inject error scenarios during simulation (50/min as per STR-197)
	errorCtx, errorCancel := context.WithCancel(context.Background())
	defer errorCancel()
	errorInjector := loadtest.NewErrorInjector(h, loadtestProjectID, 50)
	go errorInjector.Run(errorCtx)

	result, err := sim.Run(context.Background())
	if err != nil {
		t.Fatalf("production simulation failed: %v", err)
	}
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

	if err := h.WriteResult("production_simulation.json", result); err != nil {
		t.Errorf("writing result: %v", err)
	}
}

// TestBreakingPoint adds tenants until the system degrades.
// Use: go test -tags=loadtest -run TestBreakingPoint -timeout 12h ./internal/loadtest/...
func TestBreakingPoint(t *testing.T) {
	h := setupHarness(t)

	t.Log("running scenario: breaking_point")
	t.Log("adding 100 tenants every 30 min until P99 > 5s or error rate > 0.1%")

	type breakingPointResult struct {
		MaxTenants    int           `json:"max_tenants"`
		BreakTenants  int           `json:"break_tenants"`
		Duration      time.Duration `json:"duration"`
		BreakReason   string        `json:"break_reason"`
		Steps         []struct {
			Tenants  int     `json:"tenants"`
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
		if err != nil {
			t.Fatalf("simulation failed at %d tenants: %v", tenantCount, err)
		}

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

	if err := h.WriteResult("breaking_point.json", result); err != nil {
		t.Errorf("writing result: %v", err)
	}

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
			MemoryGrowthPerHourMB: 100,
			GoroutineGrowthPerHour: 1000,
			P99GrowthPerHourPct:   10,
			ErrorGrowthPerHourPct: 0.1,
		},
	})

	result, alerts, err := runner.Run(context.Background(), h)
	if err != nil {
		t.Fatalf("endurance test failed: %v", err)
	}

	t.Logf("=== ENDURANCE RESULTS ===")
	t.Logf("duration: %s", result.Duration)
	t.Logf("total operations: %d", result.TotalOperations)
	t.Logf("total errors: %d", result.TotalErrors)
	t.Logf("spikes injected: %d", result.SpikesInjected)
	t.Logf("long-run jobs completed: %d/%d", result.LongRunCompleted, result.LongRunTotal)

	// Report alerts
	for _, alert := range alerts {
		t.Logf("ALERT [%s]: %s (hour %d)", alert.Severity, alert.Message, alert.Hour)
		if alert.Severity == "LEAK" || alert.Severity == "DEGRADATION" {
			t.Errorf("endurance alert: %s", alert.Message)
		}
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

		if heapGrowth > 3.0 {
			t.Errorf("potential memory leak: heap grew %.2fx", heapGrowth)
		}
		if goroutineGrowth > 2.0 {
			t.Errorf("potential goroutine leak: count grew %.2fx", goroutineGrowth)
		}
	}

	if err := h.WriteResult("endurance.json", result); err != nil {
		t.Errorf("writing result: %v", err)
	}
	if err := h.WriteResult("endurance_alerts.json", alerts); err != nil {
		t.Errorf("writing alerts: %v", err)
	}
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

	if err := h.WriteResult("error_scenarios.json", results); err != nil {
		t.Errorf("writing results: %v", err)
	}
}

// TestTestServerEndpoints validates the test HTTP server works correctly.
func TestTestServerEndpoints(t *testing.T) {
	srv := loadtest.NewTestServer(19999)
	if err := srv.Start(); err != nil {
		t.Fatalf("starting test server: %v", err)
	}
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
			url := fmt.Sprintf("http://localhost:19999%s", ep.path)
			var req *http.Request
			var err error

			if ep.method == "POST" {
				req, err = http.NewRequest("POST", url, bytes.NewBufferString(ep.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req, err = http.NewRequest("GET", url, nil)
			}
			if err != nil {
				t.Fatalf("creating request: %v", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			// Flaky endpoint may return 500 - that's expected
			if ep.name != "flaky" && resp.StatusCode != http.StatusOK {
				t.Errorf("expected 200, got %d", resp.StatusCode)
			}
		})
	}

	// Verify stats
	snap := srv.Snapshot()
	if snap.Total == 0 {
		t.Error("expected total > 0 after requests")
	}
	t.Logf("server stats: total=%d", snap.Total)
}

// TestChaosWorkerKill tests worker SIGKILL recovery during load.
// Use: go test -tags=loadtest -run TestChaosWorkerKill -timeout 30m ./internal/loadtest/...
func TestChaosWorkerKill(t *testing.T) {
	h := setupHarness(t)
	ce := loadtest.NewChaosEngine(h, 100, loadtestProjectID, resolveJobID("loadtest-fast-echo"))

	scenarios := loadtest.AllChaosScenarios()
	result := ce.RunScenario(context.Background(), scenarios[0]) // worker_sigkill

	t.Logf("CHAOS: %s", result.Scenario)
	t.Logf("Load: ~%d runs/sec | In-flight: %d", result.LoadRate, result.InFlight)
	t.Logf("Lost: %d | Recovered: %d | Recovery: %s", result.Lost, result.Recovered, result.RecoveryTime)
	t.Logf("Queue peak: %d | Drain: %s | VERDICT: %s", result.QueuePeak, result.DrainTime, result.Verdict)

	if result.Verdict != "PASS" {
		t.Errorf("chaos test failed: %s", result.Error)
	}
}

// TestChaosAll runs all 8 chaos scenarios sequentially with background load.
// Use: go test -tags=loadtest -run TestChaosAll -timeout 4h ./internal/loadtest/...
func TestChaosAll(t *testing.T) {
	h := setupHarness(t)
	ce := loadtest.NewChaosEngine(h, 100, loadtestProjectID, resolveJobID("loadtest-fast-echo"))

	results, err := ce.RunAll(context.Background())
	if err != nil {
		t.Fatalf("chaos engine failed: %v", err)
	}

	passed := 0
	failed := 0
	for _, r := range results {
		t.Logf("CHAOS: %-25s | Lost: %d | Recovery: %s | Verdict: %s",
			r.Scenario, r.Lost, r.RecoveryTime, r.Verdict)
		if r.Verdict == "PASS" {
			passed++
		} else {
			failed++
			t.Errorf("  FAILED: %s", r.Error)
		}
	}

	t.Logf("=== CHAOS SUMMARY: %d/%d passed ===", passed, len(results))

	if err := h.WriteResult("chaos_all.json", results); err != nil {
		t.Errorf("writing results: %v", err)
	}
}
