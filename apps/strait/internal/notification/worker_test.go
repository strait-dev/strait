package notification

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
)

type fakeNotificationStore struct {
	mu                    sync.Mutex
	channel               *domain.NotificationChannel
	deliveries            map[string]*domain.NotificationDelivery
	order                 []string
	claimLimits           []int
	claimTokenSequence    int
	leaseDurationOverride *time.Duration
}

func newFakeNotificationStore(channel *domain.NotificationChannel, deliveries ...*domain.NotificationDelivery) *fakeNotificationStore {
	store := &fakeNotificationStore{
		channel:    channel,
		deliveries: make(map[string]*domain.NotificationDelivery, len(deliveries)),
	}
	for _, delivery := range deliveries {
		copyDelivery := *delivery
		store.deliveries[delivery.ID] = &copyDelivery
		store.order = append(store.order, delivery.ID)
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

	f.claimLimits = append(f.claimLimits, limit)

	if f.leaseDurationOverride != nil {
		leaseDuration = *f.leaseDurationOverride
	}

	now := time.Now().UTC()
	deliveries := make([]domain.NotificationDelivery, 0, limit)
	for _, id := range f.order {
		delivery := f.deliveries[id]
		if len(deliveries) >= limit {
			break
		}

		switch {
		case delivery.Status == "pending" && (delivery.NextRetryAt == nil || !delivery.NextRetryAt.After(now)):
		case delivery.Status == "processing" && delivery.LeaseExpiry != nil && !delivery.LeaseExpiry.After(now):
		default:
			continue
		}

		f.claimTokenSequence++
		token := fmt.Sprintf("%s-claim-%d", delivery.ID, f.claimTokenSequence)
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
	mu    sync.Mutex
	delay time.Duration
	total int
	byID  map[string]int
}

func newFakeChannelSender(delay time.Duration) *fakeChannelSender {
	return &fakeChannelSender{
		delay: delay,
		byID:  make(map[string]int),
	}
}

func (f *fakeChannelSender) Send(ctx context.Context, _ *domain.NotificationChannel, d *domain.NotificationDelivery) error {
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(f.delay):
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.total++
	f.byID[d.ID]++
	return nil
}

func (f *fakeChannelSender) TotalCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.total
}

func (f *fakeChannelSender) CallsFor(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byID[id]
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
	sender := newFakeChannelSender(0)

	workerOne := NewWorker(store, nil)
	workerOne.senders = map[string]ChannelSender{domain.ChannelTypeWebhook: sender}
	workerTwo := NewWorker(store, nil)
	workerTwo.senders = map[string]ChannelSender{domain.ChannelTypeWebhook: sender}

	var wg sync.WaitGroup
	wg.Go(func() {
		workerOne.process(context.Background())
	})
	wg.Go(func() {
		workerTwo.process(context.Background())
	})
	wg.Wait()

	if got := sender.TotalCalls(); got != 1 {
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

func TestWorkerProcessClaimsOneDeliveryPerIteration(t *testing.T) {
	t.Parallel()

	channel := &domain.NotificationChannel{
		ID:          "ch-1",
		ProjectID:   "proj-1",
		ChannelType: domain.ChannelTypeWebhook,
	}
	store := newFakeNotificationStore(
		channel,
		&domain.NotificationDelivery{ID: "del-1", ChannelID: channel.ID, ProjectID: channel.ProjectID, EventType: domain.NotificationEventApprovalRequested, Status: "pending", MaxAttempts: 3},
		&domain.NotificationDelivery{ID: "del-2", ChannelID: channel.ID, ProjectID: channel.ProjectID, EventType: domain.NotificationEventApprovalRequested, Status: "pending", MaxAttempts: 3},
		&domain.NotificationDelivery{ID: "del-3", ChannelID: channel.ID, ProjectID: channel.ProjectID, EventType: domain.NotificationEventApprovalRequested, Status: "pending", MaxAttempts: 3},
	)
	sender := newFakeChannelSender(0)

	worker := NewWorker(store, nil)
	worker.senders = map[string]ChannelSender{domain.ChannelTypeWebhook: sender}
	worker.process(context.Background())

	if got := sender.TotalCalls(); got != 3 {
		t.Fatalf("sender calls = %d, want 3", got)
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.claimLimits) != 4 {
		t.Fatalf("claim call count = %d, want 4", len(store.claimLimits))
	}
	for i, limit := range store.claimLimits {
		if limit != 1 {
			t.Fatalf("claim limit at call %d = %d, want 1", i, limit)
		}
	}
}

func TestWorkerProcessDoesNotDuplicateSlowMultiDeliveryBatches(t *testing.T) {
	t.Parallel()

	channel := &domain.NotificationChannel{
		ID:          "ch-1",
		ProjectID:   "proj-1",
		ChannelType: domain.ChannelTypeWebhook,
	}
	store := newFakeNotificationStore(
		channel,
		&domain.NotificationDelivery{ID: "del-1", ChannelID: channel.ID, ProjectID: channel.ProjectID, EventType: domain.NotificationEventApprovalRequested, Status: "pending", MaxAttempts: 3},
		&domain.NotificationDelivery{ID: "del-2", ChannelID: channel.ID, ProjectID: channel.ProjectID, EventType: domain.NotificationEventApprovalRequested, Status: "pending", MaxAttempts: 3},
		&domain.NotificationDelivery{ID: "del-3", ChannelID: channel.ID, ProjectID: channel.ProjectID, EventType: domain.NotificationEventApprovalRequested, Status: "pending", MaxAttempts: 3},
		&domain.NotificationDelivery{ID: "del-4", ChannelID: channel.ID, ProjectID: channel.ProjectID, EventType: domain.NotificationEventApprovalRequested, Status: "pending", MaxAttempts: 3},
	)
	leaseDuration := 20 * time.Millisecond
	store.leaseDurationOverride = &leaseDuration
	sender := newFakeChannelSender(15 * time.Millisecond)

	workerOne := NewWorker(store, nil)
	workerOne.senders = map[string]ChannelSender{domain.ChannelTypeWebhook: sender}
	workerTwo := NewWorker(store, nil)
	workerTwo.senders = map[string]ChannelSender{domain.ChannelTypeWebhook: sender}

	var wg sync.WaitGroup
	wg.Go(func() {
		workerOne.process(context.Background())
	})

	time.Sleep(25 * time.Millisecond)

	wg.Go(func() {
		workerTwo.process(context.Background())
	})
	wg.Wait()

	if got := sender.TotalCalls(); got != 4 {
		t.Fatalf("sender calls = %d, want 4", got)
	}

	for _, deliveryID := range []string{"del-1", "del-2", "del-3", "del-4"} {
		if got := sender.CallsFor(deliveryID); got != 1 {
			t.Fatalf("sender calls for %s = %d, want 1", deliveryID, got)
		}
	}
}
