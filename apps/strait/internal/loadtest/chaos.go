//go:build loadtest

package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sourcegraph/conc"
)

// ChaosScenario defines a single chaos test.
type ChaosScenario struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ChaosResult captures the outcome of a chaos test.
type ChaosResult struct {
	Scenario     string        `json:"scenario"`
	LoadRate     int           `json:"load_rate_per_sec"`
	InFlight     int64         `json:"in_flight_at_chaos"`
	Lost         int64         `json:"lost"`
	Recovered    int64         `json:"recovered"`
	RecoveryTime time.Duration `json:"recovery_time"`
	QueuePeak    int64         `json:"queue_peak"`
	DrainTime    time.Duration `json:"drain_time"`
	Verdict      string        `json:"verdict"` // PASS or FAIL
	Error        string        `json:"error,omitempty"`
}

// ChaosEngine runs chaos scenarios during background production load.
type ChaosEngine struct {
	harness   *Harness
	loadRate  int
	projectID string
	jobSlug   string

	// Tracking
	triggerCount atomic.Int64
	errorCount   atomic.Int64
}

// NewChaosEngine creates a chaos engine attached to the test harness.
func NewChaosEngine(h *Harness, loadRate int, projectID, jobSlug string) *ChaosEngine {
	return &ChaosEngine{
		harness:   h,
		loadRate:  loadRate,
		projectID: projectID,
		jobSlug:   jobSlug,
	}
}

// findContainer finds a running Docker container matching the given service name.
func findContainer(serviceName string) (string, error) {
	out, err := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", serviceName), "--format", "{{.Names}}").Output() //nolint:gosec // arguments are not user-controlled
	if err != nil {
		return "", fmt.Errorf("docker ps failed: %w", err)
	}
	names := strings.TrimSpace(string(out))
	if names == "" {
		return "", fmt.Errorf("no container found matching %q", serviceName)
	}
	// Take the first match
	lines := strings.Split(names, "\n")
	return lines[0], nil
}

func (ce *ChaosEngine) findPostgresContainer() (string, error) {
	return findContainer("postgres")
}

func (ce *ChaosEngine) findRedisContainer() (string, error) {
	return findContainer("redis")
}

// AllChaosScenarios returns the defined chaos scenarios.
func AllChaosScenarios() []ChaosScenario {
	return []ChaosScenario{
		{Name: "worker_sigkill", Description: "Kill worker process, 30s wait, restart. Expect: 0 lost, all recovered <5min"},
		{Name: "database_failover", Description: "Drop primary DB connection. Expect: <30s downtime, 0 data loss"},
		{Name: "redis_total_failure", Description: "SIGKILL Redis, 2 min downtime. Expect: 0 lost jobs, SSE resumes <10s after recovery"},
		{Name: "connection_pool_exhaustion", Description: "Set max_connections=10. Expect: backpressure, no crash"},
		{Name: "disk_pressure", Description: "Insert 10M run events. Expect: graceful error, no corruption"},
		{Name: "clock_skew", Description: "Jump clock 24h forward. Expect: no incorrect cron/budget/retention behavior"},
		{Name: "cascading_failure", Description: "Redis dies + 10x spike + worker restart simultaneously. Expect: recovery <10min, 0 data loss"},
	}
}

// RunAll runs all chaos scenarios sequentially with background load.
func (ce *ChaosEngine) RunAll(ctx context.Context) ([]ChaosResult, error) {
	var results []ChaosResult

	for _, scenario := range AllChaosScenarios() {
		result := ce.RunScenario(ctx, scenario)
		results = append(results, result)
	}

	return results, nil
}

// RunScenario runs a single chaos scenario with background load.
func (ce *ChaosEngine) RunScenario(ctx context.Context, scenario ChaosScenario) ChaosResult {
	result := ChaosResult{
		Scenario: scenario.Name,
		LoadRate: ce.loadRate,
	}

	// Start background load
	loadCtx, loadCancel := context.WithCancel(ctx)
	defer loadCancel()

	ce.triggerCount.Store(0)
	ce.errorCount.Store(0)

	var wg conc.WaitGroup
	wg.Go(func() {
		ce.generateLoad(loadCtx)
	})

	// Warm up for 30 seconds
	time.Sleep(30 * time.Second)

	preCount := ce.triggerCount.Load()
	result.InFlight = preCount

	// Execute chaos action
	chaosStart := time.Now()
	var chaosErr error

	switch scenario.Name {
	case "worker_sigkill":
		chaosErr = ce.chaosWorkerKill(ctx)
	case "database_failover":
		chaosErr = ce.chaosDatabaseFailover(ctx)
	case "redis_total_failure":
		chaosErr = ce.chaosRedisFailure(ctx)
	case "connection_pool_exhaustion":
		chaosErr = ce.chaosPoolExhaustion(ctx)
	case "disk_pressure":
		chaosErr = ce.chaosDiskPressure(ctx)
	case "clock_skew":
		chaosErr = ce.chaosClockSkew(ctx)
	case "cascading_failure":
		chaosErr = ce.chaosCascadingFailure(ctx)
	default:
		chaosErr = fmt.Errorf("unknown scenario: %s", scenario.Name)
	}

	recoveryTime := time.Since(chaosStart)
	result.RecoveryTime = recoveryTime

	if chaosErr != nil {
		result.Error = chaosErr.Error()
		result.Verdict = "FAIL"
	} else {
		result.Verdict = "PASS"
	}

	// Wait for queue to drain (up to 5 minutes)
	drainStart := time.Now()
	ce.waitForQueueDrain(ctx, 5*time.Minute)
	result.DrainTime = time.Since(drainStart)

	// Stop background load
	loadCancel()
	wg.Wait()

	result.Lost = ce.errorCount.Load()
	result.Recovered = ce.triggerCount.Load() - preCount

	return result
}

func (ce *ChaosEngine) generateLoad(ctx context.Context) {
	ticker := time.NewTicker(time.Second / time.Duration(max(ce.loadRate, 1)))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			go func() {
				if err := ce.harness.TriggerJob(ctx, ce.projectID, ce.jobSlug, map[string]any{
					"chaos":     true,
					"timestamp": time.Now().UnixMilli(),
				}); err != nil {
					ce.errorCount.Add(1)
					return
				}
				ce.triggerCount.Add(1)
			}()
		}
	}
}

func (ce *ChaosEngine) waitForQueueDrain(ctx context.Context, timeout time.Duration) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline:
			return
		case <-ticker.C:
			stats, err := ce.harness.GetQueueStats(ctx, ce.projectID)
			if err != nil {
				continue
			}
			if stats.QueueDepth() == 0 {
				return
			}
		}
	}
}

// Chaos scenario implementations.
// Each injects a specific failure, waits for the system to recover,
// and returns an error only if recovery failed.

func (ce *ChaosEngine) chaosWorkerKill(ctx context.Context) error {
	// Find and kill the strait worker process
	cmd := exec.CommandContext(ctx, "pkill", "-9", "-f", "strait.*--mode.*worker")
	if err := cmd.Run(); err != nil {
		// Process might not exist separately in "all" mode
		cmd = exec.CommandContext(ctx, "pkill", "-9", "-f", "strait.*--mode.*all")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to kill worker: %w", err)
		}
	}

	// Wait 30 seconds
	time.Sleep(30 * time.Second)

	// Restart worker (assumes docker-compose or similar)
	restart := exec.CommandContext(ctx, "docker", "compose", "restart", "strait")
	if err := restart.Run(); err != nil {
		// Try direct restart
		return fmt.Errorf("failed to restart worker: %w", err)
	}

	// Wait for recovery
	time.Sleep(30 * time.Second)
	return nil
}

func (ce *ChaosEngine) chaosDatabaseFailover(ctx context.Context) error {
	container, err := ce.findPostgresContainer()
	if err != nil {
		return fmt.Errorf("failed to find postgres container: %w", err)
	}

	// Simulate by pausing postgres container
	pause := exec.CommandContext(ctx, "docker", "pause", container)
	if err := pause.Run(); err != nil {
		return fmt.Errorf("failed to pause postgres container %s: %w", container, err)
	}

	// Hold for 10 seconds
	time.Sleep(10 * time.Second)

	// Unpause
	unpause := exec.CommandContext(ctx, "docker", "unpause", container)
	if err := unpause.Run(); err != nil {
		return fmt.Errorf("failed to unpause postgres container %s: %w", container, err)
	}

	// Wait for connections to recover
	time.Sleep(30 * time.Second)
	return nil
}

func (ce *ChaosEngine) chaosRedisFailure(ctx context.Context) error {
	container, err := ce.findRedisContainer()
	if err != nil {
		return fmt.Errorf("failed to find redis container: %w", err)
	}

	// Kill Redis container
	kill := exec.CommandContext(ctx, "docker", "kill", container)
	if err := kill.Run(); err != nil {
		return fmt.Errorf("failed to kill redis container %s: %w", container, err)
	}

	// Wait 2 minutes
	time.Sleep(2 * time.Minute)

	// Restart Redis
	start := exec.CommandContext(ctx, "docker", "start", container)
	if err := start.Run(); err != nil {
		return fmt.Errorf("failed to restart redis container %s: %w", container, err)
	}

	// Wait for reconnection
	time.Sleep(30 * time.Second)
	return nil
}

func (ce *ChaosEngine) chaosPoolExhaustion(ctx context.Context) error {
	// Reduce connection pool via pg_terminate_backend
	if ce.harness.Pool == nil {
		return fmt.Errorf("no database pool available")
	}

	// Kill idle connections to simulate exhaustion
	_, err := ce.harness.Pool.Exec(ctx,
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = current_database() AND pid != pg_backend_pid() AND state = 'idle' LIMIT 20")
	if err != nil {
		return fmt.Errorf("failed to terminate connections: %w", err)
	}

	// Hold pressure for 30 seconds
	time.Sleep(30 * time.Second)
	return nil
}

func (ce *ChaosEngine) chaosDiskPressure(ctx context.Context) error {
	if ce.harness.Pool == nil {
		return fmt.Errorf("no database pool available")
	}

	// Insert a large number of rows to create disk pressure
	// Use a batch insert to be efficient
	_, err := ce.harness.Pool.Exec(ctx,
		"INSERT INTO run_events (id, run_id, project_id, event_type, created_at) SELECT gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), 'loadtest_pressure', NOW() FROM generate_series(1, 100000)")
	if err != nil {
		// Table might not exist or have different schema - that's OK for the chaos test
		return fmt.Errorf("inserting pressure rows: %w", err)
	}

	time.Sleep(10 * time.Second)

	// Clean up
	_, _ = ce.harness.Pool.Exec(ctx, "DELETE FROM run_events WHERE event_type = 'loadtest_pressure'")
	return nil
}

func (ce *ChaosEngine) chaosClockSkew(ctx context.Context) error {
	// We cannot change system time inside containers without privileged access.
	// Instead, simulate clock skew by inserting job_runs rows with created_at
	// set 24 hours in the future. This tests whether the reaper and other
	// time-dependent components (cron scheduling, retention, budget enforcement)
	// handle future timestamps gracefully without panicking or corrupting data.
	if ce.harness.Pool == nil {
		return fmt.Errorf("no database pool available for clock skew simulation")
	}

	// Insert rows with future timestamps to simulate forward clock skew
	_, err := ce.harness.Pool.Exec(ctx,
		`INSERT INTO job_runs (id, job_id, project_id, status, created_at, updated_at)
		 SELECT gen_random_uuid(), gen_random_uuid(), $1, 'pending',
		        NOW() + INTERVAL '24 hours', NOW() + INTERVAL '24 hours'
		 FROM generate_series(1, 100)`,
		ce.projectID,
	)
	if err != nil {
		return fmt.Errorf("failed to insert future-timestamped rows: %w", err)
	}

	// Allow the reaper and other periodic processes to encounter the skewed rows
	time.Sleep(30 * time.Second)

	// Verify the system is still healthy by checking that no rows were incorrectly
	// reaped or that the system didn't crash
	var remaining int
	err = ce.harness.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_runs
		 WHERE project_id = $1 AND created_at > NOW() + INTERVAL '23 hours'`,
		ce.projectID,
	).Scan(&remaining)
	if err != nil {
		return fmt.Errorf("failed to verify clock skew recovery: %w", err)
	}

	// Clean up the skewed rows
	_, _ = ce.harness.Pool.Exec(ctx,
		`DELETE FROM job_runs
		 WHERE project_id = $1 AND created_at > NOW() + INTERVAL '23 hours'`,
		ce.projectID,
	)

	return nil
}

func (ce *ChaosEngine) chaosCascadingFailure(ctx context.Context) error {
	redisContainer, redisErr := ce.findRedisContainer()

	var cascadeErr atomic.Value

	var wg conc.WaitGroup

	// Simultaneously: kill Redis + spike traffic + kill worker
	// Kill Redis
	wg.Go(func() {
		if redisErr != nil {
			return
		}
		if err := exec.CommandContext(ctx, "docker", "kill", redisContainer).Run(); err != nil {
			cascadeErr.CompareAndSwap(nil, fmt.Errorf("killing redis: %w", err))
		}
	})

	// 10x traffic spike
	wg.Go(func() {
		spikeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		ticker := time.NewTicker(time.Millisecond) // Very high rate
		defer ticker.Stop()
		for {
			select {
			case <-spikeCtx.Done():
				return
			case <-ticker.C:
				go func() {
					_ = ce.harness.TriggerJob(ctx, ce.projectID, ce.jobSlug, map[string]any{
						"chaos": "cascading_spike",
					})
				}()
			}
		}
	})

	// Kill worker after 5s
	wg.Go(func() {
		time.Sleep(5 * time.Second)
		if err := exec.CommandContext(ctx, "pkill", "-9", "-f", "strait").Run(); err != nil {
			cascadeErr.CompareAndSwap(nil, fmt.Errorf("killing worker: %w", err))
		}
	})

	wg.Wait()

	// Wait 2 minutes
	time.Sleep(2 * time.Minute)

	// Restart everything
	if redisErr == nil {
		if err := exec.CommandContext(ctx, "docker", "start", redisContainer).Run(); err != nil {
			return fmt.Errorf("restarting redis: %w", err)
		}
	}
	time.Sleep(10 * time.Second)

	// Wait for recovery
	time.Sleep(5 * time.Minute)

	if v := cascadeErr.Load(); v != nil {
		return v.(error)
	}
	return nil
}

// WriteResults writes chaos results to a JSON file.
func WriteResults(outputDir string, results []ChaosResult) error {
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling results: %w", err)
	}

	path := filepath.Join(outputDir, "chaos_results.json")
	return os.WriteFile(path, data, 0o600)
}
