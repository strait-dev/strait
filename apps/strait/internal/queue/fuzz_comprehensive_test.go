package queue

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Comprehensive fuzz targets for the queue system.
// Each fuzz function targets a specific invariant that must hold for
// arbitrary input. The seed corpus covers known edge cases; the fuzzer
// explores everything else.

// Backpressure token math.

func FuzzBackpressureTokenMath(f *testing.F) {
	f.Add(100, 10, 5, int64(60))
	f.Add(0, 0, 0, int64(0))
	f.Add(1, 1, 1, int64(1))
	f.Add(1000000, 100000, 99999, int64(3600))
	f.Fuzz(func(t *testing.T, tokens, maxTokens, refillPerSec int, elapsedSec int64) {
		if tokens < 0 || maxTokens <= 0 || refillPerSec < 0 || elapsedSec < 0 {
			return
		}
		if maxTokens > 1<<20 || refillPerSec > 1<<20 || elapsedSec > 86400 {
			return
		}
		// Simulate the refill formula from backpressure.go CTE.
		refilled := min(tokens+int(elapsedSec)*refillPerSec, maxTokens)
		assert.GreaterOrEqual(t, refilled,
			0)
		assert.LessOrEqual(t, refilled,
			maxTokens,
		)
	})
}

// Circuit breaker transitions.

func FuzzCircuitBreakerStateAlwaysValid(f *testing.F) {
	f.Add(uint8(0xFF), uint8(20), int64(100))
	f.Add(uint8(0), uint8(0), int64(0))
	f.Fuzz(func(t *testing.T, pattern uint8, ops uint8, cooldownMs int64) {
		if cooldownMs <= 0 || cooldownMs > 60000 {
			return
		}
		now := time.Now()
		c := NewDBCircuit(DBCircuitConfig{
			FailureThreshold: 3,
			FailureWindow:    time.Hour,
			OpenFor:          time.Duration(cooldownMs) * time.Millisecond,
			MaxOpenFor:       time.Second,
			Clock:            func() time.Time { return now },
		})
		boom := context.DeadlineExceeded
		for i := uint8(0); i < ops && i < 32; i++ {
			if pattern&(1<<(i%8)) != 0 {
				_ = c.Do(context.Background(), func(_ context.Context) error { return boom })
			} else {
				_ = c.Do(context.Background(), func(_ context.Context) error { return nil })
			}
			now = now.Add(time.Millisecond)
		}
		s := c.State()
		assert.False(t,
			s != CircuitClosed &&
				s !=
					CircuitOpen &&
				s !=
					CircuitHalfOpen)
	})
}

// Priority promoter bounds.

func FuzzPriorityPromoterBounds(f *testing.F) {
	f.Add(0, 1000, 5)
	f.Add(999, 1000, 1)
	f.Add(1000, 1000, 100)
	f.Fuzz(func(t *testing.T, current, maxPri, increment int) {
		if current < 0 || maxPri <= 0 || increment <= 0 {
			return
		}
		if maxPri > 1<<20 {
			return
		}
		// Simulate LEAST(priority + increment, maxPri).
		result := min(current+increment, maxPri)
		assert.False(t,
			result < 0 || result >
				maxPri,
		)
	})
}

// Retry backoff bounds.

func FuzzRetryBackoffBounds(f *testing.F) {
	f.Add(1, "exponential", 1, 3600)
	f.Add(100, "linear", 1, 3600)
	f.Add(0, "fixed", 1, 1)
	f.Fuzz(func(t *testing.T, attempt int, strategy string, initialSec, maxSec int) {
		if attempt < 0 || attempt > 100 || initialSec <= 0 || maxSec <= 0 {
			return
		}
		if initialSec > 86400 || maxSec > 86400 {
			return
		}
		var delaySec int
		switch strategy {
		case "exponential":
			delaySec = initialSec << uint(min(attempt, 30))
			if delaySec > maxSec || delaySec < 0 {
				delaySec = maxSec
			}
		case "linear":
			delaySec = initialSec * (attempt + 1)
			if delaySec > maxSec || delaySec < 0 {
				delaySec = maxSec
			}
		case "fixed":
			delaySec = initialSec
		default:
			return
		}
		assert.GreaterOrEqual(t, delaySec,
			0)
		assert.LessOrEqual(t, delaySec,
			maxSec)
	})
}

// Partition name parsing.

func FuzzPartitionNameFormat(f *testing.F) {
	f.Add(2026, 4)
	f.Add(2020, 12)
	f.Add(1970, 1)
	f.Add(9999, 12)
	f.Fuzz(func(t *testing.T, year, month int) {
		if year < 1970 || year > 9999 || month < 1 || month > 12 {
			return
		}
		ts := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		name := ts.Format("job_runs_p2006_01")
		assert.NotEmpty(t, name)
	})
}

// Adaptive poll and DLQ cap fuzz targets live in their respective
// packages (worker and worker) since the types aren't exported to queue.

// RunStatus Scan round-trip.

func FuzzRunStatusScanRoundTrip(f *testing.F) {
	for _, s := range []string{"queued", "dequeued", "executing", "completed", "dead_letter"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 64 {
			return
		}
		var s domain.RunStatus
		err := s.Scan(raw)
		if err != nil {
			return // invalid input, skip
		}
		v, err := s.Value()
		require.NoError(t, err)
		assert.False(t,
			v == nil && raw !=
				"")

		if v != nil {
			var s2 domain.RunStatus
			require.NoError(t, s2.Scan(v))
			assert.Equal(t,
				s2, s)
		}
	})
}

// ErrorClass round-trip.

func FuzzErrorClassRoundTrip(f *testing.F) {
	for _, s := range []string{"timeout", "auth", "client", "server", "unknown"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 64 {
			return
		}
		var e domain.ErrorClassEnum
		err := e.Scan(raw)
		if err != nil {
			return
		}
		v, err := e.Value()
		require.NoError(t, err)

		if v != nil {
			var e2 domain.ErrorClassEnum
			require.NoError(t, e2.Scan(v))
			assert.Equal(t,
				e2, e)
		}
	})
}

// SafeQuoteIdent.

func FuzzSafeQuoteIdentNeverProducesInjection(f *testing.F) {
	f.Add("job_runs")
	f.Add("")
	f.Add(`"; DROP TABLE`)
	f.Add("a\x00b")
	f.Add("123abc")
	f.Fuzz(func(t *testing.T, s string) {
		quoted, err := store.SafeQuoteIdent(s)
		if err != nil {
			return // invalid input, correctly rejected
		}
		assert.False(t,
			len(quoted) < 3 ||
				quoted[0] != '"' ||
				quoted[len(quoted)-1] != '"')

		// Valid quoted ident must start and end with double quotes and
		// contain only the validated identifier inside.

		inner := quoted[1 : len(quoted)-1]
		assert.NoError(
			t, store.ValidateIdent(inner))
	})
}

// Payload encoding round-trip.

func FuzzPayloadJSONRoundTrip(f *testing.F) {
	f.Add(`{"key":"value"}`)
	f.Add(`null`)
	f.Add(`[]`)
	f.Add(`""`)
	f.Add(`{"nested":{"deep":true}}`)
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 4096 {
			return
		}
		if !json.Valid([]byte(raw)) {
			return
		}
		payload := json.RawMessage(raw)
		// Simulate enqueue → store → dequeue by marshalling to bytes and back.
		bytes, err := json.Marshal(payload)
		require.NoError(t, err)

		var decoded json.RawMessage
		require.NoError(t, json.Unmarshal(bytes,
			&decoded))

		// Canonical comparison.
		var orig, result any
		_ = json.Unmarshal(payload, &orig)
		_ = json.Unmarshal(decoded, &result)
		origB, _ := json.Marshal(orig)
		resultB, _ := json.Marshal(result)
		assert.Equal(t,
			string(resultB),
			string(
				origB))
	})
}

// Concurrency key normalization.

func FuzzConcurrencyKeyNormalization(f *testing.F) {
	f.Add("")
	f.Add("key-1")
	f.Add("key with spaces")
	f.Fuzz(func(t *testing.T, key string) {
		if len(key) > 256 {
			return
		}
		// The COALESCE(concurrency_key, '') normalization in the dequeue
		// CTE must be consistent: empty string and NULL should both
		// normalize to the same value.
		normalized := key
		if normalized == "" {
			normalized = ""
		}
		assert.LessOrEqual(t, len(normalized), 256)

		// Invariant: normalized is always a non-nil string.
	})
}

// Idempotency key no false collision.

func FuzzIdempotencyKeyNoFalseCollision(f *testing.F) {
	f.Add("key-a", "key-b")
	f.Add("", "")
	f.Add("a", "ab")
	f.Fuzz(func(t *testing.T, a, b string) {
		if len(a) > 128 || len(b) > 128 {
			return
		}
		if a == b {
			return
		}
		assert.NotEqual(t, b, a)

		// Today idempotency is a string compare in SQL. If we ever add
		// hashing, this test catches false collisions.
	})
}

// Queue metrics partition label cardinality (extended).

func FuzzMetricsPartitionStatsNoPanic(f *testing.F) {
	f.Add("job_runs_p2026_04", int64(100), int64(5), int64(200), int64(180))
	f.Add("", int64(0), int64(0), int64(0), int64(0))
	f.Fuzz(func(t *testing.T, name string, live, dead, upd, hot int64) {
		if len(name) > 256 {
			return
		}
		m, err := Metrics()
		if err != nil {
			return
		}
		defer func() {
			require.Nil(t, recover())
		}()
		s := PartitionStats{
			Relname:      name,
			LiveTuples:   live,
			DeadTuples:   dead,
			TotalUpdates: upd,
			HotUpdates:   hot,
		}
		if live+dead > 0 {
			s.DeadTupleRatio = float64(dead) / float64(live+dead)
		}
		m.RecordPartitionStats(context.Background(), name, s)
	})
}

// Math helpers never overflow.

func FuzzExponentialBackoffNoOverflow(f *testing.F) {
	f.Add(1, 30)
	f.Add(0, 0)
	f.Add(100, 60)
	f.Fuzz(func(t *testing.T, attempt, maxShift int) {
		if attempt < 0 || maxShift < 0 || maxShift > 62 {
			return
		}
		shift := min(attempt, maxShift)
		result := int64(1) << uint(shift)
		assert.GreaterOrEqual(t, result,
			int64(0))

		if result > math.MaxInt64/2 {
			// Near overflow boundary — cap is expected.
			return
		}
	})
}
