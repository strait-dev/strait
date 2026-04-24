// Package pgxslow provides a pgx QueryTracer that injects configurable
// latency into matching SQL queries. It is intended for load and chaos
// tests that need to simulate a slow or degraded database.
//
// The tracer honours context cancellation: if the injected sleep would
// outlast the query context, it returns early. It is safe for concurrent
// use.
package pgxslow

import (
	"context"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

// Rule is a single latency injection rule. If Pattern is nil the rule
// matches every query. Delay is the latency applied before the query
// executes.
type Rule struct {
	Pattern *regexp.Regexp
	Delay   time.Duration
}

// Tracer implements pgx.QueryTracer and delays queries that match any
// configured rule. Rules can be mutated at runtime via SetRules.
type Tracer struct {
	mu       sync.RWMutex
	rules    []Rule
	injected atomic.Int64
	total    atomic.Int64
}

// New returns a tracer pre-populated with the given rules.
func New(rules ...Rule) *Tracer {
	t := &Tracer{}
	t.SetRules(rules)
	return t
}

// SetRules replaces the active rule set atomically.
func (t *Tracer) SetRules(rules []Rule) {
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := make([]Rule, len(rules))
	copy(cp, rules)
	t.rules = cp
}

// InjectedCount returns the number of queries that had latency injected.
func (t *Tracer) InjectedCount() int64 { return t.injected.Load() }

// TotalCount returns the total number of queries observed.
func (t *Tracer) TotalCount() int64 { return t.total.Load() }

// TraceQueryStart is part of the pgx.QueryTracer contract. It blocks for
// the configured delay (or until ctx cancels) when a rule matches.
func (t *Tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	t.total.Add(1)
	delay := t.match(data.SQL)
	if delay <= 0 {
		return ctx
	}
	t.injected.Add(1)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
	return ctx
}

// TraceQueryEnd is a no-op but required to satisfy pgx.QueryTracer.
func (t *Tracer) TraceQueryEnd(_ context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {}

func (t *Tracer) match(sql string) time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, r := range t.rules {
		if r.Pattern == nil || r.Pattern.MatchString(sql) {
			return r.Delay
		}
	}
	return 0
}
