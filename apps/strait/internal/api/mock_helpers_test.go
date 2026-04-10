package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"strait/internal/domain"
	"strait/internal/pubsub"
)

// testInternalSecret is the internal secret used across all api package tests.
// Defined once here so tests stay consistent and the value is easy to update.
const testInternalSecret = "test-secret-value"

// testJWTSigningKey is a cryptographically random 32-byte key generated once
// per test binary. Using a random key instead of a hardcoded string avoids
// gitleaks false positives and ensures tests don't depend on a specific key value.
var testJWTSigningKey = func() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate test JWT key: " + err.Error())
	}
	return hex.EncodeToString(b)
}()

type mockQueue struct {
	enqueueFn           func(ctx context.Context, run *domain.JobRun) error
	enqueueBatchFn      func(ctx context.Context, runs []*domain.JobRun) (int64, error)
	dequeueFn           func(ctx context.Context) (*domain.JobRun, error)
	dequeueNFn          func(ctx context.Context, n int) ([]domain.JobRun, error)
	dequeueNByProjectFn func(ctx context.Context, n int, projectID string) ([]domain.JobRun, error)
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

func (m *mockQueue) EnqueueBatch(ctx context.Context, runs []*domain.JobRun) (int64, error) {
	if m.enqueueBatchFn != nil {
		return m.enqueueBatchFn(ctx, runs)
	}
	// Fall back to individual enqueue for backwards-compatible tests.
	for _, run := range runs {
		if m.enqueueFn != nil {
			if err := m.enqueueFn(ctx, run); err != nil {
				return 0, err
			}
		}
	}
	return int64(len(runs)), nil
}

func (m *mockQueue) DequeueNFair(_ context.Context, _ int) ([]domain.JobRun, error) {
	return nil, nil
}

func (m *mockQueue) DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	if m.dequeueNByProjectFn != nil {
		return m.dequeueNByProjectFn(ctx, n, projectID)
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

func (m *mockPublisher) PublishBatch(ctx context.Context, messages []pubsub.PubSubMessage) error {
	for _, msg := range messages {
		if err := m.Publish(ctx, msg.Channel, msg.Data); err != nil {
			return err
		}
	}
	return nil
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
