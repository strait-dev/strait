//go:build loadtest

package loadtest

import (
	"context"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

func TestTenantSimulator_TimeOfDayMultiplierUsesConfiguredTrough(t *testing.T) {
	sim := NewTenantSimulator(TenantSimulatorConfig{
		PeakHourUTC:   14,
		TroughHourUTC: 4,
	}, func(context.Context, TenantProfile, string) error {
		return nil
	})

	trough := sim.timeOfDayMultiplierAt(time.Date(2026, 5, 16, 4, 0, 0, 0, time.UTC))
	legacySymmetricTrough := sim.timeOfDayMultiplierAt(time.Date(2026, 5, 16, 2, 0, 0, 0, time.UTC))
	peak := sim.timeOfDayMultiplierAt(time.Date(2026, 5, 16, 14, 0, 0, 0, time.UTC))
	require.LessOrEqual(t, math.
		Abs(trough-
			0.2),
		0.0001)
	require.False(t, legacySymmetricTrough <=
		trough,
	)
	require.LessOrEqual(t, math.
		Abs(peak-
			1.0), 0.0001,
	)

}

func TestTenantSimulator_RunWaitsForInFlightTriggers(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce atomic.Bool

	sim := NewTenantSimulator(TenantSimulatorConfig{
		Duration: 20 * time.Millisecond,
		Tenants: []TenantProfile{{
			ID:            "tenant-1",
			RunsPerMinute: 60000,
			HTTPPercent:   1,
		}},
	}, func(context.Context, TenantProfile, string) error {
		if startedOnce.CompareAndSwap(false, true) {
			close(started)
		}
		<-release
		return nil
	})

	done := make(chan *TenantSimulatorResult, 1)
	errs := make(chan error, 1)
	concWG.Go(func() {
		result, err := sim.Run(context.Background())
		if err != nil {
			errs <- err
			return
		}
		done <- result
	})

	select {
	case <-started:
	case <-time.After(time.Second):
		require.Fail(t, "expected simulator to start at least one trigger")
	}

	select {
	case result := <-done:
		require.Failf(t, "Run returned before in-flight trigger completed", "%#v", result)
	case err := <-errs:
		require.Failf(t, "Run returned error before in-flight trigger completed", "%v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	select {
	case result := <-done:
		require.NotZero(t, result.TotalRuns)
	case err := <-errs:
		require.Failf(t, "Run returned error", "%v", err)
	case <-time.After(time.Second):
		require.Fail(t, "Run did not return after in-flight trigger completed")
	}
}
