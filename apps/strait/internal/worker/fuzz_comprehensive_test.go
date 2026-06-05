package worker

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
)

// Fuzz targets that live in the worker package because they
// test types not exported to queue.

func FuzzAdaptivePollBounds(f *testing.F) {
	f.Add(int64(5e9), int64(2e8), int64(30e9), 0, int64(0))
	f.Add(int64(1e9), int64(1e8), int64(10e9), 10, int64(10000))
	f.Fuzz(func(t *testing.T, baseNs, minNs, maxNs int64, empties int, depth int64) {
		if baseNs <= 0 || minNs <= 0 || maxNs <= 0 || empties < 0 || depth < 0 {
			return
		}
		if baseNs > int64(time.Hour) || minNs > int64(time.Hour) || maxNs > int64(time.Hour) {
			return
		}
		if empties > 30 {
			return
		}
		a := NewAdaptivePollInterval(
			time.Duration(baseNs),
			time.Duration(minNs),
			time.Duration(maxNs),
		)
		for range empties {
			a.ObserveEmpty()
		}
		a.ObserveDepth(depth)
		d := a.Next()
		minD := time.Duration(minNs)
		maxD := time.Duration(maxNs)
		if minD > maxD {
			minD = maxD
		}
		assert.False(t,
			d < minD ||
				d > maxD,
		)
	})
}

func FuzzDLQCapInvariant(f *testing.F) {
	f.Add(5, 10, uint8(0b10110101))
	f.Add(0, 0, uint8(0))
	f.Fuzz(func(t *testing.T, perJob, perProject int, ops uint8) {
		if perJob < 0 || perProject < 0 || perJob > 1000 || perProject > 1000 {
			return
		}
		s := newFakeDLQStore()
		cfg := DLQCapConfig{MaxPerJob: perJob, MaxPerProject: perProject, Policy: DLQOverflowReject}
		e := NewDLQCapEnforcer(s, cfg, nil)
		var depth int
		for i := range uint8(8) {
			if ops&(1<<i) != 0 {
				proceed, _ := e.EnforceBeforeTransition(context.Background(), "p", "j")
				if proceed {
					depth++
					s.perJob["p:j"] = depth
					s.perProject["p"] = depth
				}
			} else if depth > 0 {
				depth--
				s.perJob["p:j"] = depth
				s.perProject["p"] = depth
			}
		}
		assert.False(t,
			perJob > 0 &&
				depth >
					perJob)
		assert.False(t,
			perProject >
				0 && depth >
				perProject)
	})
}

func FuzzRetryBackoffNeverNegative(f *testing.F) {
	f.Add(1, 1, 3600)
	f.Add(0, 1, 1)
	f.Add(30, 5, 3600)
	f.Fuzz(func(t *testing.T, attempt, initialSec, maxSec int) {
		if attempt < 0 || attempt > 100 || initialSec <= 0 || maxSec <= 0 {
			return
		}
		if initialSec > 86400 || maxSec > 86400 {
			return
		}
		delay := NextRetryDelayWithPolicy(attempt, domain.RetryBackoffExponential, initialSec, maxSec)
		assert.GreaterOrEqual(t, delay,
			time.Duration(0))
	})
}
