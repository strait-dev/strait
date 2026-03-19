package notification

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

type fakeNotificationStore struct {
	mu         sync.Mutex
	channel    *domain.NotificationChannel
	deliveries map[string]*domain.NotificationDelivery
}

func newFakeNotificationStore(channel *domain.NotificationChannel, deliveries ...*domain.NotificationDelivery) *fakeNotificationStore {
	store := &fakeNotificationStore{
		channel:    channel,
		deliveries: make(map[string]*domain.NotificationDelivery, len(deliveries)),
	}
	for _, delivery := range deliveries {
		copyDelivery := *delivery
		store.deliveries[delivery.ID] = &copyDelivery
	}
	return store
}

func (f *fakeNotificationStore) CreateNotificationChannel(_ context.Context, _ *domain.NotificationChannel) error {
	return errors.New("not implemented")
}

func (f *fakeNotificationStore) GetNotificationChannel(_ context.Context, id, projectID string) (*domain.NotificationChannel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.channel == nil || f.channel.ID != id || f.channel.ProjectID != projectID {
		return nil, errors.New("channel not found")
	}
	channelCopy := *f.channel
	return &channelCopy, nil
}

func (f *fakeNotificationStore) ListNotificationChannels(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeNotificationStore) ListEnabledNotificationChannels(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeNotificationStore) UpdateNotificationChannel(_ context.Context, _ *domain.NotificationChannel) error {
	return errors.New("not implemented")
}

func (f *fakeNotificationStore) DeleteNotificationChannel(_ context.Context, _, _ string) error {
	return errors.New("not implemented")
}

func (f *fakeNotificationStore) CreateNotificationDelivery(_ context.Context, _ *domain.NotificationDelivery) error {
	return errors.New("not implemented")
}

func (f *fakeNotificationStore) ClaimPendingNotificationDeliveries(_ context.Context, limit int, leaseDuration time.Duration) ([]domain.NotificationDelivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now().UTC()
	deliveries := make([]domain.NotificationDelivery, 0, limit)
	for _, delivery := range f.deliveries {
		if len(deliveries) >= limit {
			break
		}

		switch {
		case delivery.Status == "pending" && (delivery.NextRetryAt == nil || !delivery.NextRetryAt.After(now)):
		case delivery.Status == "processing" && delivery.LeaseExpiry != nil && !delivery.LeaseExpiry.After(now):
		default:
			continue
		}

		token := delivery.ID + "-claim"
		leaseExpiry := now.Add(leaseDuration)
		delivery.Status = "processing"
		delivery.ClaimToken = token
		delivery.LeaseExpiry = &leaseExpiry

		copyDelivery := *delivery
		deliveries = append(deliveries, copyDelivery)
	}

	return deliveries, nil
}

func (f *fakeNotificationStore) UpdateClaimedNotificationDelivery(_ context.Context, d *domain.NotificationDelivery) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	current := f.deliveries[d.ID]
	if current == nil || current.ClaimToken != d.ClaimToken {
		return false, nil
	}

	copyDelivery := *d
	copyDelivery.ClaimToken = ""
	copyDelivery.LeaseExpiry = nil
	f.deliveries[d.ID] = &copyDelivery
	return true, nil
}

func (f *fakeNotificationStore) ListNotificationDeliveries(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.NotificationDelivery, error) {
	return nil, errors.New("not implemented")
}

type fakeChannelSender struct {
	calls atomic.Int64
}

func (f *fakeChannelSender) Send(_ context.Context, _ *domain.NotificationChannel, _ *domain.NotificationDelivery) error {
	f.calls.Add(1)
	return nil
}

func TestWorkerProcessClaimsEachDeliveryOnce(t *testing.T) {
	t.Parallel()

	channel := &domain.NotificationChannel{
		ID:          "ch-1",
		ProjectID:   "proj-1",
		ChannelType: domain.ChannelTypeWebhook,
	}
	delivery := &domain.NotificationDelivery{
		ID:          "del-1",
		ChannelID:   channel.ID,
		ProjectID:   channel.ProjectID,
		EventType:   domain.NotificationEventApprovalRequested,
		Status:      "pending",
		MaxAttempts: 3,
	}

	store := newFakeNotificationStore(channel, delivery)
	sender := &fakeChannelSender{}

	workerOne := NewWorker(store, nil)
	workerOne.senders = map[string]ChannelSender{domain.ChannelTypeWebhook: sender}
	workerTwo := NewWorker(store, nil)
	workerTwo.senders = map[string]ChannelSender{domain.ChannelTypeWebhook: sender}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		workerOne.process(context.Background())
	}()
	go func() {
		defer wg.Done()
		workerTwo.process(context.Background())
	}()
	wg.Wait()

	if got := sender.calls.Load(); got != 1 {
		t.Fatalf("sender calls = %d, want 1", got)
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	updated := store.deliveries[delivery.ID]
	if updated == nil {
		t.Fatal("delivery missing after processing")
	}
	if updated.Status != "delivered" {
		t.Fatalf("delivery status = %q, want delivered", updated.Status)
	}
	if updated.ClaimToken != "" {
		t.Fatalf("delivery claim_token = %q, want empty", updated.ClaimToken)
	}
	if updated.LeaseExpiry != nil {
		t.Fatalf("delivery lease expiry = %v, want nil", updated.LeaseExpiry)
	}
}
