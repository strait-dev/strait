package queue

import (
	"context"

	"strait/internal/domain"
)

type Queue interface {
	Enqueue(ctx context.Context, run *domain.JobRun) error
	EnqueueBatch(ctx context.Context, runs []*domain.JobRun) (int64, error)
	Dequeue(ctx context.Context) (*domain.JobRun, error)
	DequeueN(ctx context.Context, n int) ([]domain.JobRun, error)
	DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error)
}
