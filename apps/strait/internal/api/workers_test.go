package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
	"strait/internal/pubsub"
)

// helper: build a minimal Server for handleDeleteWorker tests, scoped by a
// fixed project ID via context. Avoids spinning the full HTTP stack.
func newDeleteWorkerServer(t *testing.T, pub *mockPublisher, getWorker func(ctx context.Context, workerID, projectID string) (*domain.Worker, error)) (*Server, context.Context) {
	t.Helper()
	store := &APIStoreMock{
		GetWorkerFunc: getWorker,
	}
	srv := newTestServer(t, store, &mockQueue{}, pub)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	return srv, ctx
}

func ownedWorker() *domain.Worker {
	return &domain.Worker{
		ID:        "worker-1",
		ProjectID: "proj-1",
		Status:    domain.WorkerStatusActive,
	}
}

// TestHandleDeleteWorker_NilPubsubReturns503 is the regression for the bug
// where DELETE /workers/:id silently returned 200 even when no Publisher was
// configured, leaving the operator believing the worker had been disconnected.
func TestHandleDeleteWorker_NilPubsubReturns503(t *testing.T) {
	t.Parallel()
	srv, ctx := newDeleteWorkerServer(t, nil, func(ctx context.Context, workerID, projectID string) (*domain.Worker, error) {
		return ownedWorker(), nil
	})

	out, err := srv.handleDeleteWorker(ctx, &DeleteWorkerInput{WorkerID: "worker-1"})
	require.Error(t, err)
	require.Nil(t, out)

	var statusErr huma.StatusError
	require.ErrorAs(
		t, err, &statusErr)
	require.Equal(t, 503, statusErr.
		GetStatus())
}

// TestHandleDeleteWorker_PublishErrorReturns503 ensures publish failure does
// not silently return 200 — the caller must learn that the disconnect did
// not propagate.
func TestHandleDeleteWorker_PublishErrorReturns503(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{
		publishFn: func(_ context.Context, _ string, _ []byte) error {
			return errors.New("redis down")
		},
	}
	srv, ctx := newDeleteWorkerServer(t, pub, func(ctx context.Context, workerID, projectID string) (*domain.Worker, error) {
		return ownedWorker(), nil
	})

	out, err := srv.handleDeleteWorker(ctx, &DeleteWorkerInput{WorkerID: "worker-1"})
	require.Error(t, err)
	require.Nil(t, out)

	var statusErr huma.StatusError
	require.ErrorAs(
		t, err, &statusErr)
	require.Equal(t, 503, statusErr.
		GetStatus())
}

// TestHandleDeleteWorker_HealthyPublishReturns200 confirms the happy path
// still returns the disconnect_requested envelope when publish succeeds.
func TestHandleDeleteWorker_HealthyPublishReturns200(t *testing.T) {
	t.Parallel()
	var publishedChannel string
	var subscribedChannel string
	var publishedData string
	pub := &mockPublisher{
		publishFn: func(_ context.Context, channel string, data []byte) error {
			publishedChannel = channel
			publishedData = string(data)
			return nil
		},
		subscribeFn: func(ctx context.Context, channel string) (*pubsub.Subscription, error) {
			subscribedChannel = channel
			ch := make(chan []byte, 1)
			ch <- []byte("worker-1")
			return pubsub.NewSubscription(ch, func() {}), nil
		},
	}
	srv, ctx := newDeleteWorkerServer(t, pub, func(ctx context.Context, workerID, projectID string) (*domain.Worker, error) {
		return ownedWorker(), nil
	})

	out, err := srv.handleDeleteWorker(ctx, &DeleteWorkerInput{WorkerID: "worker-1"})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "disconnected", out.Body["status"])
	require.Equal(t, "worker:disconnect:proj-1:worker-1",

		publishedChannel)
	require.Equal(t, "worker:disconnect_ack:proj-1:worker-1",

		subscribedChannel)
	require.Equal(t, "worker-1",
		publishedData,
	)
}

func TestHandleDeleteWorker_AckTimeoutReturns503(t *testing.T) {
	oldTimeout := workerDisconnectAckTimeout
	workerDisconnectAckTimeout = 20 * time.Millisecond
	t.Cleanup(func() { workerDisconnectAckTimeout = oldTimeout })

	pub := &mockPublisher{
		subscribeFn: func(ctx context.Context, channel string) (*pubsub.Subscription, error) {
			ch := make(chan []byte)
			return pubsub.NewSubscription(ch, func() { close(ch) }), nil
		},
	}
	srv, ctx := newDeleteWorkerServer(t, pub, func(ctx context.Context, workerID, projectID string) (*domain.Worker, error) {
		return ownedWorker(), nil
	})

	start := time.Now()
	out, err := srv.handleDeleteWorker(ctx, &DeleteWorkerInput{WorkerID: "worker-1"})
	require.Error(t, err)
	require.Nil(t, out)
	require.GreaterOrEqual(
		t, time.Since(start),
		workerDisconnectAckTimeout,
	)

	var statusErr huma.StatusError
	require.ErrorAs(
		t, err, &statusErr)
	require.Equal(t, 503, statusErr.
		GetStatus())
	require.Contains(
		t, err.
			Error(), "worker_disconnect_pending")
}

func TestHandleDeleteWorker_ClosedAckChannelReturns503(t *testing.T) {
	oldTimeout := workerDisconnectAckTimeout
	workerDisconnectAckTimeout = time.Second
	t.Cleanup(func() { workerDisconnectAckTimeout = oldTimeout })

	pub := &mockPublisher{
		subscribeFn: func(ctx context.Context, channel string) (*pubsub.Subscription, error) {
			ch := make(chan []byte)
			close(ch)
			return pubsub.NewSubscription(ch, func() {}), nil
		},
	}
	srv, ctx := newDeleteWorkerServer(t, pub, func(ctx context.Context, workerID, projectID string) (*domain.Worker, error) {
		return ownedWorker(), nil
	})

	out, err := srv.handleDeleteWorker(ctx, &DeleteWorkerInput{WorkerID: "worker-1"})
	require.Error(t, err)
	require.Nil(t, out)

	var statusErr huma.StatusError
	require.ErrorAs(
		t, err, &statusErr)
	require.Equal(t, 503, statusErr.
		GetStatus())
}

func TestHandleDeleteWorker_MismatchedAckReturns503(t *testing.T) {
	pub := &mockPublisher{
		subscribeFn: func(ctx context.Context, channel string) (*pubsub.Subscription, error) {
			ch := make(chan []byte, 1)
			ch <- []byte("other-worker")
			return pubsub.NewSubscription(ch, func() {}), nil
		},
	}
	srv, ctx := newDeleteWorkerServer(t, pub, func(ctx context.Context, workerID, projectID string) (*domain.Worker, error) {
		return ownedWorker(), nil
	})

	out, err := srv.handleDeleteWorker(ctx, &DeleteWorkerInput{WorkerID: "worker-1"})
	require.Error(t, err)
	require.Nil(t, out)

	var statusErr huma.StatusError
	require.ErrorAs(
		t, err, &statusErr)
	require.Equal(t, 503, statusErr.
		GetStatus())
}

// TestHandleDeleteWorker_UnknownWorker404 — cross-tenant existence guard:
// a worker not in the caller's project must yield 404, not 503 / 500.
func TestHandleDeleteWorker_UnknownWorker404(t *testing.T) {
	t.Parallel()
	srv, ctx := newDeleteWorkerServer(t, &mockPublisher{}, func(ctx context.Context, workerID, projectID string) (*domain.Worker, error) {
		return nil, nil // not found
	})

	_, err := srv.handleDeleteWorker(ctx, &DeleteWorkerInput{WorkerID: "ghost"})
	require.Error(t, err)

	var statusErr huma.StatusError
	require.ErrorAs(
		t, err, &statusErr)
	require.Equal(t, 404, statusErr.
		GetStatus())
}

// TestHandleDeleteWorker_StoreError404 — store failures should also yield
// 404 (existence-leak avoidance), and we must reach this branch BEFORE the
// publish path so a store outage doesn't turn into a misleading 503.
func TestHandleDeleteWorker_StoreError404(t *testing.T) {
	t.Parallel()
	srv, ctx := newDeleteWorkerServer(t, &mockPublisher{}, func(ctx context.Context, workerID, projectID string) (*domain.Worker, error) {
		return nil, errors.New("db down")
	})

	_, err := srv.handleDeleteWorker(ctx, &DeleteWorkerInput{WorkerID: "worker-1"})
	require.Error(t, err)

	var statusErr huma.StatusError
	require.ErrorAs(
		t, err, &statusErr)
	require.Equal(t, 404, statusErr.
		GetStatus())
}
