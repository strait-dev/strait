// Package cache holds shared cache primitives used across the codebase.
package cache

import (
	"math/rand/v2"
	"time"
)

// JitterTTL returns base plus a uniformly random extra duration in
// [0, base*fraction). Use it to spread synchronized cache populations
// across a window so they do not expire simultaneously and trigger a
// thundering herd on the upstream source.
//
// fraction must be in (0, 1]; values outside that range or a non-positive
// base return base unchanged. Backed by math/rand/v2 to avoid global
// rand contention.
func JitterTTL(base time.Duration, fraction float64) time.Duration {
	if base <= 0 || fraction <= 0 || fraction > 1 {
		return base
	}
	maxExtra := int64(float64(base) * fraction)
	if maxExtra <= 0 {
		return base
	}
	// G404: non-crypto randomness is intentional — TTL jitter for cache stampede mitigation.
	return base + time.Duration(rand.Int64N(maxExtra)) //nolint:gosec
}
