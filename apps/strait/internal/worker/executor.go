package worker

func (e *Executor) CloseCache() {}

func (e *Executor) tryAcquireBulkheadSlot(jobID string, maxConcurrency int) bool {
	return e.bulkhead.TryAcquire(jobID, maxConcurrency)
}

func (e *Executor) releaseBulkheadSlot(jobID string, maxConcurrency int) {
	e.bulkhead.Release(jobID, maxConcurrency)
}

// Use adds execution middleware to the chain. Must be called before Run().
func (e *Executor) Use(mw ExecutionMiddleware) {
	e.middlewares = append(e.middlewares, mw)
}
