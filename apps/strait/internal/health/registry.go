package health

import (
	"context"
	"sync"
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
	mu       sync.RWMutex
	checkers []Checker
}

func NewRegistry() *Registry {
	return &Registry{}
}

func (r *Registry) Register(c Checker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkers = append(r.checkers, c)
}

func (r *Registry) CheckAll(ctx context.Context) CheckResult {
	r.mu.RLock()
	checkers := make([]Checker, len(r.checkers))
	copy(checkers, r.checkers)
	r.mu.RUnlock()

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

type CheckerFunc struct {
	name    string
	checkFn func(ctx context.Context) error
}

func NewChecker(name string, fn func(ctx context.Context) error) Checker {
	return &CheckerFunc{name: name, checkFn: fn}
}

func (c *CheckerFunc) Name() string                    { return c.name }
func (c *CheckerFunc) Check(ctx context.Context) error { return c.checkFn(ctx) }
