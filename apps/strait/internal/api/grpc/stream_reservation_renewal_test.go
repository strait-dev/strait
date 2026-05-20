package grpc

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/config"

	"github.com/sourcegraph/conc"
)

type reservationRenewalRecorder struct {
	renewCalls atomic.Int64
	failOnCall int64
}

func (r *reservationRenewalRecorder) ReserveWorkerConnection(context.Context, string, string, time.Duration) (func(), error) {
	return func() {}, nil
}

func (r *reservationRenewalRecorder) RenewWorkerConnection(context.Context, string, string, time.Duration) error {
	call := r.renewCalls.Add(1)
	if r.failOnCall > 0 && call >= r.failOnCall {
		return errors.New("redis renewal failed")
	}
	return nil
}

func TestWorkerConnectionReservationRenewal_RenewsWithoutHeartbeat(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	recorder := &reservationRenewalRecorder{}
	svc := &workerService{cfg: &config.Config{WorkerHeartbeatTimeout: 30 * time.Millisecond}}
	streamErr := make(chan error, 4)
	var wg conc.WaitGroup
	svc.startWorkerConnectionReservationRenewal(ctx, &wg, streamErr, recorder, "org-1", "reservation-1", "worker-1")

	deadline := time.After(250 * time.Millisecond)
	for {
		if recorder.renewCalls.Load() >= 2 {
			cancel()
			return
		}
		select {
		case err := <-streamErr:
			t.Fatalf("unexpected stream error before renewal: %v", err)
		case <-deadline:
			t.Fatalf("renew calls = %d, want at least 2", recorder.renewCalls.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestWorkerConnectionReservationRenewal_FailureClosesStream(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	recorder := &reservationRenewalRecorder{failOnCall: 1}
	svc := &workerService{cfg: &config.Config{WorkerHeartbeatTimeout: 30 * time.Millisecond}}
	streamErr := make(chan error, 4)
	var wg conc.WaitGroup
	svc.startWorkerConnectionReservationRenewal(ctx, &wg, streamErr, recorder, "org-1", "reservation-1", "worker-1")

	select {
	case err := <-streamErr:
		if !errors.Is(err, errWorkerConnectionRenewalFailed) {
			t.Fatalf("stream error = %v, want %v", err, errWorkerConnectionRenewalFailed)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for renewal failure to close stream")
	}
}
