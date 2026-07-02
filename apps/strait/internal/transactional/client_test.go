package transactional

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestClientSendPostsExpectedJSON(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/internal/transactional-email", r.URL.Path)
		assert.Equal(t, "internal-secret", r.Header.Get("X-Internal-Secret"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		_, _ = w.Write([]byte(`{"id":"email-123"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "internal-secret", time.Second, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	require.NotNil(t, client)

	req := BillingPaymentFailedRequest([]string{"admin@example.com"}, "billing@strait.dev", "Pro", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	require.NoError(t, client.Send(context.Background(), req))

	assert.Equal(t, "billing.payment_failed", got["template"])
	assert.Equal(t, "billing@strait.dev", got["from"])
	assert.Equal(t, "billing:payment_failed:admin@example.com:Pro:2026-04-15", got["idempotencyKey"])
	assert.Equal(t, []any{"admin@example.com"}, got["to"])
	props, ok := got["props"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Pro", props["planName"])
	assert.Equal(t, "April 15, 2026", props["gracePeriodEnd"])
}

func TestClientSendHandlesNon2xxResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad template", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.URL, "internal-secret", time.Second, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	require.NotNil(t, client)

	req := BillingDisputeAlertRequest([]string{"admin@example.com"}, "billing@strait.dev", "$25.00")
	err := client.Send(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transactional email endpoint returned 400")
}

func TestClientSendHandlesTransportError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"email-123"}`))
	}))
	client := NewClient(server.URL, "internal-secret", time.Second, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	require.NotNil(t, client)
	server.Close()

	req := BillingDisputeAlertRequest([]string{"admin@example.com"}, "billing@strait.dev", "$25.00")
	require.Error(t, client.Send(context.Background(), req))
}

func TestNewClientRejectsMissingOrInvalidConfig(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	assert.Nil(t, NewClient("", "internal-secret", time.Second, logger))
	assert.Nil(t, NewClient("http://localhost:5173", "", time.Second, logger))
	assert.Nil(t, NewClient("localhost:5173", "internal-secret", time.Second, logger))
}

func TestClientSendDoesNotLogSensitiveFields(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "provider unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	client := NewClient(server.URL, "internal-secret", time.Second, logger)
	require.NotNil(t, client)

	req := Request{
		Template:       TemplateNotificationGeneric,
		To:             []string{"admin@example.com"},
		From:           "alerts@strait.dev",
		IdempotencyKey: "notification:delivery-1:unknown.event",
		Props: map[string]any{
			"eventType":   "unknown.event",
			"payload":     `{"secret":"secret-prop"}`,
			"secretToken": "secret-prop",
		},
	}
	require.Error(t, client.Send(context.Background(), req))

	logOutput := logs.String()
	assert.Contains(t, logOutput, "template=notification.generic")
	assert.Contains(t, logOutput, "recipient_count=1")
	assert.NotContains(t, logOutput, "admin@example.com")
	assert.NotContains(t, logOutput, "notification:delivery-1:unknown.event")
	assert.NotContains(t, logOutput, "secret-prop")
}

func TestClientSendRecordsMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	transactionalMetricsOnce = sync.Once{}
	transactionalRequests = nil
	transactionalDuration = nil

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"email-123"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "internal-secret", time.Second, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	require.NotNil(t, client)
	req := BillingDisputeAlertRequest([]string{"admin@example.com"}, "billing@strait.dev", "$25.00")
	require.NoError(t, client.Send(context.Background(), req))

	var metrics metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &metrics))
	assertMetricPoint(t, metrics, transactionalEmailRequestsMetric, "billing.dispute_alert", "success", "200")
	assertMetricPoint(t, metrics, transactionalEmailDurationMetric, "billing.dispute_alert", "success", "200")
}

func assertMetricPoint(t *testing.T, metrics metricdata.ResourceMetrics, name, template, outcome, statusCode string) {
	t.Helper()
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			for _, attrs := range metricAttributes(metric) {
				if attributeValue(attrs, "template") == template &&
					attributeValue(attrs, "outcome") == outcome &&
					attributeValue(attrs, "status_code") == statusCode {
					return
				}
			}
		}
	}
	require.Failf(t, "metric point not found", "name=%s template=%s outcome=%s status_code=%s", name, template, outcome, statusCode)
}

func metricAttributes(metric metricdata.Metrics) []attribute.Set {
	switch data := metric.Data.(type) {
	case metricdata.Sum[int64]:
		attrs := make([]attribute.Set, 0, len(data.DataPoints))
		for _, point := range data.DataPoints {
			attrs = append(attrs, point.Attributes)
		}
		return attrs
	case metricdata.Histogram[float64]:
		attrs := make([]attribute.Set, 0, len(data.DataPoints))
		for _, point := range data.DataPoints {
			attrs = append(attrs, point.Attributes)
		}
		return attrs
	default:
		return nil
	}
}

func attributeValue(attrs attribute.Set, key string) string {
	iter := attrs.Iter()
	for iter.Next() {
		attr := iter.Attribute()
		if strings.EqualFold(string(attr.Key), key) {
			return attr.Value.AsString()
		}
	}
	return ""
}
