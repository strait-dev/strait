package workflow

import (
	"strconv"
	"time"

	otter "github.com/maypok86/otter/v2"

	"strait/internal/domain"
)

// stepDefCacheTTL bounds how long a memoized step-definition set lives. The
// definitions are immutable for a given snapshot ID / (workflow, version) pair,
// so the TTL exists only to cap memory for runs that will never be touched
// again, not to chase freshness.
const stepDefCacheTTL = 10 * time.Minute

// stepDefCache memoizes resolved workflow step definitions on the callback hot
// path. loadStepDefinitions runs on every step/job-run terminal callback and
// otherwise re-reads the run's definitions each time: a workflow_snapshots row
// plus a JSON parse, or a live workflow_version_steps read. Those definitions
// never change for a given snapshot ID / (workflow, version) pair, so memoizing
// them removes one query (and the snapshot JSON parse) per callback. Backed by
// otter (W-TinyLFU) for a bounded, concurrency-safe store.
type stepDefCache struct {
	inner    *otter.Cache[string, []domain.WorkflowStep]
	disabled bool
}

// newStepDefCache builds a step-definition cache. A ttl <= 0 disables caching
// (every lookup misses), which keeps tests that want to observe every load able
// to opt out without special-casing.
func newStepDefCache(ttl time.Duration) *stepDefCache {
	if ttl <= 0 {
		return &stepDefCache{disabled: true}
	}
	opts := &otter.Options[string, []domain.WorkflowStep]{
		MaximumSize:      10_000,
		ExpiryCalculator: otter.ExpiryWriting[string, []domain.WorkflowStep](ttl),
	}
	return &stepDefCache{inner: otter.Must(opts)}
}

// stepDefCacheKey derives a stable key for a run's step definitions. A run
// either resolves from an immutable snapshot or from its immutable workflow
// version, so those two namespaces never collide.
func stepDefCacheKey(wfRun *domain.WorkflowRun) string {
	if wfRun.WorkflowSnapshotID != "" {
		return "snap\x00" + wfRun.WorkflowSnapshotID
	}
	return "ver\x00" + wfRun.WorkflowID + "\x00" + strconv.Itoa(wfRun.WorkflowVersion)
}

func (c *stepDefCache) get(key string) ([]domain.WorkflowStep, bool) {
	if c == nil || c.disabled {
		return nil, false
	}
	return c.inner.GetIfPresent(key)
}

func (c *stepDefCache) set(key string, steps []domain.WorkflowStep) {
	if c == nil || c.disabled {
		return
	}
	c.inner.Set(key, steps)
}
