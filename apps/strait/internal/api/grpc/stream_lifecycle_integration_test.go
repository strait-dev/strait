//go:build integration

package grpc

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

type blockingWorkerStream struct {
	ctx      context.Context
	recvCh   chan *workerv1.WorkerMessage
	sentCh   chan *workerv1.ServerMessage
	recvDone chan struct{}
	recvWait chan struct{}
	recvOnce sync.Once
}

func newBlockingWorkerStream(ctx context.Context, rawKey string) *blockingWorkerStream {
	streamCtx := metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer "+rawKey))
	return &blockingWorkerStream{
		ctx:      streamCtx,
		recvCh:   make(chan *workerv1.WorkerMessage, 1),
		sentCh:   make(chan *workerv1.ServerMessage, 4),
		recvDone: make(chan struct{}),
		recvWait: make(chan struct{}),
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
	s.recvOnce.Do(func() { close(s.recvWait) })
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

type testRevocationPublisher struct {
	mu   sync.Mutex
	subs map[string][]chan []byte
}

func newTestRevocationPublisher() *testRevocationPublisher {
	return &testRevocationPublisher{subs: make(map[string][]chan []byte)}
}

func (p *testRevocationPublisher) Publish(_ context.Context, channel string, payload []byte) error {
	p.mu.Lock()
	subs := append([]chan []byte(nil), p.subs[channel]...)
	p.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- payload:
		default:
		}
	}
	return nil
}

func (p *testRevocationPublisher) PublishBatch(ctx context.Context, messages []pubsub.PubSubMessage) error {
	for _, msg := range messages {
		if err := p.Publish(ctx, msg.Channel, msg.Data); err != nil {
			return err
		}
	}
	return nil
}

func (p *testRevocationPublisher) Subscribe(_ context.Context, channel string) (*pubsub.Subscription, error) {
	ch := make(chan []byte, 1)
	p.mu.Lock()
	p.subs[channel] = append(p.subs[channel], ch)
	p.mu.Unlock()
	return pubsub.NewSubscription(ch, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		subs := p.subs[channel]
		filtered := subs[:0]
		for _, sub := range subs {
			if sub != ch {
				filtered = append(filtered, sub)
			}
		}
		if len(filtered) == 0 {
			delete(p.subs, channel)
		} else {
			p.subs[channel] = filtered
		}
		close(ch)
	}), nil
}

func (p *testRevocationPublisher) Close() error { return nil }

type failingSubscribePublisher struct{}

func (failingSubscribePublisher) Publish(context.Context, string, []byte) error { return nil }
func (failingSubscribePublisher) PublishBatch(context.Context, []pubsub.PubSubMessage) error {
	return nil
}
func (failingSubscribePublisher) Subscribe(context.Context, string) (*pubsub.Subscription, error) {
	return nil, errors.New("redis subscription unavailable")
}
func (failingSubscribePublisher) Close() error { return nil }

func seedGRPCAPIKey(t *testing.T, ctx context.Context, q *store.Queries, projectID, keyID, rawKey string) {
	t.Helper()
	seedGRPCAPIKeyWithExpiry(t, ctx, q, projectID, keyID, rawKey, nil)
}

func seedGRPCAPIKeyWithExpiry(t *testing.T, ctx context.Context, q *store.Queries, projectID, keyID, rawKey string, expiresAt *time.Time) {
	t.Helper()
	require.NoError(t,

		q.CreateProject(ctx,
			&domain.
				Project{ID: projectID,
				OrgID: "org-" +
					projectID, Name: projectID},
		))
	require.NoError(t,

		q.CreateAPIKey(ctx,
			&domain.APIKey{
				ID:        keyID,
				ProjectID: projectID,

				Name: "worker-key", KeyHash: hashGRPCAPIKey(rawKey), KeyPrefix: "strait", Scopes: []string{"*"},

				ExpiresAt: expiresAt}))

}

func newLifecycleBillingEnforcer(t *testing.T, env *testutil.TestEnv) *billing.Enforcer {
	t.Helper()
	require.False(t,
		env ==
			nil ||
			env.Redis ==
				nil ||
			env.
				Redis.Client ==
				nil,
	)

	return billing.NewEnforcer(billing.NewPgStore(env.DB.Pool), env.Redis.Client, nil)
}

func TestIntegration_StreamTasks_SubscribeFailureRejectsWorker(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const (
		projectID = "proj-subscribe-failure"
		apiKeyID  = "key-subscribe-failure"
		rawKey    = "strait_subscribeFailureKey"
	)
	seedGRPCAPIKey(t, ctx, q, projectID, apiKeyID, rawKey)

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newBlockingWorkerStream(streamCtx, rawKey)
	svc := &workerService{
		queries:         q,
		pub:             failingSubscribePublisher{},
		registry:        NewConnectionRegistry(),
		resultChannels:  NewResultChannelRegistry(),
		billingEnforcer: newLifecycleBillingEnforcer(t, env),
	}

	err := svc.StreamTasks(stream)
	require.Equal(t,
		codes.
			Unavailable,
		status.
			Code(
				err))

	if got := svc.registry.Snapshot(); len(got) != 0 {
		require.Failf(t, "test failure",

			"subscribe failure mutated registry: got %d workers", len(got))
	}
}

func TestIntegration_StreamTasks_APIKeyRevokeReturnsWithoutClientRecv(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const (
		projectID = "proj-stream-revoke"
		workerID  = "worker-stream-revoke"
		apiKeyID  = "key-stream-revoke"
		rawKey    = "strait_streamRevokeTestKey"
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
		queries:         q,
		pub:             &noopPublisher{},
		registry:        NewConnectionRegistry(),
		resultChannels:  NewResultChannelRegistry(),
		billingEnforcer: newLifecycleBillingEnforcer(t, env),
	}

	done := make(chan error, 1)
	concWG.Go(func() {
		done <- svc.StreamTasks(stream)
	})

	select {
	case msg := <-stream.sentCh:
		if msg.GetAck() == nil {
			require.Failf(t, "test failure",

				"first server message = %T, want ack", msg.Payload)
		}
	case err := <-done:
		require.Failf(t, "test failure", "StreamTasks returned before registration ack: %v", err)
	case <-time.After(5 * time.Second):
		require.Fail(t, "timed out waiting for registration ack")
	}

	svc.registry.CloseByAPIKey(apiKeyID)

	select {
	case err := <-done:
		if err == nil || !errors.Is(err, errAPIKeyRevoked) {
			require.Failf(t, "test failure",

				"StreamTasks error = %v, want api key revoked", err)
		}
	case <-time.After(2 * time.Second):
		require.Fail(t, "StreamTasks did not return after API-key revoke signal")
	}

	cancel()
	select {
	case <-stream.recvDone:
	case <-time.After(2 * time.Second):
		require.Fail(t, "recv loop did not exit after test context cancellation")
	}
}

func TestIntegration_StreamTasks_RevokeBeforeRegistrationRejectsWorker(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const (
		projectID = "proj-pre-registration-revoke"
		workerID  = "worker-pre-registration-revoke"
		apiKeyID  = "key-pre-registration-revoke"
		rawKey    = "strait_preRegistrationRevokeKey"
	)
	seedGRPCAPIKey(t, ctx, q, projectID, apiKeyID, rawKey)

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newBlockingWorkerStream(streamCtx, rawKey)
	pub := newTestRevocationPublisher()
	svc := &workerService{
		queries:         q,
		pub:             pub,
		registry:        NewConnectionRegistry(),
		resultChannels:  NewResultChannelRegistry(),
		billingEnforcer: newLifecycleBillingEnforcer(t, env),
	}

	done := make(chan error, 1)
	concWG.Go(func() {
		done <- svc.StreamTasks(stream)
	})

	select {
	case <-stream.recvWait:
	case err := <-done:
		require.Failf(t, "test failure", "StreamTasks returned before registration wait: %v", err)
	case <-time.After(5 * time.Second):
		require.Fail(t, "timed out waiting for pre-registration recv")
	}
	require.NoError(t,

		q.RevokeAPIKey(ctx,
			apiKeyID),
	)
	require.NoError(t,

		pub.Publish(ctx, "apikey:revoked:"+
			apiKeyID,
			[]byte(apiKeyID)),
	)

	select {
	case err := <-done:
		if !errors.Is(err, errAPIKeyRevoked) {
			require.Failf(t, "test failure",

				"StreamTasks error = %v, want api key revoked", err)
		}
	case <-time.After(5 * time.Second):
		require.Fail(t, "StreamTasks did not return after pre-registration revoke")
	}

	stream.recvCh <- &workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_Registration{
			Registration: &workerv1.WorkerRegistration{
				WorkerId:       workerID,
				Name:           "late worker",
				Hostname:       "host",
				SdkVersion:     "1.0.0",
				SdkLanguage:    "go",
				Queues:         []string{"default"},
				SlotsTotal:     1,
				SlotsAvailable: 1,
			},
		},
	}
	cancel()
	if got := svc.registry.Snapshot(); len(got) != 0 {
		require.Failf(t, "test failure",

			"revoked pre-registration stream mutated registry: got %d workers", len(got))
	}
	select {
	case msg := <-stream.sentCh:
		require.Failf(t, "test failure", "revoked pre-registration stream sent message: %T", msg.Payload)
	default:
	}
}

func TestIntegration_StreamTasks_APIKeyExpiryClosesRegisteredStream(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const (
		projectID = "proj-stream-expiry"
		workerID  = "worker-stream-expiry"
		apiKeyID  = "key-stream-expiry"
		rawKey    = "strait_streamExpiryTestKey"
	)
	expiresAt := time.Now().Add(750 * time.Millisecond)
	seedGRPCAPIKeyWithExpiry(t, ctx, q, projectID, apiKeyID, rawKey, &expiresAt)

	streamCtx, cancel := context.WithCancel(ctx)
	stream := newBlockingWorkerStream(streamCtx, rawKey)
	stream.recvCh <- &workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_Registration{
			Registration: &workerv1.WorkerRegistration{
				WorkerId:       workerID,
				Name:           "expiring worker",
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
		queries:         q,
		pub:             &noopPublisher{},
		registry:        NewConnectionRegistry(),
		resultChannels:  NewResultChannelRegistry(),
		billingEnforcer: newLifecycleBillingEnforcer(t, env),
	}

	done := make(chan error, 1)
	concWG.Go(func() {
		done <- svc.StreamTasks(stream)
	})

	select {
	case msg := <-stream.sentCh:
		if msg.GetAck() == nil {
			require.Failf(t, "test failure",

				"first server message = %T, want ack", msg.Payload)
		}
	case err := <-done:
		require.Failf(t, "test failure", "StreamTasks returned before registration ack: %v", err)
	case <-time.After(5 * time.Second):
		require.Fail(t, "timed out waiting for registration ack")
	}

	select {
	case err := <-done:
		if err == nil || !errors.Is(err, errAPIKeyExpired) {
			require.Failf(t, "test failure",

				"StreamTasks error = %v, want api key expired", err)
		}
	case <-time.After(5 * time.Second):
		require.Fail(t, "StreamTasks did not return after API-key expiry")
	}

	if got := svc.registry.Snapshot(); len(got) != 0 {
		require.Failf(t, "test failure",

			"expired stream left registered worker: got %d workers", len(got))
	}
	cancel()
	select {
	case <-stream.recvDone:
	case <-time.After(2 * time.Second):
		require.Fail(t, "recv loop did not exit after test context cancellation")
	}
}

func TestIntegration_StreamTasks_APIKeyRotationGraceSignalClosesRegisteredStream(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const (
		projectID = "proj-stream-rotation-expiry"
		workerID  = "worker-stream-rotation-expiry"
		apiKeyID  = "key-stream-rotation-expiry"
		rawKey    = "strait_streamRotationExpiryKey"
	)
	seedGRPCAPIKey(t, ctx, q, projectID, apiKeyID, rawKey)

	pub := newTestRevocationPublisher()
	streamCtx, cancel := context.WithCancel(ctx)
	stream := newBlockingWorkerStream(streamCtx, rawKey)
	stream.recvCh <- &workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_Registration{
			Registration: &workerv1.WorkerRegistration{
				WorkerId:       workerID,
				Name:           "rotation expiring worker",
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
		queries:         q,
		pub:             pub,
		registry:        NewConnectionRegistry(),
		resultChannels:  NewResultChannelRegistry(),
		billingEnforcer: newLifecycleBillingEnforcer(t, env),
	}

	done := make(chan error, 1)
	concWG.Go(func() {
		done <- svc.StreamTasks(stream)
	})

	select {
	case msg := <-stream.sentCh:
		if msg.GetAck() == nil {
			require.Failf(t, "test failure",

				"first server message = %T, want ack", msg.Payload)
		}
	case err := <-done:
		require.Failf(t, "test failure", "StreamTasks returned before registration ack: %v", err)
	case <-time.After(5 * time.Second):
		require.Fail(t, "timed out waiting for registration ack")
	}

	graceExpiresAt := time.Now().Add(250 * time.Millisecond)
	require.NoError(t,

		pub.Publish(ctx, "apikey:expires:"+
			apiKeyID,
			[]byte(graceExpiresAt.
				UTC().Format(time.RFC3339Nano))))

	select {
	case err := <-done:
		if err == nil || !errors.Is(err, errAPIKeyExpired) {
			require.Failf(t, "test failure",

				"StreamTasks error = %v, want api key expired", err)
		}
	case <-time.After(5 * time.Second):
		require.Fail(t, "StreamTasks did not return after rotation grace expiry signal")
	}
	if got := svc.registry.Snapshot(); len(got) != 0 {
		require.Failf(t, "test failure",

			"rotation-expired stream left registered worker: got %d workers", len(got))
	}
	cancel()
	select {
	case <-stream.recvDone:
	case <-time.After(2 * time.Second):
		require.Fail(t, "recv loop did not exit after test context cancellation")
	}
}

func TestIntegration_StreamTasks_APIKeyExpiryBeforeRegistrationRejectsWorker(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const (
		projectID = "proj-pre-registration-expiry"
		apiKeyID  = "key-pre-registration-expiry"
		rawKey    = "strait_preRegistrationExpiryKey"
	)
	expiresAt := time.Now().Add(250 * time.Millisecond)
	seedGRPCAPIKeyWithExpiry(t, ctx, q, projectID, apiKeyID, rawKey, &expiresAt)

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newBlockingWorkerStream(streamCtx, rawKey)
	svc := &workerService{
		queries:         q,
		pub:             &noopPublisher{},
		registry:        NewConnectionRegistry(),
		resultChannels:  NewResultChannelRegistry(),
		billingEnforcer: newLifecycleBillingEnforcer(t, env),
	}

	done := make(chan error, 1)
	concWG.Go(func() {
		done <- svc.StreamTasks(stream)
	})

	select {
	case <-stream.recvWait:
	case err := <-done:
		require.Failf(t, "test failure", "StreamTasks returned before registration wait: %v", err)
	case <-time.After(5 * time.Second):
		require.Fail(t, "timed out waiting for pre-registration recv")
	}

	select {
	case err := <-done:
		if err == nil || !errors.Is(err, errAPIKeyExpired) {
			require.Failf(t, "test failure",

				"StreamTasks error = %v, want api key expired", err)
		}
	case <-time.After(5 * time.Second):
		require.Fail(t, "StreamTasks did not return after pre-registration API-key expiry")
	}
	if got := svc.registry.Snapshot(); len(got) != 0 {
		require.Failf(t, "test failure",

			"expired pre-registration stream mutated registry: got %d workers", len(got))
	}
	select {
	case msg := <-stream.sentCh:
		require.Failf(t, "test failure", "expired pre-registration stream sent message: %T", msg.Payload)
	default:
	}
}

func TestIntegration_StreamTasks_RevalidatesAPIKeyAfterDelayedRegistration(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const (
		projectID = "proj-registration-revalidate"
		workerID  = "worker-registration-revalidate"
		apiKeyID  = "key-registration-revalidate"
		rawKey    = "strait_registrationRevalidateKey"
	)
	seedGRPCAPIKey(t, ctx, q, projectID, apiKeyID, rawKey)

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newBlockingWorkerStream(streamCtx, rawKey)
	svc := &workerService{
		queries:         q,
		pub:             &noopPublisher{},
		registry:        NewConnectionRegistry(),
		resultChannels:  NewResultChannelRegistry(),
		billingEnforcer: newLifecycleBillingEnforcer(t, env),
	}

	done := make(chan error, 1)
	concWG.Go(func() {
		done <- svc.StreamTasks(stream)
	})

	select {
	case <-stream.recvWait:
	case err := <-done:
		require.Failf(t, "test failure", "StreamTasks returned before registration wait: %v", err)
	case <-time.After(5 * time.Second):
		require.Fail(t, "timed out waiting for pre-registration recv")
	}
	require.NoError(t,

		q.RevokeAPIKey(ctx,
			apiKeyID),
	)

	stream.recvCh <- &workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_Registration{
			Registration: &workerv1.WorkerRegistration{
				WorkerId:       workerID,
				Name:           "stale worker",
				Hostname:       "host",
				SdkVersion:     "1.0.0",
				SdkLanguage:    "go",
				Queues:         []string{"default"},
				SlotsTotal:     1,
				SlotsAvailable: 1,
			},
		},
	}

	select {
	case err := <-done:
		if status.Code(err) != codes.Unauthenticated {
			require.Failf(t, "test failure",

				"StreamTasks error = %v, want Unauthenticated", err)
		}
	case <-time.After(5 * time.Second):
		require.Fail(t, "StreamTasks did not revalidate revoked key")
	}
	if got := svc.registry.Snapshot(); len(got) != 0 {
		require.Failf(t, "test failure",

			"stale registration mutated registry: got %d workers", len(got))
	}
}

func TestIntegration_StreamTasks_PreRegistrationStreamsCountTowardAPIKeyQuota(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const (
		projectID = "proj-pre-registration-quota"
		apiKeyID  = "key-pre-registration-quota"
		rawKey    = "strait_preRegistrationQuotaKey"
	)
	seedGRPCAPIKey(t, ctx, q, projectID, apiKeyID, rawKey)

	registry := NewConnectionRegistry()
	registry.maxStreamsPerProject = 10
	registry.maxStreamsPerAPIKey = 1
	svc := &workerService{
		queries:         q,
		pub:             &noopPublisher{},
		registry:        registry,
		resultChannels:  NewResultChannelRegistry(),
		billingEnforcer: newLifecycleBillingEnforcer(t, env),
	}

	firstCtx, firstCancel := context.WithCancel(ctx)
	defer firstCancel()
	firstStream := newBlockingWorkerStream(firstCtx, rawKey)
	firstDone := make(chan error, 1)
	concWG.Go(func() {
		firstDone <- svc.StreamTasks(firstStream)
	})
	select {
	case <-firstStream.recvWait:
	case err := <-firstDone:
		require.Failf(t, "test failure", "first StreamTasks returned before registration wait: %v", err)
	case <-time.After(5 * time.Second):
		require.Fail(t, "timed out waiting for first pre-registration recv")
	}

	secondStream := newBlockingWorkerStream(ctx, rawKey)
	err := svc.StreamTasks(secondStream)
	require.Equal(t,
		codes.
			ResourceExhausted,

		status.
			Code(
				err))

	firstCancel()
	select {
	case <-firstDone:
	case <-time.After(5 * time.Second):
		require.Fail(t, "first pre-registration stream did not exit after cancellation")
	}
	require.NoError(t,

		svc.registry.
			ReservePendingStream(projectID,

				apiKeyID),
	)

	svc.registry.ReleasePendingStream(projectID, apiKeyID)
}
