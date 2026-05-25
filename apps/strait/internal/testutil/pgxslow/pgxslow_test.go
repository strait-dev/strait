package pgxslow

import (
	"context"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sourcegraph/conc"
)

func TestTracer_UnconditionalDelay(t *testing.T) {
	tr := New(Rule{Delay: 20 * time.Millisecond})
	start := time.Now()
	_ = tr.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "select 1"})
	elapsed := time.Since(start)
	if elapsed < 15*time.Millisecond {
		t.Fatalf("expected delay, got %v", elapsed)
	}
	if tr.InjectedCount() != 1 || tr.TotalCount() != 1 {
		t.Fatalf("counters: injected=%d total=%d", tr.InjectedCount(), tr.TotalCount())
	}
}

func TestTracer_PatternMatch(t *testing.T) {
	re := regexp.MustCompile(`(?i)update\s+job_runs`)
	tr := New(Rule{Pattern: re, Delay: 10 * time.Millisecond})

	// Non-matching query: no delay.
	start := time.Now()
	_ = tr.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "select 1"})
	if d := time.Since(start); d > 5*time.Millisecond {
		t.Fatalf("non-match should not delay, got %v", d)
	}

	// Matching query: delay applied.
	start = time.Now()
	_ = tr.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "UPDATE job_runs SET x=1"})
	if d := time.Since(start); d < 5*time.Millisecond {
		t.Fatalf("match should delay, got %v", d)
	}

	if tr.InjectedCount() != 1 {
		t.Fatalf("injected=%d want 1", tr.InjectedCount())
	}
	if tr.TotalCount() != 2 {
		t.Fatalf("total=%d want 2", tr.TotalCount())
	}
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
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Fatalf("expected early return on cancel, got %v", d)
	}
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
	if tr.TotalCount() != 800 {
		t.Fatalf("total=%d want 800", tr.TotalCount())
	}
}

func TestTracer_TraceQueryEndNoOp(t *testing.T) {
	tr := New()
	tr.TraceQueryEnd(context.Background(), nil, pgx.TraceQueryEndData{})
}

// Ensure Tracer satisfies pgx.QueryTracer.
var _ pgx.QueryTracer = (*Tracer)(nil)
