package api

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

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
	if err == nil {
		t.Fatalf("expected error when pubsub is nil, got out=%+v", out)
	}
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected huma.StatusError, got %T: %v", err, err)
	}
	if statusErr.GetStatus() != 503 {
		t.Fatalf("expected 503, got %d (%v)", statusErr.GetStatus(), err)
	}
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
	if err == nil {
		t.Fatalf("expected error when publish fails, got out=%+v", out)
	}
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected huma.StatusError, got %T: %v", err, err)
	}
	if statusErr.GetStatus() != 503 {
		t.Fatalf("expected 503, got %d (%v)", statusErr.GetStatus(), err)
	}
}

// TestHandleDeleteWorker_HealthyPublishReturns200 confirms the happy path
// still returns the disconnect_requested envelope when publish succeeds.
func TestHandleDeleteWorker_HealthyPublishReturns200(t *testing.T) {
	t.Parallel()
	var publishedChannel string
	var publishedData string
	pub := &mockPublisher{
		publishFn: func(_ context.Context, channel string, data []byte) error {
			publishedChannel = channel
			publishedData = string(data)
			return nil
		},
		subscribeFn: func(ctx context.Context, channel string) (*pubsub.Subscription, error) {
			ch := make(chan []byte, 1)
			ch <- []byte("worker-1")
			return pubsub.NewSubscription(ch, func() {}), nil
		},
	}
	srv, ctx := newDeleteWorkerServer(t, pub, func(ctx context.Context, workerID, projectID string) (*domain.Worker, error) {
		return ownedWorker(), nil
	})

	out, err := srv.handleDeleteWorker(ctx, &DeleteWorkerInput{WorkerID: "worker-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.Body["status"] != "disconnected" {
		t.Fatalf("expected disconnected envelope, got %+v", out)
	}
	if !strings.HasPrefix(publishedChannel, "worker:disconnect:") {
		t.Fatalf("expected channel prefix worker:disconnect:, got %q", publishedChannel)
	}
	if publishedData != "worker-1" {
		t.Fatalf("expected published data %q, got %q", "worker-1", publishedData)
	}
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
	if err == nil {
		t.Fatalf("expected timeout error, got out=%+v", out)
	}
	if elapsed := time.Since(start); elapsed < workerDisconnectAckTimeout {
		t.Fatalf("handler returned before ack timeout: %v", elapsed)
	}
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected huma.StatusError, got %T: %v", err, err)
	}
	if statusErr.GetStatus() != 503 {
		t.Fatalf("expected 503, got %d (%v)", statusErr.GetStatus(), err)
	}
	if !strings.Contains(err.Error(), "worker_disconnect_pending") {
		t.Fatalf("expected worker_disconnect_pending error, got %v", err)
	}
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
	if err == nil {
		t.Fatalf("expected closed ack channel to fail, got out=%+v", out)
	}
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected huma.StatusError, got %T: %v", err, err)
	}
	if statusErr.GetStatus() != 503 {
		t.Fatalf("expected 503, got %d (%v)", statusErr.GetStatus(), err)
	}
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
	if err == nil {
		t.Fatalf("expected mismatched ack to fail, got out=%+v", out)
	}
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected huma.StatusError, got %T: %v", err, err)
	}
	if statusErr.GetStatus() != 503 {
		t.Fatalf("expected 503, got %d (%v)", statusErr.GetStatus(), err)
	}
}

// TestHandleDeleteWorker_UnknownWorker404 — cross-tenant existence guard:
// a worker not in the caller's project must yield 404, not 503 / 500.
func TestHandleDeleteWorker_UnknownWorker404(t *testing.T) {
	t.Parallel()
	srv, ctx := newDeleteWorkerServer(t, &mockPublisher{}, func(ctx context.Context, workerID, projectID string) (*domain.Worker, error) {
		return nil, nil // not found
	})

	_, err := srv.handleDeleteWorker(ctx, &DeleteWorkerInput{WorkerID: "ghost"})
	if err == nil {
		t.Fatal("expected 404 for unknown worker")
	}
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected huma.StatusError, got %T: %v", err, err)
	}
	if statusErr.GetStatus() != 404 {
		t.Fatalf("expected 404, got %d (%v)", statusErr.GetStatus(), err)
	}
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
	if err == nil {
		t.Fatal("expected 404 when store errors")
	}
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected huma.StatusError, got %T: %v", err, err)
	}
	if statusErr.GetStatus() != 404 {
		t.Fatalf("expected 404, got %d (%v)", statusErr.GetStatus(), err)
	}
}
