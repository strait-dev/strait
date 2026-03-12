package webhook

import (
	"testing"
	"time"
)

func BenchmarkRedisWebhookCircuitBreaker_RecordFailureThenCanDeliver(b *testing.B) {
	state := newRedisCircuitState()
	client := newMockRedisClient(state.process)
	b.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(
		client,
		true,
		WithWebhookCircuitBreakerThreshold(10_000_000),
		WithWebhookCircuitBreakerWindow(time.Minute),
	)

	ctx := b.Context()
	url := "https://example.com/webhook"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		breaker.RecordFailure(ctx, url)
		if _, err := breaker.CanDeliver(ctx, url); err != nil {
			b.Fatalf("CanDeliver() error = %v", err)
		}
	}
}
