package webhook

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
)

func BenchmarkDeliveryWorker_Throughput(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	const total = 500
	ms := &mockDeliveryStore{
		listPendingFn: func(_ context.Context) ([]domain.WebhookDelivery, error) {
			deliveries := make([]domain.WebhookDelivery, 0, total)
			now := time.Now().Add(-time.Second)
			for i := range total {
				deliveries = append(deliveries, domain.WebhookDelivery{
					ID:          fmt.Sprintf("bench-%d", i),
					WebhookURL:  ts.URL,
					Status:      domain.WebhookStatusPending,
					MaxAttempts: 1,
					NextRetryAt: &now,
					LastError:   fmt.Sprintf(`{"i":%d}`, i),
				})
			}
			return deliveries, nil
		},
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithConcurrency(50))

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		worker.processBatch(context.Background())
	}

	elapsed := b.Elapsed()
	totalDeliveries := float64(b.N) * total
	deliveriesPerSec := totalDeliveries / elapsed.Seconds()
	b.ReportMetric(deliveriesPerSec, "deliveries/sec")
}

func BenchmarkDeliveryWorker_BatchThroughput(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	const total = 500
	ms := &mockDeliveryStore{
		listPendingFn: func(_ context.Context) ([]domain.WebhookDelivery, error) {
			deliveries := make([]domain.WebhookDelivery, 0, total)
			now := time.Now().Add(-time.Second)
			for i := range total {
				deliveries = append(deliveries, domain.WebhookDelivery{
					ID:          fmt.Sprintf("bench-batch-%d", i),
					WebhookURL:  ts.URL, // all same URL
					Status:      domain.WebhookStatusPending,
					MaxAttempts: 1,
					NextRetryAt: &now,
					LastError:   fmt.Sprintf(`{"i":%d}`, i),
				})
			}
			return deliveries, nil
		},
	}

	worker := NewDeliveryWorker(ms, slog.Default(),
		WithConcurrency(50),
		WithBatchByURL(true),
		WithMaxBatchSize(50),
	)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		worker.processBatch(context.Background())
	}

	elapsed := b.Elapsed()
	totalDeliveries := float64(b.N) * total
	deliveriesPerSec := totalDeliveries / elapsed.Seconds()
	b.ReportMetric(deliveriesPerSec, "deliveries/sec")
}

func BenchmarkDeliveryWorker_TransportReuse(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	const total = 100
	ms := &mockDeliveryStore{
		listPendingFn: func(_ context.Context) ([]domain.WebhookDelivery, error) {
			deliveries := make([]domain.WebhookDelivery, 0, total)
			now := time.Now().Add(-time.Second)
			for i := range total {
				deliveries = append(deliveries, domain.WebhookDelivery{
					ID:          fmt.Sprintf("bench-tr-%d", i),
					WebhookURL:  ts.URL,
					Status:      domain.WebhookStatusPending,
					MaxAttempts: 1,
					NextRetryAt: &now,
					LastError:   fmt.Sprintf(`{"i":%d}`, i),
				})
			}
			return deliveries, nil
		},
	}

	worker := NewDeliveryWorker(ms, slog.Default(),
		WithConcurrency(25),
		WithHTTPTransport(30*time.Second, time.Minute, 50, 25),
	)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		worker.processBatch(context.Background())
	}

	elapsed := b.Elapsed()
	totalDeliveries := float64(b.N) * total
	deliveriesPerSec := totalDeliveries / elapsed.Seconds()
	b.ReportMetric(deliveriesPerSec, "deliveries/sec")
}
