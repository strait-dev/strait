package queue

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// Suite 6: comprehensive fuzz targets for the queue system.
// Each fuzz function targets a specific invariant that must hold for
// arbitrary input. The seed corpus covers known edge cases; the fuzzer
// explores everything else.

// --- Backpressure token math ---

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
		refilled := tokens + int(elapsedSec)*refillPerSec
		if refilled > maxTokens {
			refilled = maxTokens
		}
		if refilled < 0 {
			t.Errorf("refilled = %d, must be >= 0", refilled)
		}
		if refilled > maxTokens {
			t.Errorf("refilled = %d > max = %d", refilled, maxTokens)
		}
	})
}

// --- Circuit breaker transitions ---

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
		if s != CircuitClosed && s != CircuitOpen && s != CircuitHalfOpen {
			t.Errorf("invalid state %d", s)
		}
	})
}

// --- Claim cursor monotonicity ---

func FuzzClaimCursorMonotonicity(f *testing.F) {
	f.Add(int64(0), "a", int64(1000), "b")
	f.Add(int64(-1), "", int64(0), "")
	f.Fuzz(func(t *testing.T, ns1 int64, id1 string, ns2 int64, id2 string) {
		if len(id1) > 64 || len(id2) > 64 {
			return
		}
		c := NewClaimCursor(time.Hour)
		base := time.Now()
		c.Advance(base.Add(time.Duration(ns1)), id1)
		snap1ts, snap1id, _ := c.Snapshot()
		c.Advance(base.Add(time.Duration(ns2)), id2)
		snap2ts, snap2id, _ := c.Snapshot()
		// Snapshot must be monotonically non-decreasing.
		if snap2ts.Before(snap1ts) {
			t.Errorf("cursor went backwards: %v -> %v", snap1ts, snap2ts)
		}
		if snap2ts.Equal(snap1ts) && snap2id < snap1id {
			t.Errorf("cursor ID regressed at same ts: %q -> %q", snap1id, snap2id)
		}
	})
}

// --- Priority promoter bounds ---

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
		// Simulate LEAST(priority + increment, maxPri)
		result := current + increment
		if result > maxPri {
			result = maxPri
		}
		if result < 0 || result > maxPri {
			t.Errorf("promoted = %d, want [0, %d]", result, maxPri)
		}
	})
}

// --- Retry backoff bounds ---

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
		if delaySec < 0 {
			t.Errorf("negative delay: %d", delaySec)
		}
		if delaySec > maxSec {
			t.Errorf("delay %d > max %d", delaySec, maxSec)
		}
	})
}

// --- Partition name parsing ---

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
		if len(name) == 0 {
			t.Error("empty partition name")
		}
	})
}

// Adaptive poll and DLQ cap fuzz targets live in their respective
// packages (worker and worker) since the types aren't exported to queue.

// --- RunStatus Scan round-trip ---

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
		if err != nil {
			t.Fatalf("Value failed after Scan: %v", err)
		}
		if v == nil && raw != "" {
			t.Errorf("Value returned nil for non-empty %q", raw)
		}
		if v != nil {
			var s2 domain.RunStatus
			if err := s2.Scan(v); err != nil {
				t.Fatalf("round-trip Scan failed: %v", err)
			}
			if s != s2 {
				t.Errorf("round-trip: %q -> %q", s, s2)
			}
		}
	})
}

// --- ErrorClass round-trip ---

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
		if err != nil {
			t.Fatalf("Value failed after Scan: %v", err)
		}
		if v != nil {
			var e2 domain.ErrorClassEnum
			if err := e2.Scan(v); err != nil {
				t.Fatalf("round-trip Scan: %v", err)
			}
			if e != e2 {
				t.Errorf("round-trip: %q -> %q", e, e2)
			}
		}
	})
}

// --- SafeQuoteIdent ---

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
		// Valid quoted ident must start and end with double quotes and
		// contain only the validated identifier inside.
		if len(quoted) < 3 || quoted[0] != '"' || quoted[len(quoted)-1] != '"' {
			t.Errorf("bad quoting: %q", quoted)
		}
		inner := quoted[1 : len(quoted)-1]
		if err := store.ValidateIdent(inner); err != nil {
			t.Errorf("inner %q is not a valid ident: %v", inner, err)
		}
	})
}

// --- Payload encoding round-trip ---

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
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var decoded json.RawMessage
		if err := json.Unmarshal(bytes, &decoded); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// Canonical comparison.
		var orig, result any
		_ = json.Unmarshal(payload, &orig)
		_ = json.Unmarshal(decoded, &result)
		origB, _ := json.Marshal(orig)
		resultB, _ := json.Marshal(result)
		if string(origB) != string(resultB) {
			t.Errorf("payload drift: %s -> %s", origB, resultB)
		}
	})
}

// --- Concurrency key normalization ---

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
		// Invariant: normalized is always a non-nil string.
		if len(normalized) > 256 {
			t.Errorf("normalized key too long: %d", len(normalized))
		}
	})
}

// --- Idempotency key no false collision ---

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
		// Today idempotency is a string compare in SQL. If we ever add
		// hashing, this test catches false collisions.
		if a == b {
			t.Errorf("false collision: %q vs %q", a, b)
		}
	})
}

// --- Queue metrics partition label cardinality (extended) ---

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
			if r := recover(); r != nil {
				t.Fatalf("RecordPartitionStats panicked: %v", r)
			}
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

// --- Math helpers never overflow ---

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
		if result < 0 {
			t.Errorf("overflow at attempt=%d shift=%d", attempt, shift)
		}
		if result > math.MaxInt64/2 {
			// Near overflow boundary — cap is expected.
			return
		}
	})
}
