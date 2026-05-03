package api

import (
	"context"

	"strait/internal/domain"
)

// WorkerStore stub methods on APIStoreMock.
// These satisfy the WorkerStore interface that was added to APIStore in Phase 8.
// The generated mock (mock_apistore_test.go) does not include them yet; this file
// bridges the gap without touching the generated file.

func (m *APIStoreMock) GetWorker(ctx context.Context, workerID, projectID string) (*domain.Worker, error) {
	if m.GetWorkerFunc != nil {
		return m.GetWorkerFunc(ctx, workerID, projectID)
	}
	return nil, nil
}

func (m *APIStoreMock) ListWorkers(ctx context.Context, projectID, queueName string, limit, offset int) ([]domain.Worker, error) {
	if m.ListWorkersFunc != nil {
		return m.ListWorkersFunc(ctx, projectID, queueName, limit, offset)
	}
	return nil, nil
}

func (m *APIStoreMock) ListWorkerTasksByWorker(ctx context.Context, workerID string, status domain.WorkerTaskStatus, limit, offset int) ([]domain.WorkerTask, error) {
	if m.ListWorkerTasksByWorkerFunc != nil {
		return m.ListWorkerTasksByWorkerFunc(ctx, workerID, status, limit, offset)
	}
	return nil, nil
}
