package webhook

import (
	"context"
	"sync"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
)

// TestBillingDispatcher_ConcurrentDispatch_NoRaces fans 50 goroutines at the
// same dispatcher with the same store and asserts that the dispatcher itself
// (including the org→project→subscription resolution and the underlying
// DeliveryWorker.EnqueueSubscriptionWebhooks call) is safe to call
// concurrently. The mockDeliveryStore is mutex-guarded so we can also verify
// the total enqueue count matches goroutines × subs.
//
// Run with -race to catch reads/writes the dispatcher might add on a future
// edit; the count assertion guards against silent drops.
func TestBillingDispatcher_ConcurrentDispatch_NoRaces(t *testing.T) {
	t.Parallel()

	const (
		goroutines  = 50
		dispatchPer = 1
	)

	orgID := "org-race"
	projectID := "proj-race"
	subs := map[string][]domain.WebhookSubscription{
		projectID: {
			{
				ID:         "sub-a",
				ProjectID:  projectID,
				WebhookURL: "http://a.example.com",
				EventTypes: []string{domain.WebhookEventBillingCapWarning},
				Active:     true,
			},
			{
				ID:         "sub-b",
				ProjectID:  projectID,
				WebhookURL: "http://b.example.com",
				EventTypes: []string{"*"},
				Active:     true,
			},
			{
				ID:         "sub-c-skip",
				ProjectID:  projectID,
				WebhookURL: "http://c.example.com",
				EventTypes: []string{domain.WebhookEventBillingCapReached},
				Active:     true,
			},
		},
	}

	d, ms := newDispatcherFixture(t,
		map[string][]string{orgID: {projectID}},
		subs,
	)

	payload := []byte(`{"event":"billing.cap_warning","org_id":"org-race"}`)

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for range goroutines {
		wg.Go(func() {
			for range dispatchPer {
				if err := d.DispatchBillingEvent(
					context.Background(),
					orgID,
					domain.WebhookEventBillingCapWarning,
					payload,
				); err != nil {
					errs <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		assert.Failf(t, "test failure", "DispatchBillingEvent error: %v", err)
	}

	want := goroutines * dispatchPer * 2 // sub-a and sub-b match; sub-c-skip does not.
	deliveries := ms.getDeliveries()
	assert.Len(t, deliveries,

		want)

	for _, d := range deliveries {
		assert.NotEqual(t,
			"sub-c-skip",
			d.SubscriptionID,
		)

	}
}
