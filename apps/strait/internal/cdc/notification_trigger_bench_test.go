package cdc

import (
	"context"
	"fmt"
	"testing"

	"strait/internal/domain"
)

func BenchmarkNotificationTriggerHandleEnabledChannels(b *testing.B) {
	for _, count := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("channels_%d", count), func(b *testing.B) {
			channels := make([]domain.NotificationChannel, count)
			for i := range channels {
				channels[i] = domain.NotificationChannel{
					ID:          fmt.Sprintf("ch-%03d", i),
					ProjectID:   "p1",
					ChannelType: "slack",
					Enabled:     true,
				}
			}
			store := &mockNotificationStore{channels: channels}
			handler := NewNotificationTriggerHandler(store, nil)
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

func BenchmarkNotificationTriggerHandleNoDeliveries(b *testing.B) {
	for _, count := range []int{0, 100} {
		b.Run(fmt.Sprintf("disabled_%d", count), func(b *testing.B) {
			channels := make([]domain.NotificationChannel, count)
			for i := range channels {
				channels[i] = domain.NotificationChannel{
					ID:          fmt.Sprintf("ch-%03d", i),
					ProjectID:   "p1",
					ChannelType: "slack",
					Enabled:     false,
				}
			}
			store := &mockNotificationStore{channels: channels}
			handler := NewNotificationTriggerHandler(store, nil)
			msg := cdcUpdateMsg("completed", "p1", "run-1", "job-1")

			b.ReportAllocs()
			for b.Loop() {
				if err := handler.Handle(context.Background(), msg); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
