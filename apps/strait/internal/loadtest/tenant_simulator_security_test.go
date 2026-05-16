//go:build loadtest

package loadtest

import (
	"context"
	"math"
	"testing"
	"time"
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

	if math.Abs(trough-0.2) > 0.0001 {
		t.Fatalf("configured trough multiplier = %.4f, want 0.2", trough)
	}
	if legacySymmetricTrough <= trough {
		t.Fatalf("02:00 multiplier = %.4f, should be higher than configured 04:00 trough %.4f", legacySymmetricTrough, trough)
	}
	if math.Abs(peak-1.0) > 0.0001 {
		t.Fatalf("configured peak multiplier = %.4f, want 1.0", peak)
	}
}
