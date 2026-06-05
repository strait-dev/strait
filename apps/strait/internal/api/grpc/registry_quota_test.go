package grpc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistry_Register_ProjectStreamQuota(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 2
	r.maxStreamsPerAPIKey = 10
	require.NoError(t, r.Register(makeWorker("w1", "proj-a", "key-1", []string{"q"}, 1)))
	require.NoError(t, r.Register(makeWorker("w2", "proj-a", "key-2", []string{"q"}, 1)))

	err := r.Register(makeWorker("w3", "proj-a", "key-3", []string{"q"}, 1))
	require.True(
		t, errors.Is(
			err,
			ErrWorkerStreamQuotaExceeded))
	require.NoError(t, r.Register(makeWorker("w4", "proj-b", "key-4", []string{"q"}, 1)))

}

func TestRegistry_Register_APIKeyStreamQuota(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 10
	r.maxStreamsPerAPIKey = 2
	require.NoError(t, r.Register(makeWorker("w1", "proj-a", "key-1", []string{"q"}, 1)))
	require.NoError(t, r.Register(makeWorker("w2", "proj-a", "key-1", []string{"q"}, 1)))

	err := r.Register(makeWorker("w3", "proj-a", "key-1", []string{"q"}, 1))
	require.True(
		t, errors.Is(
			err,
			ErrWorkerStreamQuotaExceeded))
	require.NoError(t, r.Register(makeWorker("w4", "proj-a", "key-2", []string{"q"}, 1)))

}

func TestRegistry_Register_ReconnectBypassesQuotaForSameWorker(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 1
	r.maxStreamsPerAPIKey = 1
	require.NoError(t, r.Register(makeWorker("w1", "proj-a", "key-1", []string{"q"}, 1)))
	require.NoError(t, r.Register(makeWorker("w1", "proj-a", "key-1", []string{"q"}, 1)))

	err := r.Register(makeWorker("w2", "proj-a", "key-1", []string{"q"}, 1))
	require.True(
		t, errors.Is(
			err,
			ErrWorkerStreamQuotaExceeded))

}

func TestRegistry_ReservePendingStream_CountsTowardAPIKeyQuota(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 10
	r.maxStreamsPerAPIKey = 1
	require.NoError(t, r.ReservePendingStream("proj-a", "key-1"))

	if err := r.ReservePendingStream("proj-a", "key-1"); !errors.Is(err, ErrWorkerStreamQuotaExceeded) {
		require.Failf(t, "test failure",

			"second pending reserve error = %v, want ErrWorkerStreamQuotaExceeded", err)
	}

	r.ReleasePendingStream("proj-a", "key-1")
	require.NoError(t, r.ReservePendingStream("proj-a", "key-1"))

}

func TestRegistry_ReservePendingStream_CountsTowardProjectQuota(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 1
	r.maxStreamsPerAPIKey = 10
	require.NoError(t, r.ReservePendingStream("proj-a", "key-1"))

	if err := r.ReservePendingStream("proj-a", "key-2"); !errors.Is(err, ErrWorkerStreamQuotaExceeded) {
		require.Failf(t, "test failure",

			"second project reserve error = %v, want ErrWorkerStreamQuotaExceeded", err)
	}
	require.NoError(t, r.ReservePendingStream("proj-b", "key-2"))

}

func TestRegistry_ReservePendingStream_ActiveStreamsCountTowardQuota(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 10
	r.maxStreamsPerAPIKey = 1
	require.NoError(t, r.Register(makeWorker("w1", "proj-a", "key-1", []string{"q"}, 1)))

	if err := r.ReservePendingStream("proj-a", "key-1"); !errors.Is(err, ErrWorkerStreamQuotaExceeded) {
		require.Failf(t, "test failure",

			"pending over active api-key quota error = %v, want ErrWorkerStreamQuotaExceeded", err)
	}
}
