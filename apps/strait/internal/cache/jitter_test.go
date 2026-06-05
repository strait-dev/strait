package cache

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJitterTTL_BoundedRange(t *testing.T) {
	t.Parallel()

	const (
		base     = 60 * time.Second
		fraction = 0.1
		samples  = 10_000
	)
	maxExclusive := base + time.Duration(float64(base)*fraction)

	for range samples {
		got := JitterTTL(base, fraction)
		require.GreaterOrEqual(
			t, got,
			base,
		)
		require.False(t, got >=
			maxExclusive,
		)

	}
}

func TestJitterTTL_ZeroFraction(t *testing.T) {
	t.Parallel()

	base := 30 * time.Second
	require.Equal(t, base, JitterTTL(base, 0))
}

func TestJitterTTL_NegativeFraction(t *testing.T) {
	t.Parallel()

	base := 30 * time.Second
	require.Equal(t, base, JitterTTL(base, -0.1))
}

func TestJitterTTL_FractionAboveOne(t *testing.T) {
	t.Parallel()

	base := 30 * time.Second
	require.Equal(t, base, JitterTTL(base, 1.5))
}

func TestJitterTTL_NonPositiveBase(t *testing.T) {
	t.Parallel()

	require.Equal(t, time.Duration(0), JitterTTL(0, 0.1))
	require.Equal(t, -time.Second, JitterTTL(-time.Second, 0.1))
}

func TestJitterTTL_FullFraction(t *testing.T) {
	t.Parallel()

	base := 100 * time.Millisecond
	maxExclusive := 2 * base
	for range 1000 {
		got := JitterTTL(base, 1.0)
		require.False(t, got <
			base ||
			got >=
				maxExclusive)

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
	require.LessOrEqual(t,
		math.Abs(mean-
			expectedMean), expectedMean*0.1)
	require.LessOrEqual(t,
		math.Abs(stddev-
			expectedStddev), expectedStddev*0.15)

	// Allow generous tolerance to keep the test stable while still catching
	// degenerate distributions (e.g. constant or extreme skew).

}
