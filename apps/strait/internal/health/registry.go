package health

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sourcegraph/conc"
)

type Status string

const (
	StatusUp       Status = "up"
	StatusDown     Status = "down"
	StatusDegraded Status = "degraded"
)

type ComponentResult struct {
	Name      string        `json:"name"`
	Status    Status        `json:"status"`
	Latency   time.Duration `json:"-"`
	LatencyMs int64         `json:"latency_ms"`
	Error     string        `json:"error,omitempty"`
}

type CheckResult struct {
	Status     Status            `json:"status"`
	Components []ComponentResult `json:"components"`
}

type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

type Registry struct {
	mu       sync.Mutex
	checkers atomic.Pointer[[]Checker]
}

func NewRegistry() *Registry {
	r := &Registry{}
	r.storeCheckers(nil)
	return r
}

func (r *Registry) Register(c Checker) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current := r.loadCheckers()
	next := make([]Checker, len(current)+1)
	copy(next, current)
	next[len(current)] = c
	r.storeCheckers(next)
}

func (r *Registry) CheckAll(ctx context.Context) CheckResult {
	checkers := r.loadCheckers()

	results := make([]ComponentResult, len(checkers))
	var wg conc.WaitGroup

	for i, c := range checkers {
		idx, checker := i, c
		wg.Go(func() {
			start := time.Now()
			err := checker.Check(ctx)
			latency := time.Since(start)

			cr := ComponentResult{
				Name:      checker.Name(),
				Latency:   latency,
				LatencyMs: latency.Milliseconds(),
			}
			if err != nil {
				cr.Status = StatusDown
				cr.Error = err.Error()
			} else {
				cr.Status = StatusUp
			}
			results[idx] = cr
		})
	}

	wg.Wait()

	overall := StatusUp
	for i, cr := range results {
		if cr.Status == StatusDown {
			if IsCritical(checkers[i]) {
				overall = StatusDown
				break
			}
			if overall != StatusDown {
				overall = StatusDegraded
			}
		}
	}

	return CheckResult{
		Status:     overall,
		Components: results,
	}
}

func (r *Registry) loadCheckers() []Checker {
	checkers := r.checkers.Load()
	if checkers == nil {
		return nil
	}
	return *checkers
}

func (r *Registry) storeCheckers(checkers []Checker) {
	r.checkers.Store(&checkers)
}

type CheckerFunc struct {
	name    string
	checkFn func(ctx context.Context) error
}

func NewChecker(name string, fn func(ctx context.Context) error) Checker {
	return &CheckerFunc{name: name, checkFn: fn}
}

func (c *CheckerFunc) Name() string                    { return c.name }
func (c *CheckerFunc) Check(ctx context.Context) error { return c.checkFn(ctx) }
