package api

import (
	"context"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/pubsub"
	"orchestrator/internal/store"
)

// mockAPIStore implements APIStore for testing.
type mockAPIStore struct {
	createJobFn               func(ctx context.Context, job *domain.Job) error
	getJobFn                  func(ctx context.Context, id string) (*domain.Job, error)
	getJobBySlugFn            func(ctx context.Context, projectID, slug string) (*domain.Job, error)
	listJobsFn                func(ctx context.Context, projectID string) ([]domain.Job, error)
	updateJobFn               func(ctx context.Context, job *domain.Job) error
	getRunFn                  func(ctx context.Context, id string) (*domain.JobRun, error)
	getRunByIdempotencyKeyFn  func(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error)
	listRunsByProjectFn       func(ctx context.Context, projectID string, status *domain.RunStatus, limit int, cursor *time.Time) ([]domain.JobRun, error)
	updateRunStatusFn         func(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	listChildRunsFn           func(ctx context.Context, parentRunID string) ([]domain.JobRun, error)
	insertEventFn             func(ctx context.Context, event *domain.RunEvent) error
	listEventsByRunFilteredFn func(ctx context.Context, runID string, level, eventType string) ([]domain.RunEvent, error)
	listWebhookDeliveriesFn   func(ctx context.Context, status string, limit int) ([]domain.WebhookDelivery, error)
	updateHeartbeatFn         func(ctx context.Context, id string) error
	queueStatsFn              func(ctx context.Context) (*store.QueueStats, error)
}

func (m *mockAPIStore) CreateJob(ctx context.Context, job *domain.Job) error {
	if m.createJobFn != nil {
		return m.createJobFn(ctx, job)
	}
	return nil
}

func (m *mockAPIStore) GetJob(ctx context.Context, id string) (*domain.Job, error) {
	if m.getJobFn != nil {
		return m.getJobFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAPIStore) GetJobBySlug(ctx context.Context, projectID, slug string) (*domain.Job, error) {
	if m.getJobBySlugFn != nil {
		return m.getJobBySlugFn(ctx, projectID, slug)
	}
	return nil, nil
}

func (m *mockAPIStore) ListJobs(ctx context.Context, projectID string) ([]domain.Job, error) {
	if m.listJobsFn != nil {
		return m.listJobsFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateJob(ctx context.Context, job *domain.Job) error {
	if m.updateJobFn != nil {
		return m.updateJobFn(ctx, job)
	}
	return nil
}

func (m *mockAPIStore) GetRun(ctx context.Context, id string) (*domain.JobRun, error) {
	if m.getRunFn != nil {
		return m.getRunFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAPIStore) GetRunByIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error) {
	if m.getRunByIdempotencyKeyFn != nil {
		return m.getRunByIdempotencyKeyFn(ctx, jobID, idempotencyKey)
	}
	return nil, nil
}

func (m *mockAPIStore) ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	if m.listRunsByProjectFn != nil {
		return m.listRunsByProjectFn(ctx, projectID, status, limit, cursor)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	if m.updateRunStatusFn != nil {
		return m.updateRunStatusFn(ctx, id, from, to, fields)
	}
	return nil
}

func (m *mockAPIStore) ListChildRuns(ctx context.Context, parentRunID string) ([]domain.JobRun, error) {
	if m.listChildRunsFn != nil {
		return m.listChildRunsFn(ctx, parentRunID)
	}
	return nil, nil
}

func (m *mockAPIStore) InsertEvent(ctx context.Context, event *domain.RunEvent) error {
	if m.insertEventFn != nil {
		return m.insertEventFn(ctx, event)
	}
	return nil
}

func (m *mockAPIStore) ListEventsByRunFiltered(ctx context.Context, runID string, level, eventType string) ([]domain.RunEvent, error) {
	if m.listEventsByRunFilteredFn != nil {
		return m.listEventsByRunFilteredFn(ctx, runID, level, eventType)
	}
	return nil, nil
}

func (m *mockAPIStore) ListWebhookDeliveries(ctx context.Context, status string, limit int) ([]domain.WebhookDelivery, error) {
	if m.listWebhookDeliveriesFn != nil {
		return m.listWebhookDeliveriesFn(ctx, status, limit)
	}
	return nil, nil
}

func (m *mockAPIStore) UpdateHeartbeat(ctx context.Context, id string) error {
	if m.updateHeartbeatFn != nil {
		return m.updateHeartbeatFn(ctx, id)
	}
	return nil
}

func (m *mockAPIStore) QueueStats(ctx context.Context) (*store.QueueStats, error) {
	if m.queueStatsFn != nil {
		return m.queueStatsFn(ctx)
	}
	return &store.QueueStats{}, nil
}

// mockQueue implements queue.Queue for testing.
type mockQueue struct {
	enqueueFn  func(ctx context.Context, run *domain.JobRun) error
	dequeueFn  func(ctx context.Context) (*domain.JobRun, error)
	dequeueNFn func(ctx context.Context, n int) ([]domain.JobRun, error)
}

func (m *mockQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	if m.enqueueFn != nil {
		return m.enqueueFn(ctx, run)
	}
	return nil
}

func (m *mockQueue) Dequeue(ctx context.Context) (*domain.JobRun, error) {
	if m.dequeueFn != nil {
		return m.dequeueFn(ctx)
	}
	return nil, nil
}

func (m *mockQueue) DequeueN(ctx context.Context, n int) ([]domain.JobRun, error) {
	if m.dequeueNFn != nil {
		return m.dequeueNFn(ctx, n)
	}
	return nil, nil
}

// mockPublisher implements pubsub.Publisher for testing.
type mockPublisher struct {
	publishFn   func(ctx context.Context, channel string, data []byte) error
	subscribeFn func(ctx context.Context, channel string) (*pubsub.Subscription, error)
	closeFn     func() error
}

func (m *mockPublisher) Publish(ctx context.Context, channel string, data []byte) error {
	if m.publishFn != nil {
		return m.publishFn(ctx, channel, data)
	}
	return nil
}

func (m *mockPublisher) Subscribe(ctx context.Context, channel string) (*pubsub.Subscription, error) {
	if m.subscribeFn != nil {
		return m.subscribeFn(ctx, channel)
	}
	return nil, nil
}

func (m *mockPublisher) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

type mockPinger struct {
	err error
}

func (m *mockPinger) Ping(_ context.Context) error {
	return m.err
}
