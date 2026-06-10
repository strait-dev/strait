package api

import (
	"context"

	"strait/internal/domain"
	"strait/internal/store"
)

func (s *Server) enqueueTriggerRun(ctx context.Context, tx store.DBTX, run *domain.JobRun) error {
	if tx != nil {
		return s.queue.EnqueueInTx(ctx, tx, run)
	}
	return s.queue.Enqueue(ctx, run)
}
