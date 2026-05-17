//go:build loadtest

package loadtest

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestChaosHarness_DoesNotUseHostWideProcessKills(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("chaos.go")
	if err != nil {
		t.Fatalf("read chaos.go: %v", err)
	}
	source := string(data)
	for _, forbidden := range []string{`"pkill"`, `"killall"`, `"pgrep"`} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("chaos harness contains host-wide process command %s", forbidden)
		}
	}
}

func TestChaosHarness_RunEventsPressureUsesCurrentSchema(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("chaos.go")
	if err != nil {
		t.Fatalf("read chaos.go: %v", err)
	}
	source := string(data)
	if strings.Contains(source, "event_type") {
		t.Fatal("run_events pressure scenario references removed event_type column")
	}
	if strings.Contains(source, "run_events (id, run_id, project_id") {
		t.Fatal("run_events pressure scenario inserts removed project_id column")
	}
	if strings.Contains(source, "job_runs (id, job_id, project_id, status, payload, triggered_by, created_at, updated_at)") {
		t.Fatal("job_runs chaos scenarios insert removed updated_at column")
	}
	for _, required := range []string{"run_events (id, run_id, type, level, message, data, created_at)", "'loadtest_pressure'"} {
		if !strings.Contains(source, required) {
			t.Fatalf("run_events pressure scenario missing expected schema fragment %q", required)
		}
	}
	for _, required := range []string{
		"job_runs (id, job_id, project_id, status, payload, triggered_by, created_at)",
		"'loadtest-clock-skew-'",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("job_runs chaos scenario missing expected schema fragment %q", required)
		}
	}
}

func TestChaosHarness_DiskPressureCleanupIsRunScoped(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("chaos.go")
	if err != nil {
		t.Fatalf("read chaos.go: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		"DELETE FROM run_events re",
		"USING job_runs jr",
		"re.run_id = jr.id",
		"jr.project_id = $1",
		"jr.id LIKE 'loadtest-pressure-%'",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("disk pressure cleanup missing scoped fragment %q", required)
		}
	}
}

func TestFindContainer_RequiresExactLoadtestContainerName(t *testing.T) {
	restore := stubDockerContainerNames(t, []string{
		"customer-postgres",
		"strait-postgres-backup",
		"strait-postgres",
		"other-redis",
	})
	defer restore()

	got, err := findContainer("postgres")
	if err != nil {
		t.Fatalf("findContainer(postgres): %v", err)
	}
	if got != "strait-postgres" {
		t.Fatalf("findContainer(postgres) = %q, want exact strait-postgres", got)
	}
}

func TestFindContainer_FailsClosedWhenOnlySubstringMatches(t *testing.T) {
	restore := stubDockerContainerNames(t, []string{
		"customer-postgres",
		"strait-postgres-backup",
		"project-redis",
	})
	defer restore()

	if got, err := findContainer("postgres"); err == nil {
		t.Fatalf("findContainer(postgres) = %q, want error when only substring matches exist", got)
	}
}

func TestFindContainer_UsesExplicitLoadtestOverrideExactly(t *testing.T) {
	t.Setenv("LOADTEST_REDIS_CONTAINER", "strait-pr-147-redis")

	restore := stubDockerContainerNames(t, []string{
		"strait-redis",
		"strait-pr-147-redis-extra",
		"strait-pr-147-redis",
	})
	defer restore()

	got, err := findContainer("redis")
	if err != nil {
		t.Fatalf("findContainer(redis): %v", err)
	}
	if got != "strait-pr-147-redis" {
		t.Fatalf("findContainer(redis) = %q, want override exact match", got)
	}
}

func TestFindContainer_PropagatesDockerListError(t *testing.T) {
	want := errors.New("docker unavailable")

	orig := listDockerContainerNames
	listDockerContainerNames = func() ([]string, error) {
		return nil, want
	}
	t.Cleanup(func() {
		listDockerContainerNames = orig
	})

	if _, err := findContainer("postgres"); !errors.Is(err, want) {
		t.Fatalf("findContainer(postgres) error = %v, want %v", err, want)
	}
}

func TestChaosCascadingFailure_FailsClosedWhenRedisMissing(t *testing.T) {
	restore := stubDockerContainerNames(t, []string{"strait"})
	defer restore()

	ce := &ChaosEngine{}
	err := ce.chaosCascadingFailure(t.Context())
	if err == nil {
		t.Fatal("expected cascading failure scenario to fail when redis container is missing")
	}
	if !strings.Contains(err.Error(), "finding redis container") {
		t.Fatalf("error = %q, want redis discovery failure", err.Error())
	}
}

func TestTrafficSpikeFailsClosedWhenAllTriggersFail(t *testing.T) {
	t.Parallel()

	ce := &ChaosEngine{
		loadRate: 10,
		trigger: func(context.Context, string, string, map[string]any) error {
			return errors.New("trigger rejected")
		},
	}

	attempts, successes, err := ce.runTrafficSpike(t.Context(), 20*time.Millisecond, time.Millisecond)
	if err == nil {
		t.Fatal("expected traffic spike to fail when every trigger fails")
	}
	if attempts == 0 {
		t.Fatal("expected traffic spike attempts")
	}
	if successes != 0 {
		t.Fatalf("successes = %d, want 0", successes)
	}
	if ce.errorCount.Load() == 0 {
		t.Fatal("expected failed spike attempts to increment errorCount")
	}
}

func TestTrafficSpikeRequiresAtLeastOneAttempt(t *testing.T) {
	t.Parallel()

	ce := &ChaosEngine{loadRate: 1}
	attempts, successes, err := ce.runTrafficSpike(t.Context(), time.Nanosecond, time.Hour)
	if err == nil {
		t.Fatal("expected traffic spike with no ticks to fail")
	}
	if attempts != 0 || successes != 0 {
		t.Fatalf("attempts=%d successes=%d, want zero values", attempts, successes)
	}
}

func TestTrafficSpikeCountsSuccessfulTriggers(t *testing.T) {
	t.Parallel()

	ce := &ChaosEngine{
		loadRate: 10,
		trigger: func(context.Context, string, string, map[string]any) error {
			return nil
		},
	}

	attempts, successes, err := ce.runTrafficSpike(t.Context(), 20*time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("runTrafficSpike() error = %v", err)
	}
	if attempts == 0 || successes == 0 {
		t.Fatalf("attempts=%d successes=%d, want non-zero", attempts, successes)
	}
	if ce.triggerCount.Load() != successes {
		t.Fatalf("triggerCount = %d, want successes %d", ce.triggerCount.Load(), successes)
	}
}

func stubDockerContainerNames(t *testing.T, names []string) func() {
	t.Helper()

	orig := listDockerContainerNames
	listDockerContainerNames = func() ([]string, error) {
		return append([]string(nil), names...), nil
	}
	return func() {
		listDockerContainerNames = orig
	}
}
