//go:build integration

package grpc

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

type blockingWorkerStream struct {
	ctx      context.Context
	recvCh   chan *workerv1.WorkerMessage
	sentCh   chan *workerv1.ServerMessage
	recvDone chan struct{}
}

func newBlockingWorkerStream(ctx context.Context, rawKey string) *blockingWorkerStream {
	streamCtx := metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer "+rawKey))
	return &blockingWorkerStream{
		ctx:      streamCtx,
		recvCh:   make(chan *workerv1.WorkerMessage, 1),
		sentCh:   make(chan *workerv1.ServerMessage, 4),
		recvDone: make(chan struct{}),
	}
}

func (s *blockingWorkerStream) Context() context.Context { return s.ctx }

func (s *blockingWorkerStream) Send(msg *workerv1.ServerMessage) error {
	select {
	case s.sentCh <- msg:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *blockingWorkerStream) Recv() (*workerv1.WorkerMessage, error) {
	select {
	case msg, ok := <-s.recvCh:
		if !ok {
			close(s.recvDone)
			return nil, io.EOF
		}
		return msg, nil
	case <-s.ctx.Done():
		close(s.recvDone)
		return nil, s.ctx.Err()
	}
}

func (s *blockingWorkerStream) SetHeader(metadata.MD) error  { return nil }
func (s *blockingWorkerStream) SendHeader(metadata.MD) error { return nil }
func (s *blockingWorkerStream) SetTrailer(metadata.MD)       {}
func (s *blockingWorkerStream) SendMsg(any) error            { return nil }
func (s *blockingWorkerStream) RecvMsg(any) error            { return nil }

func seedGRPCAPIKey(t *testing.T, ctx context.Context, q *store.Queries, projectID, keyID, rawKey string) {
	t.Helper()
	if err := q.CreateAPIKey(ctx, &domain.APIKey{
		ID:        keyID,
		ProjectID: projectID,
		Name:      "worker-key",
		KeyHash:   hashGRPCAPIKey(rawKey),
		KeyPrefix: "strait",
		Scopes:    []string{"*"},
	}); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
}

func TestIntegration_StreamTasks_APIKeyRevokeReturnsWithoutClientRecv(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

	q := store.New(env.DB.Pool)
	const (
		projectID = "proj-stream-revoke"
		workerID  = "worker-stream-revoke"
		apiKeyID  = "key-stream-revoke"
		rawKey    = "strait_stream_revoke_test_key"
	)
	seedGRPCAPIKey(t, ctx, q, projectID, apiKeyID, rawKey)

	streamCtx, cancel := context.WithCancel(ctx)
	stream := newBlockingWorkerStream(streamCtx, rawKey)
	stream.recvCh <- &workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_Registration{
			Registration: &workerv1.WorkerRegistration{
				WorkerId:       workerID,
				Name:           "test worker",
				Hostname:       "host",
				SdkVersion:     "1.0.0",
				SdkLanguage:    "go",
				Queues:         []string{"default"},
				SlotsTotal:     1,
				SlotsAvailable: 1,
			},
		},
	}

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
	}

	done := make(chan error, 1)
	go func() {
		done <- svc.StreamTasks(stream)
	}()

	select {
	case msg := <-stream.sentCh:
		if msg.GetAck() == nil {
			t.Fatalf("first server message = %T, want ack", msg.Payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for registration ack")
	}

	svc.registry.CloseByAPIKey(apiKeyID)

	select {
	case err := <-done:
		if err == nil || !errors.Is(err, errAPIKeyRevoked) {
			t.Fatalf("StreamTasks error = %v, want api key revoked", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StreamTasks did not return after API-key revoke signal")
	}

	cancel()
	select {
	case <-stream.recvDone:
	case <-time.After(2 * time.Second):
		t.Fatal("recv loop did not exit after test context cancellation")
	}
}
