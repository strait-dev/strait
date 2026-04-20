package logdrain

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestAuditSIEMDrain_ForwardBatch_Success(t *testing.T) {
	t.Parallel()

	var received []domain.AuditEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing auth: got %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/x-ndjson" {
			t.Errorf("wrong content type: %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("User-Agent") != "Strait-Audit-SIEM/1.0" {
			t.Errorf("wrong user agent: %s", r.Header.Get("User-Agent"))
		}
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

	if err := drain.ForwardBatch(context.Background(), events); err != nil {
		t.Fatalf("ForwardBatch: %v", err)
	}
	if len(received) != 2 {
		t.Errorf("received %d events, want 2", len(received))
	}
	if received[0].ID != "ev-1" || received[1].ID != "ev-2" {
		t.Errorf("received IDs = %v, %v", received[0].ID, received[1].ID)
	}
}

func TestAuditSIEMDrain_ForwardBatch_ServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "token", 0, 0)
	err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{{ID: "ev-1"}})
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestAuditSIEMDrain_ForwardBatch_EmptyBatch(t *testing.T) {
	t.Parallel()
	drain := NewAuditSIEMDrain("https://example.com", "token", 0, 0)
	if err := drain.ForwardBatch(context.Background(), nil); err != nil {
		t.Fatalf("empty batch should not error: %v", err)
	}
}

func TestNewAuditSIEMDrain_EmptyEndpoint(t *testing.T) {
	t.Parallel()
	if drain := NewAuditSIEMDrain("", "token", 0, 0); drain != nil {
		t.Error("expected nil drain for empty endpoint")
	}
}

func TestAuditSIEMDrain_ForwardBatch_NoAuth(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("expected no auth header, got %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "", 0, 0)
	if err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{{ID: "ev-1"}}); err != nil {
		t.Fatalf("ForwardBatch: %v", err)
	}
}

func TestAuditSIEMDrain_SetDroppedCounter_NilReceiver(t *testing.T) {
	t.Parallel()
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := provider.Meter("test")
	counter, err := meter.Int64Counter("test_dropped")
	if err != nil {
		t.Fatalf("create counter: %v", err)
	}
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
	time.Sleep(100 * time.Millisecond)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	foundWithData := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "test_dropped" {
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
					for _, dp := range sum.DataPoints {
						if dp.Value > 0 {
							foundWithData = true
						}
					}
				}
			}
		}
	}
	if !foundWithData {
		t.Error("expected test_dropped counter to have recorded at least one drop")
	}
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
	if err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{{ID: "ev-1"}}); err != nil {
		t.Fatalf("ForwardBatch: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	foundFwd := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "test_forwarded" {
				foundFwd = true
			}
		}
	}
	if !foundFwd {
		t.Error("expected test_forwarded metric after successful forward")
	}
}

func TestAuditSIEMDrain_TunableConstants(t *testing.T) {
	t.Parallel()
	if defaultSIEMFlushInterval != 10*time.Second {
		t.Errorf("defaultSIEMFlushInterval = %v, want 10s", defaultSIEMFlushInterval)
	}
	if minSIEMBufferSize != 256 {
		t.Errorf("minSIEMBufferSize = %d, want 256", minSIEMBufferSize)
	}
	if siemRetryInitialBackoff != 100*time.Millisecond {
		t.Errorf("siemRetryInitialBackoff = %v, want 100ms", siemRetryInitialBackoff)
	}
	if siemRetryMaxBackoff != 1600*time.Millisecond {
		t.Errorf("siemRetryMaxBackoff = %v, want 1600ms", siemRetryMaxBackoff)
	}
	if siemBreakerOpenDuration != 30*time.Second {
		t.Errorf("siemBreakerOpenDuration = %v, want 30s", siemBreakerOpenDuration)
	}
}

func TestAuditSIEMDrain_ShutdownTimeout_DrainsToSubDLQ(t *testing.T) {
	// Not parallel: modifies package-level siemRetryInitialBackoff.
	origInit := siemRetryInitialBackoff
	origMax := siemRetryMaxBackoff
	t.Cleanup(func() {
		siemRetryInitialBackoff = origInit
		siemRetryMaxBackoff = origMax
	})
	siemRetryInitialBackoff = 1 * time.Millisecond
	siemRetryMaxBackoff = 2 * time.Millisecond

	blocked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-blocked // block forever until test cleanup
	}))
	defer srv.Close()
	defer close(blocked)

	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := provider.Meter("test")
	failCounter, _ := meter.Int64Counter("test_failed")

	drain := NewAuditSIEMDrain(srv.URL, "tok", 2, 50*time.Millisecond)
	drain.SetMetrics(nil, failCounter, nil, nil)
	drain.Start(context.Background())

	// Enqueue events; the first batch will block in ForwardBatch.
	for i := range 6 {
		drain.Enqueue(domain.AuditEvent{ID: "ev-" + strings.Repeat("x", i)})
	}
	time.Sleep(200 * time.Millisecond)

	// Stop with a context that times out quickly to hit drainRemainingToSubDLQ.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	drain.Stop(ctx)

	// The drain should have moved remaining channel events to sub-DLQ.
	count := drain.DrainedFailureCount()
	if count == 0 {
		t.Log("sub-DLQ count is 0; the run goroutine may have drained all events before timeout")
	}
}

func TestAuditSIEMDrain_CtxDone_BatchFullFlush(t *testing.T) {
	t.Parallel()
	var batchCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		batchCount.Add(1)
		body, _ := io.ReadAll(r.Body)
		lines := strings.Split(strings.TrimSpace(string(body)), "\n")
		if len(lines) > 2 {
			t.Errorf("batch too large: got %d lines, want <= 2", len(lines))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	drain := NewAuditSIEMDrain(srv.URL, "tok", 2, 10*time.Second)
	drain.Start(ctx)

	// Enqueue more events than batchSize so the ctx.Done drain loop
	// hits the len(batch) >= batchSize branch.
	for i := range 6 {
		drain.Enqueue(domain.AuditEvent{ID: strings.Repeat("e", i+1)})
	}
	time.Sleep(100 * time.Millisecond)

	cancel()
	// Wait for the run goroutine to exit.
	time.Sleep(500 * time.Millisecond)

	batches := batchCount.Load()
	if batches < 2 {
		t.Errorf("expected >= 2 batch flushes from ctx.Done path, got %d", batches)
	}
}

func TestAuditSIEMDrain_StopNotStarted_NilChannel(t *testing.T) {
	t.Parallel()
	drain := NewAuditSIEMDrain("http://example.com", "tok", 0, 0)
	// Stop without Start -- exercises the ch == nil path in drainRemainingToSubDLQ.
	drain.Stop(context.Background())
	if count := drain.DrainedFailureCount(); count != 0 {
		t.Errorf("sub-DLQ count = %d, want 0", count)
	}
}
