package grpc

import (
	"errors"
	"testing"
)

func TestRegistry_Register_ProjectStreamQuota(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 2
	r.maxStreamsPerAPIKey = 10

	if err := r.Register(makeWorker("w1", "proj-a", "key-1", []string{"q"}, 1)); err != nil {
		t.Fatalf("register w1: %v", err)
	}
	if err := r.Register(makeWorker("w2", "proj-a", "key-2", []string{"q"}, 1)); err != nil {
		t.Fatalf("register w2: %v", err)
	}
	err := r.Register(makeWorker("w3", "proj-a", "key-3", []string{"q"}, 1))
	if !errors.Is(err, ErrWorkerStreamQuotaExceeded) {
		t.Fatalf("register over project quota error = %v, want ErrWorkerStreamQuotaExceeded", err)
	}
	if err := r.Register(makeWorker("w4", "proj-b", "key-4", []string{"q"}, 1)); err != nil {
		t.Fatalf("separate project should not share quota: %v", err)
	}
}

func TestRegistry_Register_APIKeyStreamQuota(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 10
	r.maxStreamsPerAPIKey = 2

	if err := r.Register(makeWorker("w1", "proj-a", "key-1", []string{"q"}, 1)); err != nil {
		t.Fatalf("register w1: %v", err)
	}
	if err := r.Register(makeWorker("w2", "proj-a", "key-1", []string{"q"}, 1)); err != nil {
		t.Fatalf("register w2: %v", err)
	}
	err := r.Register(makeWorker("w3", "proj-a", "key-1", []string{"q"}, 1))
	if !errors.Is(err, ErrWorkerStreamQuotaExceeded) {
		t.Fatalf("register over api-key quota error = %v, want ErrWorkerStreamQuotaExceeded", err)
	}
	if err := r.Register(makeWorker("w4", "proj-a", "key-2", []string{"q"}, 1)); err != nil {
		t.Fatalf("separate api key should not share quota: %v", err)
	}
}

func TestRegistry_Register_ReconnectBypassesQuotaForSameWorker(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 1
	r.maxStreamsPerAPIKey = 1

	if err := r.Register(makeWorker("w1", "proj-a", "key-1", []string{"q"}, 1)); err != nil {
		t.Fatalf("register w1: %v", err)
	}
	if err := r.Register(makeWorker("w1", "proj-a", "key-1", []string{"q"}, 1)); err != nil {
		t.Fatalf("same worker reconnect should replace instead of hitting quota: %v", err)
	}
	err := r.Register(makeWorker("w2", "proj-a", "key-1", []string{"q"}, 1))
	if !errors.Is(err, ErrWorkerStreamQuotaExceeded) {
		t.Fatalf("register second worker over quota error = %v, want ErrWorkerStreamQuotaExceeded", err)
	}
}

func TestRegistry_ReservePendingStream_CountsTowardAPIKeyQuota(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 10
	r.maxStreamsPerAPIKey = 1

	if err := r.ReservePendingStream("proj-a", "key-1"); err != nil {
		t.Fatalf("reserve pending stream: %v", err)
	}
	if err := r.ReservePendingStream("proj-a", "key-1"); !errors.Is(err, ErrWorkerStreamQuotaExceeded) {
		t.Fatalf("second pending reserve error = %v, want ErrWorkerStreamQuotaExceeded", err)
	}

	r.ReleasePendingStream("proj-a", "key-1")
	if err := r.ReservePendingStream("proj-a", "key-1"); err != nil {
		t.Fatalf("reserve after release: %v", err)
	}
}

func TestRegistry_ReservePendingStream_CountsTowardProjectQuota(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 1
	r.maxStreamsPerAPIKey = 10

	if err := r.ReservePendingStream("proj-a", "key-1"); err != nil {
		t.Fatalf("reserve pending stream: %v", err)
	}
	if err := r.ReservePendingStream("proj-a", "key-2"); !errors.Is(err, ErrWorkerStreamQuotaExceeded) {
		t.Fatalf("second project reserve error = %v, want ErrWorkerStreamQuotaExceeded", err)
	}
	if err := r.ReservePendingStream("proj-b", "key-2"); err != nil {
		t.Fatalf("separate project should not share pending quota: %v", err)
	}
}

func TestRegistry_ReservePendingStream_ActiveStreamsCountTowardQuota(t *testing.T) {
	t.Parallel()

	r := NewConnectionRegistry()
	r.maxStreamsPerProject = 10
	r.maxStreamsPerAPIKey = 1

	if err := r.Register(makeWorker("w1", "proj-a", "key-1", []string{"q"}, 1)); err != nil {
		t.Fatalf("register active worker: %v", err)
	}
	if err := r.ReservePendingStream("proj-a", "key-1"); !errors.Is(err, ErrWorkerStreamQuotaExceeded) {
		t.Fatalf("pending over active api-key quota error = %v, want ErrWorkerStreamQuotaExceeded", err)
	}
}
