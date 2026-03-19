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
