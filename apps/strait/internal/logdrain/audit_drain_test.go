package logdrain

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestAuditSIEMDrain_ForwardBatch_Success(t *testing.T) {
	t.Parallel()

	var received []domain.AuditEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token",

			r.Header.
				Get("Authorization"))
		assert.Equal(t, "application/x-ndjson",

			r.
				Header.Get(
				"Content-Type"))
		assert.Equal(t, "Strait-Audit-SIEM/1.0",

			r.Header.Get("User-Agent"))

		body, _ := io.ReadAll(r.Body)
		for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
			var ev domain.AuditEvent
			if err := json.Unmarshal([]byte(line), &ev); err == nil {
				received = append(received, ev)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "test-token", 0, 0)
	events := []domain.AuditEvent{
		{ID: "ev-1", Action: "job.created", ProjectID: "p1"},
		{ID: "ev-2", Action: "job.deleted", ProjectID: "p1"},
	}
	require.NoError(t,
		drain.ForwardBatch(context.
			Background(), events))
	assert.Len(t, received,
		2)
	assert.False(t, received[0].ID != "ev-1" ||
		received[1].ID != "ev-2")

}

func TestAuditSIEMDrain_ForwardBatch_ServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "token", 0, 0)
	err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{{ID: "ev-1"}})
	require.Error(t, err)

}

func TestAuditSIEMDrain_ForwardBatch_EmptyBatch(t *testing.T) {
	t.Parallel()
	drain := NewAuditSIEMDrain("https://example.com", "token", 0, 0)
	require.NoError(t,
		drain.ForwardBatch(context.
			Background(), nil))

}

func TestNewAuditSIEMDrain_EmptyEndpoint(t *testing.T) {
	t.Parallel()
	require.Nil(t, NewAuditSIEMDrain("", "token", 0, 0))
}

func TestAuditSIEMDrain_ForwardBatch_NoAuth(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "",
			r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "", 0, 0)
	require.NoError(t,
		drain.ForwardBatch(context.
			Background(), []domain.AuditEvent{{ID: "ev-1"}}))

}

func TestAuditSIEMDrain_SetDroppedCounter_NilReceiver(t *testing.T) {
	t.Parallel()
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := provider.Meter("test")
	counter, err := meter.Int64Counter("test_dropped")
	require.NoError(t,
		err)

	// Nil receiver must not panic.
	(*AuditSIEMDrain)(nil).SetDroppedCounter(counter)

	// Use a blocking server so the channel fills up and triggers drops.
	blocked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-blocked
	}))
	defer srv.Close()
	defer close(blocked)

	// batchSize=1 with tiny flush interval means minimal buffer (max(1*4, 256) = 256).
	drain := NewAuditSIEMDrain(srv.URL, "tok", 1, 10*time.Millisecond)
	drain.SetDroppedCounter(counter)
	drain.Start(context.Background())
	defer drain.Stop(context.Background())

	// Flood the buffer well past its capacity.
	for range 600 {
		drain.Enqueue(domain.AuditEvent{ID: "drop"})
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var rm metricdata.ResourceMetrics
		require.NoError(t,
			reader.Collect(context.
				Background(), &rm))

		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				if m.Name == "test_dropped" {
					if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
						for _, dp := range sum.DataPoints {
							if dp.Value > 0 {
								return
							}
						}
					}
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.Fail(t, "expected test_dropped counter to have recorded at least one drop")
}

func TestAuditSIEMDrain_SetMetrics_NilReceiver(t *testing.T) {
	t.Parallel()
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := provider.Meter("test")
	fwd, _ := meter.Int64Counter("test_forwarded")
	fail, _ := meter.Int64Counter("test_failed")
	co, _ := meter.Int64Counter("test_circuit_open")
	bh, _ := meter.Int64Histogram("test_batch_size")
	(*AuditSIEMDrain)(nil).SetMetrics(fwd, fail, co, bh)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "tok", 0, 0)
	drain.SetMetrics(fwd, fail, co, bh)
	require.NoError(t,
		drain.ForwardBatch(context.
			Background(), []domain.AuditEvent{{ID: "ev-1"}}))

	var rm metricdata.ResourceMetrics
	require.NoError(t,
		reader.Collect(context.
			Background(), &rm))

	foundFwd := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "test_forwarded" {
				foundFwd = true
			}
		}
	}
	assert.True(t, foundFwd)

}

func TestAuditSIEMDrain_TunableConstants(t *testing.T) {
	assert.Equal(t, 100,
		defaultSIEMBatchSize,
	)
	assert.Equal(t, 10*
		time.Second, defaultSIEMFlushInterval,
	)
	assert.Equal(t, 256,
		minSIEMBufferSize,
	)
	assert.Equal(t, 5*
		time.Second, siemShutdownTimeout,
	)
	assert.Equal(t, 3,
		siemMaxRetryAttempts,
	)
	assert.Equal(t, 5,
		siemBreakerFailureThreshold,
	)
	assert.Equal(t, 1,
		siemBreakerHalfOpenSuccesses,
	)
	assert.Equal(t, 1024,
		siemSubDLQCapacity,
	)
	assert.Equal(t, 100*
		time.Millisecond,
		siemRetryInitialBackoff,
	)
	assert.Equal(t, 1600*
		time.Millisecond,

		siemRetryMaxBackoff,
	)
	assert.Equal(t, 4.0,
		siemRetryBackoffFactor,
	)
	assert.Equal(t, 30*
		time.Second, siemBreakerOpenDuration,
	)

}

func TestAuditSIEMDrain_StopNotStarted_NilChannel(t *testing.T) {
	t.Parallel()
	drain := NewAuditSIEMDrain("http://example.com", "tok", 0, 0)
	// Stop without Start -- exercises the ch == nil path in drainRemainingToSubDLQ.
	drain.Stop(context.Background())
	assert.Equal(t, 0, drain.DrainedFailureCount())
}
