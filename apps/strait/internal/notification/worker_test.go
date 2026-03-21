package notification

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// stubNotificationStore implements store.NotificationStore with no-ops.
type stubNotificationStore struct{}

func (s *stubNotificationStore) CreateNotificationChannel(_ context.Context, _ *domain.NotificationChannel) error {
	return nil
}
func (s *stubNotificationStore) GetNotificationChannel(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
	return nil, store.ErrNotificationChannelNotFound
}
func (s *stubNotificationStore) ListNotificationChannels(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
	return nil, nil
}
func (s *stubNotificationStore) ListEnabledNotificationChannels(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
	return nil, nil
}
func (s *stubNotificationStore) UpdateNotificationChannel(_ context.Context, _ *domain.NotificationChannel) error {
	return nil
}
func (s *stubNotificationStore) DeleteNotificationChannel(_ context.Context, _, _ string) error {
	return nil
}
func (s *stubNotificationStore) CreateNotificationDelivery(_ context.Context, _ *domain.NotificationDelivery) error {
	return nil
}
func (s *stubNotificationStore) ClaimPendingNotificationDeliveries(_ context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
	return nil, nil
}
func (s *stubNotificationStore) UpdateClaimedNotificationDelivery(_ context.Context, _ *domain.NotificationDelivery) (bool, error) {
	return false, nil
}
func (s *stubNotificationStore) ListNotificationDeliveries(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.NotificationDelivery, error) {
	return nil, nil
}

func TestWorker_StopTwice_NoPanic(t *testing.T) {
	t.Parallel()
	w := NewWorker(&stubNotificationStore{}, &http.Client{})
	w.Start(t.Context())
	w.Stop()
	w.Stop() // must not panic
}

func TestWorker_StopBeforeStart_NoPanic(t *testing.T) {
	t.Parallel()
	w := NewWorker(&stubNotificationStore{}, &http.Client{})
	w.Stop() // must not panic without Start
}

func TestWorker_StopIsIdempotent(t *testing.T) {
	t.Parallel()
	w := NewWorker(&stubNotificationStore{}, &http.Client{})
	w.Start(t.Context())

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			w.Stop()
		})
	}
	wg.Wait() // must not panic
}

// panicNotificationStore panics on ClaimPendingNotificationDeliveries to test recovery.
type panicNotificationStore struct {
	stubNotificationStore
}

func (s *panicNotificationStore) ClaimPendingNotificationDeliveries(_ context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
	panic("test panic in ClaimPendingNotificationDeliveries")
}

func TestWorker_PanicRecovery(t *testing.T) {
	t.Parallel()
	w := NewWorker(&panicNotificationStore{}, &http.Client{})

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	// Give the worker time to process (and panic/recover).
	time.Sleep(200 * time.Millisecond)

	// If panic recovery works, Stop should not hang.
	cancel()
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success: worker stopped despite panic
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after panic; recovery may not be working")
	}
}
