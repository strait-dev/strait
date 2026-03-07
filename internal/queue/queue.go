package queue

import (
	"context"

	"orchestrator/internal/domain"
)

type Queue interface {
	Enqueue(ctx context.Context, run *domain.JobRun) error
	Dequeue(ctx context.Context) (*domain.JobRun, error)
	DequeueN(ctx context.Context, n int) ([]domain.JobRun, error)
	DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error)
}
