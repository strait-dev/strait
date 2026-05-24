package cache

import (
	"math"
	"testing"
	"time"
)

func TestJitterTTL_BoundedRange(t *testing.T) {
	t.Parallel()

	const (
		base     = 60 * time.Second
		fraction = 0.1
		samples  = 10_000
	)
	maxExclusive := base + time.Duration(float64(base)*fraction)

	for i := range samples {
		got := JitterTTL(base, fraction)
		if got < base {
			t.Fatalf("sample %d: got %s, want >= %s", i, got, base)
		}
		if got >= maxExclusive {
			t.Fatalf("sample %d: got %s, want < %s", i, got, maxExclusive)
		}
	}
}

func TestJitterTTL_ZeroFraction(t *testing.T) {
	t.Parallel()

	base := 30 * time.Second
	if got := JitterTTL(base, 0); got != base {
		t.Fatalf("zero fraction: got %s, want %s", got, base)
	}
}

func TestJitterTTL_NegativeFraction(t *testing.T) {
	t.Parallel()

	base := 30 * time.Second
	if got := JitterTTL(base, -0.1); got != base {
		t.Fatalf("negative fraction: got %s, want %s", got, base)
	}
}

func TestJitterTTL_FractionAboveOne(t *testing.T) {
	t.Parallel()

	base := 30 * time.Second
	if got := JitterTTL(base, 1.5); got != base {
		t.Fatalf("fraction > 1: got %s, want %s", got, base)
	}
}

func TestJitterTTL_NonPositiveBase(t *testing.T) {
	t.Parallel()

	if got := JitterTTL(0, 0.1); got != 0 {
		t.Fatalf("zero base: got %s, want 0", got)
	}
	if got := JitterTTL(-time.Second, 0.1); got != -time.Second {
		t.Fatalf("negative base: got %s, want -1s", got)
	}
}

func TestJitterTTL_FullFraction(t *testing.T) {
	t.Parallel()

	base := 100 * time.Millisecond
	maxExclusive := 2 * base
	for i := range 1000 {
		got := JitterTTL(base, 1.0)
		if got < base || got >= maxExclusive {
			t.Fatalf("sample %d: got %s, want in [%s, %s)", i, got, base, maxExclusive)
		}
	}
}

func TestJitterTTL_DistributionSpread(t *testing.T) {
	t.Parallel()

	const (
		base     = time.Second
		fraction = 0.1
		samples  = 10_000
	)
	maxExtra := float64(base) * fraction

	var sum, sumSq float64
	for range samples {
		extra := float64(JitterTTL(base, fraction) - base)
		sum += extra
		sumSq += extra * extra
	}
	mean := sum / samples
	variance := sumSq/samples - mean*mean
	stddev := math.Sqrt(variance)

	// Uniform[0, maxExtra) has expected mean ~maxExtra/2 and stddev ~maxExtra/sqrt(12).
	expectedMean := maxExtra / 2
	expectedStddev := maxExtra / math.Sqrt(12)

	// Allow generous tolerance to keep the test stable while still catching
	// degenerate distributions (e.g. constant or extreme skew).
	if math.Abs(mean-expectedMean) > expectedMean*0.1 {
		t.Fatalf("mean: got %.2f, want ~%.2f (±10%%)", mean, expectedMean)
	}
	if math.Abs(stddev-expectedStddev) > expectedStddev*0.15 {
		t.Fatalf("stddev: got %.2f, want ~%.2f (±15%%)", stddev, expectedStddev)
	}
}
