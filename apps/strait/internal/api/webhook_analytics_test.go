package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestHandleWebhookDeliveryStats_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetWebhookDeliveryStatsFunc: func(_ context.Context, _ string, _, _ time.Time) ([]store.WebhookEndpointStats, error) {
			return []store.WebhookEndpointStats{
				{URL: "https://example.com/hook", Total: 100, Delivered: 90, Failed: 10, AvgLatencyMs: 200, P95LatencyMs: 800},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("webhooks/delivery-stats", validFrom(), validTo()), "", "proj-1"))
	require.Equal(t, 200, w.
		Code)
}

func TestHandleWebhookDeliveryStats_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", "/v1/analytics/webhooks/delivery-stats", "", "proj-1"))
	require.Equal(t, 400, w.
		Code)
}

func TestHandleWebhookDeliveryStats_StoreError(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetWebhookDeliveryStatsFunc: func(_ context.Context, _ string, _, _ time.Time) ([]store.WebhookEndpointStats, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("webhooks/delivery-stats", validFrom(), validTo()), "", "proj-1"))
	require.Equal(t, 500, w.
		Code)
}

func TestHandleWebhookEndpointHealth_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetWebhookEndpointHealthFunc: func(_ context.Context, _ string, _, _ time.Time, bucket string) ([]store.WebhookHealthBucket, error) {
			require.Equal(t, "day",
				bucket)

			return []store.WebhookHealthBucket{}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("webhooks/endpoint-health", validFrom(), validTo()), "", "proj-1"))
	require.Equal(t, 200, w.
		Code)
}

func TestHandleWebhookEndpointHealth_InvalidBucket(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("webhooks/endpoint-health", validFrom(), validTo(), "bucket", "month"), "", "proj-1"))
	require.Equal(t, 400, w.
		Code)
}

func TestHandleTopFailingWebhooks_Success(t *testing.T) {
	t.Parallel()
	ms := &AnalyticsStoreMock{
		GetTopFailingWebhooksFunc: func(_ context.Context, _ string, _, _ time.Time, _ int) ([]store.TopFailingEndpoint, error) {
			return []store.TopFailingEndpoint{
				{URL: "https://example.com/bad", Failed: 20, Total: 30, FailureRate: 0.67},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, ms, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("webhooks/top-failing", validFrom(), validTo()), "", "proj-1"))
	require.Equal(t, 200, w.
		Code)
}

func TestHandleTopFailingWebhooks_InvalidLimit(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("webhooks/top-failing", validFrom(), validTo(), "limit", "999"), "", "proj-1"))
	require.Equal(t, 400, w.
		Code)
}
