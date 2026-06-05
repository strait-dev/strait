package pgxslow

import (
	"context"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
)

func TestTracer_UnconditionalDelay(t *testing.T) {
	tr := New(Rule{Delay: 20 * time.Millisecond})
	start := time.Now()
	_ = tr.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "select 1"})
	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 15*time.Millisecond)
	assert.Equal(t, int64(1), tr.InjectedCount())
	assert.Equal(t, int64(1), tr.TotalCount())
}

func TestTracer_PatternMatch(t *testing.T) {
	re := regexp.MustCompile(`(?i)update\s+job_runs`)
	tr := New(Rule{Pattern: re, Delay: 10 * time.Millisecond})

	// Non-matching query: no delay.
	start := time.Now()
	_ = tr.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "select 1"})
	assert.LessOrEqual(t, time.Since(start), 5*time.Millisecond)

	// Matching query: delay applied.
	start = time.Now()
	_ = tr.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "UPDATE job_runs SET x=1"})
	assert.GreaterOrEqual(t, time.Since(start), 5*time.Millisecond)

	assert.Equal(t, int64(1), tr.InjectedCount())
	assert.Equal(t, int64(2), tr.TotalCount())
}

func TestTracer_ContextCancelShortCircuits(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	tr := New(Rule{Delay: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	concWG.Go(func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	})
	start := time.Now()
	_ = tr.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "x"})
	assert.LessOrEqual(t, time.Since(start), 200*time.Millisecond)
}

func TestTracer_SetRulesConcurrent(t *testing.T) {
	tr := New()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				tr.SetRules([]Rule{{Delay: time.Microsecond}})
				_ = tr.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "q"})
			}
		})
	}
	wg.Wait()
	assert.Equal(t, int64(800), tr.TotalCount())
}

func TestTracer_TraceQueryEndNoOp(t *testing.T) {
	tr := New()
	tr.TraceQueryEnd(context.Background(), nil, pgx.TraceQueryEndData{})
}

// Ensure Tracer satisfies pgx.QueryTracer.
var _ pgx.QueryTracer = (*Tracer)(nil)
