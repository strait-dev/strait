//go:build loadtest

package loadtest

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChaosHarness_DoesNotUseHostWideProcessKills(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("chaos.go")
	require.NoError(t,

		err)

	source := string(data)
	for _, forbidden := range []string{`"pkill"`, `"killall"`, `"pgrep"`} {
		require.False(t, strings.Contains(source,
			forbidden,
		))

	}
}

func TestChaosHarness_RunEventsPressureUsesCurrentSchema(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("chaos.go")
	require.NoError(t,

		err)

	source := string(data)
	require.False(t, strings.Contains(source,
		"event_type",
	),
	)
	require.False(t, strings.Contains(source,
		"run_events (id, run_id, project_id",
	))
	require.False(t, strings.Contains(source,
		"job_runs (id, job_id, project_id, status, payload, triggered_by, created_at, updated_at)",
	))

	for _, required := range []string{"run_events (id, run_id, type, level, message, data, created_at)", "'loadtest_pressure'"} {
		require.True(t, strings.Contains(source,
			required,
		))

	}
	for _, required := range []string{
		"job_runs (id, job_id, project_id, status, payload, triggered_by, created_at)",
		"'loadtest-clock-skew-'",
	} {
		require.True(t, strings.Contains(source,
			required,
		))

	}
}

func TestChaosHarness_DiskPressureCleanupIsRunScoped(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("chaos.go")
	require.NoError(t,

		err)

	source := string(data)
	for _, required := range []string{
		"DELETE FROM run_events re",
		"USING job_runs jr",
		"re.run_id = jr.id",
		"jr.project_id = $1",
		"AND re.run_id = $2",
		"DELETE FROM job_runs",
		"WHERE id = $2",
	} {
		require.True(t, strings.Contains(source,
			required,
		))

	}
}

func TestChaosHarness_CleanupUsesDetachedContext(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("chaos.go")
	require.NoError(t,

		err)

	source := string(data)
	require.True(t, strings.Contains(source,
		"func chaosCleanupContext() (context.Context, context.CancelFunc)",
	))

	for _, required := range []string{
		"exec.CommandContext(cleanupCtx, \"docker\", \"start\", container)",
		"exec.CommandContext(cleanupCtx, \"docker\", \"unpause\", container)",
		"ce.harness.Pool.Exec(cleanupCtx, `",
		"exec.CommandContext(cleanupCtx, \"docker\", \"start\", redisContainer)",
		"exec.CommandContext(cleanupCtx, \"docker\", \"start\", straitContainer)",
	} {
		require.True(t, strings.Contains(source,
			required,
		))

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
	require.NoError(t,

		err)
	require.Equal(t, "strait-postgres",

		got,
	)

}

func TestFindContainer_FailsClosedWhenOnlySubstringMatches(t *testing.T) {
	restore := stubDockerContainerNames(t, []string{
		"customer-postgres",
		"strait-postgres-backup",
		"project-redis",
	})
	defer restore()

	got, err := findContainer("postgres")
	require.Error(t, err)
	require.Empty(t, got)
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
	require.NoError(t,

		err)
	require.Equal(t, "strait-pr-147-redis",

		got)

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

	_, err := findContainer("postgres")
	require.ErrorIs(t, err, want)
}

func TestChaosCascadingFailure_FailsClosedWhenRedisMissing(t *testing.T) {
	restore := stubDockerContainerNames(t, []string{"strait"})
	defer restore()

	ce := &ChaosEngine{}
	err := ce.chaosCascadingFailure(t.Context())
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "finding redis container"))

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
	require.Error(t, err)
	require.NotEqual(t,

		0, attempts,
	)
	require.EqualValues(t, 0,

		successes,
	)
	require.NotEqual(t,

		0, ce.errorCount.
			Load())

}

func TestTrafficSpikeRequiresAtLeastOneAttempt(t *testing.T) {
	t.Parallel()

	ce := &ChaosEngine{loadRate: 1}
	attempts, successes, err := ce.runTrafficSpike(t.Context(), time.Nanosecond, time.Hour)
	require.Error(t, err)
	require.False(t, attempts !=
		0 || successes !=
		0)

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
	require.NoError(t,

		err)
	require.False(t, attempts ==
		0 || successes ==
		0)
	require.Equal(t, successes,

		ce.triggerCount.
			Load())

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
