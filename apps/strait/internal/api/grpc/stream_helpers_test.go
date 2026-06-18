package grpc

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type unitWorkerStream struct {
	ctx     context.Context
	recvCh  chan *workerv1.WorkerMessage
	sentCh  chan *workerv1.ServerMessage
	sendErr error
}

func newUnitWorkerStream(ctx context.Context) *unitWorkerStream {
	return &unitWorkerStream{
		ctx:    ctx,
		recvCh: make(chan *workerv1.WorkerMessage, 1),
		sentCh: make(chan *workerv1.ServerMessage, 1),
	}
}

func (s *unitWorkerStream) Context() context.Context { return s.ctx }

func (s *unitWorkerStream) Send(msg *workerv1.ServerMessage) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	select {
	case s.sentCh <- msg:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *unitWorkerStream) Recv() (*workerv1.WorkerMessage, error) {
	select {
	case msg, ok := <-s.recvCh:
		if !ok {
			return nil, io.EOF
		}
		return msg, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

func (s *unitWorkerStream) SetHeader(metadata.MD) error  { return nil }
func (s *unitWorkerStream) SendHeader(metadata.MD) error { return nil }
func (s *unitWorkerStream) SetTrailer(metadata.MD)       {}
func (s *unitWorkerStream) SendMsg(any) error            { return nil }
func (s *unitWorkerStream) RecvMsg(any) error            { return nil }

func requireLoopErr(t *testing.T, streamErr <-chan error) error {
	t.Helper()

	select {
	case err := <-streamErr:
		return err
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for stream loop result")
		return nil
	}
}

func TestEnsureAPIKeyControlSubscriptions(t *testing.T) {
	t.Parallel()

	t.Run("empty api key reuses existing subscriptions", func(t *testing.T) {
		t.Parallel()

		revokeSub, err := noopPub{}.Subscribe(t.Context(), "revoke")
		require.NoError(t, err)
		expireSub, err := noopPub{}.Subscribe(t.Context(), "expire")
		require.NoError(t, err)
		t.Cleanup(revokeSub.Close)
		t.Cleanup(expireSub.Close)

		svc := &workerService{}
		gotRevoke, gotExpire, err := svc.ensureAPIKeyControlSubscriptions(t.Context(), "", revokeSub, expireSub)
		require.NoError(t, err)
		require.Same(t, revokeSub, gotRevoke)
		require.Same(t, expireSub, gotExpire)
	})

	t.Run("subscribes missing channels", func(t *testing.T) {
		t.Parallel()

		svc := &workerService{pub: noopPub{}}
		revokeSub, expireSub, err := svc.ensureAPIKeyControlSubscriptions(t.Context(), "key-1", nil, nil)
		require.NoError(t, err)
		require.NotNil(t, revokeSub)
		require.NotNil(t, expireSub)
		t.Cleanup(revokeSub.Close)
		t.Cleanup(expireSub.Close)
	})

	t.Run("reuses existing revoke and reports expiry subscription failure", func(t *testing.T) {
		t.Parallel()

		revokeSub, err := noopPub{}.Subscribe(t.Context(), "revoke")
		require.NoError(t, err)
		t.Cleanup(revokeSub.Close)

		svc := &workerService{
			pub: controlChannelPublisher{
				subscribe: func(_ context.Context, channel string) (*pubsub.Subscription, error) {
					require.Equal(t, apiKeyExpiresChannel("key-1"), channel)
					return nil, errors.New("redis unavailable")
				},
			},
		}
		gotRevoke, expireSub, err := svc.ensureAPIKeyControlSubscriptions(t.Context(), "key-1", revokeSub, nil)
		require.Error(t, err)
		require.Equal(t, codes.Unavailable, status.Code(err))
		require.Same(t, revokeSub, gotRevoke)
		require.Nil(t, expireSub)
	})
}

func TestWorkerRegistrationFromFirstMessage(t *testing.T) {
	t.Parallel()

	reg := &workerv1.WorkerRegistration{WorkerId: "worker-1"}
	got, err := workerRegistrationFromFirstMessage(&workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_Registration{Registration: reg},
	})
	require.NoError(t, err)
	require.Same(t, reg, got)

	for _, msg := range []*workerv1.WorkerMessage{
		nil,
		{},
		{Payload: &workerv1.WorkerMessage_Heartbeat{Heartbeat: &workerv1.Heartbeat{}}},
		{Payload: &workerv1.WorkerMessage_Registration{}},
	} {
		got, err := workerRegistrationFromFirstMessage(msg)
		require.Error(t, err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.Nil(t, got)
	}
}

func TestWorkerServiceAPIKeyLookupResolver(t *testing.T) {
	t.Parallel()

	require.Nil(t, (*workerService)(nil).apiKeyLookupResolver())
	require.Nil(t, (&workerService{}).apiKeyLookupResolver())

	resolver := apiKeyResolverFunc(func(context.Context, string) (*domain.APIKey, error) {
		return &domain.APIKey{ID: "key-1"}, nil
	})
	got := (&workerService{apiKeyResolver: resolver}).apiKeyLookupResolver()
	require.NotNil(t, got)

	apiKey, err := got.LookupAPIKeyByHash(t.Context(), "hash")
	require.NoError(t, err)
	require.Equal(t, "key-1", apiKey.ID)
}

func TestWorkerStreamGoroutineCount(t *testing.T) {
	t.Parallel()

	disconnectSub, err := noopPub{}.Subscribe(t.Context(), "disconnect")
	require.NoError(t, err)
	revokeSub, err := noopPub{}.Subscribe(t.Context(), "revoke")
	require.NoError(t, err)
	t.Cleanup(disconnectSub.Close)
	t.Cleanup(revokeSub.Close)

	require.Equal(t, 2, workerStreamGoroutineCount(nil, nil, false, false))
	require.Equal(t, 3, workerStreamGoroutineCount(disconnectSub, nil, false, false))
	require.Equal(t, 3, workerStreamGoroutineCount(nil, revokeSub, false, false))
	require.Equal(t, 3, workerStreamGoroutineCount(nil, nil, true, false))
	require.Equal(t, 3, workerStreamGoroutineCount(nil, nil, false, true))
	require.Equal(t, 6, workerStreamGoroutineCount(disconnectSub, revokeSub, true, true))
}

func TestStartWorkerSendLoop(t *testing.T) {
	t.Parallel()

	t.Run("sends messages until channel closes", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		stream := newUnitWorkerStream(ctx)
		sendCh := make(chan *workerv1.ServerMessage, 1)
		streamErr := make(chan error, 1)
		var wg conc.WaitGroup

		startWorkerSendLoop(ctx, &wg, streamErr, sendCh, stream)
		msg := &workerv1.ServerMessage{
			Payload: &workerv1.ServerMessage_Ack{Ack: &workerv1.Acknowledged{Id: "worker-1"}},
		}
		sendCh <- msg
		require.Same(t, msg, <-stream.sentCh)
		close(sendCh)

		require.NoError(t, requireLoopErr(t, streamErr))
		wg.Wait()
	})

	t.Run("reports stream send error", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		sendErr := errors.New("send failed")
		stream := newUnitWorkerStream(ctx)
		stream.sendErr = sendErr
		sendCh := make(chan *workerv1.ServerMessage, 1)
		streamErr := make(chan error, 1)
		var wg conc.WaitGroup

		startWorkerSendLoop(ctx, &wg, streamErr, sendCh, stream)
		sendCh <- &workerv1.ServerMessage{}

		err := requireLoopErr(t, streamErr)
		require.ErrorIs(t, err, sendErr)
		require.ErrorContains(t, err, "send")
		wg.Wait()
	})

	t.Run("context cancellation stops loop", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		stream := newUnitWorkerStream(ctx)
		streamErr := make(chan error, 1)
		var wg conc.WaitGroup

		startWorkerSendLoop(ctx, &wg, streamErr, make(chan *workerv1.ServerMessage), stream)
		cancel()

		require.NoError(t, requireLoopErr(t, streamErr))
		wg.Wait()
	})
}

func TestStartWorkerRecvLoop(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	stream := newUnitWorkerStream(ctx)
	stream.recvCh <- &workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_Registration{Registration: &workerv1.WorkerRegistration{WorkerId: "worker-1"}},
	}
	close(stream.recvCh)
	streamErr := make(chan error, 1)
	var wg conc.WaitGroup

	svc := &workerService{}
	svc.startWorkerRecvLoop(ctx, &wg, streamErr, stream, "worker-1", "proj-1", "", "")

	require.ErrorIs(t, requireLoopErr(t, streamErr), io.EOF)
	wg.Wait()
}

func TestListenForWorkerForceDisconnect(t *testing.T) {
	t.Parallel()

	t.Run("disconnect signal closes stream", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		ch := make(chan []byte, 1)
		var closed atomic.Bool
		sub := pubsub.NewSubscription(ch, func() { closed.Store(true) })
		streamErr := make(chan error, 1)
		var wg conc.WaitGroup

		listenForWorkerForceDisconnect(ctx, &wg, streamErr, sub, "worker-1", "proj-1")
		ch <- []byte("disconnect")

		require.ErrorIs(t, requireLoopErr(t, streamErr), errForceDisconnected)
		wg.Wait()
		require.True(t, closed.Load())
	})

	t.Run("context cancellation closes subscription", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		ch := make(chan []byte)
		var closed atomic.Bool
		sub := pubsub.NewSubscription(ch, func() { closed.Store(true) })
		streamErr := make(chan error, 1)
		var wg conc.WaitGroup

		listenForWorkerForceDisconnect(ctx, &wg, streamErr, sub, "worker-1", "proj-1")
		cancel()

		require.NoError(t, requireLoopErr(t, streamErr))
		wg.Wait()
		require.True(t, closed.Load())
	})
}

func TestListenForAPIKeyRevocation(t *testing.T) {
	t.Parallel()

	t.Run("pubsub signal closes stream and worker key", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		ch := make(chan []byte, 1)
		var subClosed atomic.Bool
		sub := pubsub.NewSubscription(ch, func() { subClosed.Store(true) })
		registry := NewConnectionRegistry()
		cw := &ConnectedWorker{
			WorkerID:       "worker-1",
			ProjectID:      "proj-1",
			APIKeyID:       "key-1",
			Queues:         []string{"default"},
			SlotsTotal:     1,
			SlotsAvailable: 1,
			Status:         "active",
			SendCh:         make(chan *workerv1.ServerMessage, 1),
			revokeCh:       make(chan struct{}),
		}
		require.NoError(t, registry.Register(cw))
		svc := &workerService{registry: registry}
		streamErr := make(chan error, 1)
		var wg conc.WaitGroup

		svc.listenForAPIKeyRevocation(ctx, &wg, streamErr, sub, cw, "key-1", "worker-1", "proj-1")
		ch <- []byte("revoked")

		require.ErrorIs(t, requireLoopErr(t, streamErr), errAPIKeyRevoked)
		wg.Wait()
		require.True(t, subClosed.Load())
		select {
		case <-cw.revokeCh:
		default:
			t.Fatal("worker revoke channel was not closed")
		}
	})

	t.Run("local revoke signal closes stream", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		ch := make(chan []byte)
		sub := pubsub.NewSubscription(ch, func() {})
		cw := &ConnectedWorker{revokeCh: make(chan struct{})}
		svc := &workerService{registry: NewConnectionRegistry()}
		streamErr := make(chan error, 1)
		var wg conc.WaitGroup

		svc.listenForAPIKeyRevocation(ctx, &wg, streamErr, sub, cw, "key-1", "worker-1", "proj-1")
		close(cw.revokeCh)

		require.ErrorIs(t, requireLoopErr(t, streamErr), errAPIKeyRevoked)
		wg.Wait()
	})
}

func TestListenForAPIKeyExpiry(t *testing.T) {
	t.Parallel()

	t.Run("past deadline closes stream immediately", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		registry := NewConnectionRegistry()
		svc := &workerService{registry: registry}
		streamErr := make(chan error, 1)
		var wg conc.WaitGroup

		svc.listenForAPIKeyExpiry(ctx, &wg, streamErr, time.Now().Add(-time.Millisecond), true, nil, nil, "key-1", "worker-1", "proj-1")

		require.ErrorIs(t, requireLoopErr(t, streamErr), errAPIKeyExpired)
		wg.Wait()
	})

	t.Run("invalid expiry signal closes stream", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		ch := make(chan []byte, 1)
		var subClosed atomic.Bool
		sub := pubsub.NewSubscription(ch, func() { subClosed.Store(true) })
		svc := &workerService{registry: NewConnectionRegistry()}
		streamErr := make(chan error, 1)
		var wg conc.WaitGroup

		svc.listenForAPIKeyExpiry(ctx, &wg, streamErr, time.Time{}, false, sub, nil, "key-1", "worker-1", "proj-1")
		ch <- []byte("invalid")

		require.ErrorIs(t, requireLoopErr(t, streamErr), errAPIKeyExpired)
		wg.Wait()
		require.True(t, subClosed.Load())
	})

	t.Run("future deadline closes stream when timer fires", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		svc := &workerService{registry: NewConnectionRegistry()}
		streamErr := make(chan error, 1)
		var wg conc.WaitGroup

		svc.listenForAPIKeyExpiry(ctx, &wg, streamErr, time.Now().Add(10*time.Millisecond), true, nil, nil, "key-1", "worker-1", "proj-1")

		require.ErrorIs(t, requireLoopErr(t, streamErr), errAPIKeyExpired)
		wg.Wait()
	})
}

func TestWorkerServiceWorkerConnectionRenewer(t *testing.T) {
	t.Parallel()

	enforcer := &releaseRecordingReservationEnforcer{}
	svc := &workerService{billingEnforcer: enforcer}

	require.Nil(t, (&workerService{}).workerConnectionRenewer(registeredWorkerStream{
		orgID:                         "org-1",
		workerConnectionReservationID: "reservation-1",
	}))
	require.Nil(t, svc.workerConnectionRenewer(registeredWorkerStream{
		workerConnectionReservationID: "reservation-1",
	}))
	require.Nil(t, svc.workerConnectionRenewer(registeredWorkerStream{
		orgID: "org-1",
	}))
	require.Same(t, enforcer, svc.workerConnectionRenewer(registeredWorkerStream{
		orgID:                         "org-1",
		workerConnectionReservationID: "reservation-1",
	}))
}

func TestWorkerConnectionLease(t *testing.T) {
	t.Parallel()

	require.Equal(t, 90*time.Second, (&workerService{}).workerConnectionLease())
	require.Equal(t, 90*time.Second, (&workerService{cfg: &config.Config{}}).workerConnectionLease())
	require.Equal(t, 45*time.Second, (&workerService{
		cfg: &config.Config{WorkerHeartbeatTimeout: 15 * time.Second},
	}).workerConnectionLease())
}

func TestReservePendingWorkerStream(t *testing.T) {
	t.Parallel()

	registry := NewConnectionRegistry()
	registry.maxStreamsPerProject = 1
	registry.maxStreamsPerAPIKey = 1
	svc := &workerService{registry: registry}

	release, err := svc.reservePendingWorkerStream("proj-1", "key-1")
	require.NoError(t, err)
	require.NotNil(t, release)

	_, err = svc.reservePendingWorkerStream("proj-1", "key-2")
	require.Error(t, err)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))

	release()
	release, err = svc.reservePendingWorkerStream("proj-1", "key-2")
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
}

func TestParseAPIKeyExpirySignal(t *testing.T) {
	t.Parallel()

	deadline := time.Date(2026, 6, 18, 12, 30, 0, 123, time.UTC)
	got, err := parseAPIKeyExpirySignal([]byte(" \n" + deadline.Format(time.RFC3339Nano) + "\t"))
	require.NoError(t, err)
	require.Equal(t, deadline, got)

	got, err = parseAPIKeyExpirySignal([]byte("not-a-time"))
	require.Error(t, err)
	require.True(t, got.IsZero())
}

func TestRecvWorkerRegistrationMessage(t *testing.T) {
	t.Parallel()

	t.Run("without control channels receives directly", func(t *testing.T) {
		t.Parallel()

		stream := newUnitWorkerStream(t.Context())
		msg := &workerv1.WorkerMessage{
			Payload: &workerv1.WorkerMessage_Registration{Registration: &workerv1.WorkerRegistration{WorkerId: "worker-1"}},
		}
		stream.recvCh <- msg

		got, err := recvWorkerRegistrationMessage(t.Context(), stream, nil, "key-1", time.Time{}, false)
		require.NoError(t, err)
		require.Same(t, msg, got)
	})

	t.Run("already expired api key rejects before receive", func(t *testing.T) {
		t.Parallel()

		stream := newUnitWorkerStream(t.Context())

		got, err := recvWorkerRegistrationMessage(t.Context(), stream, nil, "key-1", time.Now().Add(-time.Millisecond), true)
		require.ErrorIs(t, err, errAPIKeyExpired)
		require.Nil(t, got)
	})

	t.Run("revocation before receive rejects registration", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		stream := newUnitWorkerStream(ctx)
		revokeCh := make(chan []byte, 1)
		revokeSub := pubsub.NewSubscription(revokeCh, func() {})
		revokeCh <- []byte("revoked")

		got, err := recvWorkerRegistrationMessage(ctx, stream, revokeSub, "key-1", time.Now().Add(time.Minute), true)
		cancel()
		require.ErrorIs(t, err, errAPIKeyRevoked)
		require.Nil(t, got)
	})

	t.Run("context cancellation wins while receive is blocked", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		stream := newUnitWorkerStream(ctx)
		revokeCh := make(chan []byte)
		revokeSub := pubsub.NewSubscription(revokeCh, func() {})
		cancel()

		got, err := recvWorkerRegistrationMessage(ctx, stream, revokeSub, "key-1", time.Now().Add(time.Minute), true)
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, got)
	})
}

func TestStreamDisconnectReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", want: "graceful"},
		{name: "deadline", err: context.DeadlineExceeded, want: "timeout"},
		{name: "cancelled", err: context.Canceled, want: "graceful"},
		{name: "forced", err: errForceDisconnected, want: "forced"},
		{name: "revoked", err: errAPIKeyRevoked, want: "revoked"},
		{name: "expired", err: errAPIKeyExpired, want: "expired"},
		{name: "reservation renewal", err: errWorkerConnectionRenewalFailed, want: "worker_connection_reservation_lost"},
		{name: "other", err: errors.New("boom"), want: "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, streamDisconnectReason(tt.err))
		})
	}
}

func TestNewConnectedWorkerFromRegistration(t *testing.T) {
	t.Parallel()

	sendCh := make(chan *workerv1.ServerMessage, 1)
	reg := &workerv1.WorkerRegistration{
		WorkerId:       "worker-1",
		Name:           "worker name",
		Hostname:       "host-1",
		SdkVersion:     "v1.2.3",
		SdkLanguage:    "go",
		Queues:         []string{"default"},
		SlotsTotal:     4,
		SlotsAvailable: 3,
	}
	apiKey := &domain.APIKey{ID: "key-1", EnvironmentID: "env-1"}

	worker := newConnectedWorkerFromRegistration(reg, apiKey, "proj-1", "org-1", sendCh)
	require.Equal(t, "worker-1", worker.WorkerID)
	require.Equal(t, "proj-1", worker.ProjectID)
	require.Equal(t, "org-1", worker.OrgID)
	require.Equal(t, "env-1", worker.EnvironmentID)
	require.Equal(t, "key-1", worker.APIKeyID)
	require.Equal(t, "worker name", worker.Name)
	require.Equal(t, "host-1", worker.Hostname)
	require.Equal(t, "v1.2.3", worker.SDKVersion)
	require.Equal(t, "go", worker.SDKLanguage)
	require.Equal(t, []string{"default"}, worker.Queues)
	require.EqualValues(t, 4, worker.SlotsTotal)
	require.EqualValues(t, 3, worker.SlotsAvailable)
	require.Equal(t, "active", worker.Status)
	msg := &workerv1.ServerMessage{}
	worker.SendCh <- msg
	require.Same(t, msg, <-sendCh)
	require.NotNil(t, worker.revokeCh)
}

func TestConnectedWorkerRefsSkipsIncompleteWorkers(t *testing.T) {
	t.Parallel()

	require.Nil(t, connectedWorkerRefs(nil))

	registry := NewConnectionRegistry()
	require.NoError(t, registry.Register(&ConnectedWorker{
		WorkerID:       "worker-ok",
		ProjectID:      "project-1",
		APIKeyID:       "key-ok",
		Queues:         []string{"default"},
		SlotsTotal:     1,
		SlotsAvailable: 1,
		Status:         "active",
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}))
	require.NoError(t, registry.Register(&ConnectedWorker{
		WorkerID:       "",
		ProjectID:      "project-1",
		APIKeyID:       "key-empty-worker",
		Queues:         []string{"default"},
		SlotsTotal:     1,
		SlotsAvailable: 1,
		Status:         "active",
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}))
	require.NoError(t, registry.Register(&ConnectedWorker{
		WorkerID:       "worker-empty-project",
		ProjectID:      "",
		APIKeyID:       "key-empty-project",
		Queues:         []string{"default"},
		SlotsTotal:     1,
		SlotsAvailable: 1,
		Status:         "active",
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}))

	require.ElementsMatch(t, []store.ActiveWorkerRef{
		{WorkerID: "worker-ok", ProjectID: "project-1"},
	}, connectedWorkerRefs(registry))
}
