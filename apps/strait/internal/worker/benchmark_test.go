package worker

import (
	"context"
	"testing"
	"time"
)

func BenchmarkCircuitBreakerAllow(b *testing.B) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, OpenDuration: time.Second})

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cb.Allow()
		}
	})
}

func BenchmarkCircuitBreakerRecordSuccess(b *testing.B) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3, OpenDuration: time.Second})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cb.RecordSuccess()
	}
}

func BenchmarkCircuitBreakerRecordFailure(b *testing.B) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: int(^uint(0) >> 1), OpenDuration: time.Second})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cb.RecordFailure()
	}
}

func BenchmarkPoolSubmit(b *testing.B) {
	p := NewPool(4)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p.Submit(ctx, func() {})
	}

	b.StopTimer()
	_ = p.Shutdown(context.Background())
}

func BenchmarkValidateEndpointURL(b *testing.B) {
	endpoint := "https://example.com/webhook"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := validateEndpointURL(endpoint); err != nil {
			b.Fatalf("validateEndpointURL() error = %v", err)
		}
	}
}
