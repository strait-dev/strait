package queue

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Per-project claim-latency histogram with a bounded allow-list.
//
// A raw project_id label on every sample would explode metric
// cardinality. We keep a fixed allow-list of project IDs that are
// important enough to track individually; everything else folds into
// a single "_other" bucket so the label set stays small.

// ProjectLabelAllowlist is a small set of project IDs that should be
// reported individually on per-project metrics. Mutable via Set so a
// feature-flag path can update it at runtime.
type ProjectLabelAllowlist struct {
	mu      sync.RWMutex
	allowed map[string]struct{}
	max     int
}

// NewProjectLabelAllowlist builds an empty allow-list with a hard cap on
// distinct labels (including the "_other" bucket).
func NewProjectLabelAllowlist(maxLabels int) *ProjectLabelAllowlist {
	if maxLabels <= 0 {
		maxLabels = 100
	}
	return &ProjectLabelAllowlist{
		allowed: make(map[string]struct{}),
		max:     maxLabels,
	}
}

// Set replaces the allow-list with the provided project IDs. Anything
// beyond `max-1` is dropped (reserving one slot for the fallback label).
func (p *ProjectLabelAllowlist) Set(ids []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.allowed = make(map[string]struct{}, len(ids))
	limit := p.max - 1
	if limit <= 0 {
		return
	}
	for i, id := range ids {
		if i >= limit {
			break
		}
		p.allowed[id] = struct{}{}
	}
}

// Add inserts a single project ID if there is room. Returns true when
// the project was added (or already present), false if the cap is full.
func (p *ProjectLabelAllowlist) Add(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.allowed[id]; ok {
		return true
	}
	if len(p.allowed)+1 >= p.max {
		return false
	}
	p.allowed[id] = struct{}{}
	return true
}

// Label returns either the project id (if allowed) or "_other".
func (p *ProjectLabelAllowlist) Label(id string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if _, ok := p.allowed[id]; ok {
		return id
	}
	return "_other"
}

// Size returns the current number of allow-listed project IDs (excluding
// the "_other" fallback). Used in tests.
func (p *ProjectLabelAllowlist) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.allowed)
}

// RecordClaimLatencyByProject records a per-project claim latency sample
// under the allow-list's labelling rules.
func (m *QueueMetrics) RecordClaimLatencyByProject(ctx context.Context, allowlist *ProjectLabelAllowlist, projectID string, seconds float64) {
	if m == nil || m.OldestQueuedAge == nil {
		return
	}
	if allowlist == nil {
		m.OldestQueuedAge.Record(ctx, seconds)
		return
	}
	label := allowlist.Label(projectID)
	m.OldestQueuedAge.Record(ctx, seconds, metric.WithAttributes(attribute.String("project", label)))
}
