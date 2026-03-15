package worker

import (
	"context"
	"time"

	"strait/internal/domain"
)

// ExecutionContext carries run/job metadata through the middleware chain.
type ExecutionContext struct {
	Run   *domain.JobRun
	Job   *domain.Job
	Start time.Time
}

// ExecutionHandler processes a job run within a middleware chain.
type ExecutionHandler func(ctx context.Context, ec *ExecutionContext)

// ExecutionMiddleware wraps an ExecutionHandler to add cross-cutting behavior.
type ExecutionMiddleware func(next ExecutionHandler) ExecutionHandler

// Chain composes middlewares into a single middleware, applied left-to-right.
func Chain(middlewares ...ExecutionMiddleware) ExecutionMiddleware {
	return func(next ExecutionHandler) ExecutionHandler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}
