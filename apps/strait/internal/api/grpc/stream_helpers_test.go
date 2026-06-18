package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
