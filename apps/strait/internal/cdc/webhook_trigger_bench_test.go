package cdc

import (
	"context"
	"fmt"
	"testing"

	"strait/internal/domain"
)

func BenchmarkWebhookTriggerHandleMatchingSubscriptions(b *testing.B) {
	for _, count := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("subs_%d", count), func(b *testing.B) {
			subs := make([]domain.WebhookSubscription, count)
			for i := range subs {
				subs[i] = domain.WebhookSubscription{
					ID:         fmt.Sprintf("sub-%03d", i),
					ProjectID:  "p1",
					WebhookURL: fmt.Sprintf("https://example.com/%03d", i),
					EventTypes: []string{"run.completed"},
					Secret:     "whsec",
					Active:     true,
				}
			}
			store := &mockWebhookStore{subs: subs}
			handler := NewWebhookTriggerHandler(store, nil)
			msg := cdcUpdateMsg("completed", "p1", "run-1", "job-1")

			b.ReportAllocs()
			for b.Loop() {
				store.mu.Lock()
				store.deliveries = store.deliveries[:0]
				store.mu.Unlock()
				if err := handler.Handle(context.Background(), msg); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
