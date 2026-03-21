package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/store"
)

func TestHandleWebhookDeliveryStats_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getWebhookDeliveryStatsFn: func(_ context.Context, _ string, _, _ time.Time) ([]store.WebhookEndpointStats, error) {
			return []store.WebhookEndpointStats{
				{URL: "https://example.com/hook", Total: 100, Delivered: 90, Failed: 10, AvgLatencyMs: 200, P95LatencyMs: 800},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("webhooks/delivery-stats", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWebhookDeliveryStats_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", "/v1/analytics/webhooks/delivery-stats", "", "proj-1"))
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleWebhookDeliveryStats_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getWebhookDeliveryStatsFn: func(_ context.Context, _ string, _, _ time.Time) ([]store.WebhookEndpointStats, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("webhooks/delivery-stats", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleWebhookEndpointHealth_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getWebhookEndpointHealthFn: func(_ context.Context, _ string, _, _ time.Time, bucket string) ([]store.WebhookHealthBucket, error) {
			if bucket != "day" {
				t.Fatalf("expected default bucket 'day', got %q", bucket)
			}
			return []store.WebhookHealthBucket{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("webhooks/endpoint-health", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWebhookEndpointHealth_InvalidBucket(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("webhooks/endpoint-health", validFrom(), validTo(), "bucket", "month"), "", "proj-1"))
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleTopFailingWebhooks_Success(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getTopFailingWebhooksFn: func(_ context.Context, _ string, _, _ time.Time, _ int) ([]store.TopFailingEndpoint, error) {
			return []store.TopFailingEndpoint{
				{URL: "https://example.com/bad", Failed: 20, Total: 30, FailureRate: 0.67},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("webhooks/top-failing", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTopFailingWebhooks_InvalidLimit(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("webhooks/top-failing", validFrom(), validTo(), "limit", "999"), "", "proj-1"))
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
