package compute

import (
	"math"
	"testing"
)

// TestPricing_OverflowDuration verifies that CalculateCost does not panic
// when given MaxFloat64 as the duration, which could cause overflow in
// the cost multiplication.
func TestPricing_OverflowDuration(t *testing.T) {
	t.Parallel()

	cost, err := CalculateCost("large-2x", math.MaxFloat64)
	if err != nil {
		t.Fatalf("CalculateCost() error = %v", err)
	}

	// float64 overflow produces +Inf; int64(+Inf) is implementation-defined.
	// The key assertion is that it does not panic. The cost clamp to 0 on
	// negative catches the int64 wrap-around on some platforms.
	t.Logf("cost for MaxFloat64 duration: %d", cost)
}

// TestPricing_SubSecondDuration verifies that a very small duration
// produces a zero or near-zero cost without error.
func TestPricing_SubSecondDuration(t *testing.T) {
	t.Parallel()

	// micro preset: 17 * 0.001 = 0.017, rounds to 0.
	cost, err := CalculateCost("micro", 0.001)
	if err != nil {
		t.Fatalf("CalculateCost() error = %v", err)
	}
	if cost != 0 {
		t.Errorf("cost = %d, want 0 for 0.001s on micro preset", cost)
	}
}

// TestPresetFromName_EmptyName verifies that an empty preset name returns
// an error.
func TestPresetFromName_EmptyName(t *testing.T) {
	t.Parallel()

	_, err := PresetFromName("")
	if err == nil {
		t.Fatal("expected error for empty preset name")
	}
}

// TestPresetFromName_SQLInjection verifies that a SQL-injection-style
// preset name is treated as an unknown preset and returns an error.
func TestPresetFromName_SQLInjection(t *testing.T) {
	t.Parallel()

	_, err := PresetFromName("standard'; DROP TABLE presets;--")
	if err == nil {
		t.Fatal("expected error for SQL injection preset name")
	}
}

// TestPricing_NegativeDuration verifies that negative durations are
// clamped to zero cost.
func TestPricing_NegativeDuration(t *testing.T) {
	t.Parallel()

	presets := []string{"micro", "small-1x", "medium-1x", "large-2x"}
	durations := []float64{-1, -0.001, -math.MaxFloat64, math.Copysign(0, -1)}

	for _, preset := range presets {
		for _, dur := range durations {
			cost, err := CalculateCost(preset, dur)
			if err != nil {
				t.Errorf("CalculateCost(%q, %g) error = %v", preset, dur, err)
				continue
			}
			if cost != 0 {
				t.Errorf("CalculateCost(%q, %g) = %d, want 0", preset, dur, cost)
			}
		}
	}
}

// FuzzCalculateCostOverflow fuzzes preset names and durations to check
// for panics in the pricing calculation.
func FuzzCalculateCostOverflow(f *testing.F) {
	f.Add("micro", 1.0)
	f.Add("large-2x", 3600.0)
	f.Add("unknown", 0.0)
	f.Add("", -1.0)
	f.Add("micro", math.MaxFloat64)
	f.Add("small-1x", math.SmallestNonzeroFloat64)
	f.Add("medium-1x", math.Inf(1))
	f.Add("large-1x", math.NaN())

	f.Fuzz(func(t *testing.T, preset string, duration float64) {
		// Must not panic.
		_, _ = CalculateCost(preset, duration)
	})
}

// FuzzContainerSpecParsing fuzzes preset name lookups and region
// validation to check for panics.
func FuzzContainerSpecParsing(f *testing.F) {
	f.Add("micro", "iad")
	f.Add("", "")
	f.Add("large-2x", "us-east")
	f.Add("unknown-preset", "zzz")
	f.Add("standard'; DROP--", "\x00")

	f.Fuzz(func(t *testing.T, presetName, region string) {
		// Must not panic.
		_, _ = PresetFromName(presetName)
		_ = IsValidRegion(region)
		_ = NearestFlyRegion(region)
		_ = RegionFallbackChain(region)
		_ = PresetMemoryMB(presetName)
		_ = PresetIndex(presetName)
		_, _ = NextPreset(presetName)
		_ = IsMaxPreset(presetName)
		_ = RegionLabel(region)
	})
}
