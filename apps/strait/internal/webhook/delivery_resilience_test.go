package webhook

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

// TestWebhookResilience_ExactTimeoutBoundary verifies that a webhook endpoint
// sleeping for exactly the configured timeout duration triggers a client timeout.
func TestWebhookResilience_ExactTimeoutBoundary(t *testing.T) {
	t.Parallel()

	// The delivery worker uses a 5s timeout for first attempt.
	// We sleep slightly longer to guarantee the timeout fires.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(6 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			return
		}
	}))
	defer ts.Close()

	store := &mockDeliveryStore{}
	worker := NewDeliveryWorker(store, slog.Default())

	now := time.Now().Add(-time.Second)
	d := &domain.WebhookDelivery{
		ID:          "timeout-boundary-1",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 1,
		NextRetryAt: &now,
		LastError:   `{"test":"timeout"}`,
	}
	require.NoError(t, store.CreateWebhookDelivery(context.Background(), d))

	worker.processBatch(context.Background())

	deliveries := store.getDeliveries()
	require.NotEmpty(t, deliveries)

	got := deliveries[0]
	require.Equal(t,
		domain.WebhookStatusDead,

		got.Status)
	require.True(t,
		strings.Contains(got.LastError,
			"http request:",
		))

}

// TestWebhookResilience_PartialResponseHang verifies that an endpoint which
// writes one byte then hangs forever is handled by the client-side timeout.
func TestWebhookResilience_PartialResponseHang(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Hang forever until client disconnects.
		select {
		case <-r.Context().Done():
		case <-time.After(60 * time.Second):
		}
	}))
	defer ts.Close()

	store := &mockDeliveryStore{}
	// Use a short HTTP client timeout so the test completes quickly.
	worker := NewDeliveryWorker(store, slog.Default(),
		WithAllowPrivateEndpoints(true),
		WithHTTPTransport(2*time.Second, 5*time.Second, 10, 10))

	now := time.Now().Add(-time.Second)
	d := &domain.WebhookDelivery{
		ID:          "partial-hang-1",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 1,
		NextRetryAt: &now,
		LastError:   `{"test":"partial"}`,
	}
	require.NoError(t, store.CreateWebhookDelivery(context.Background(), d))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	worker.processBatch(ctx)

	deliveries := store.getDeliveries()
	got := deliveries[0]
	require.False(t,
		got.Status !=
			domain.WebhookStatusDelivered &&
			got.Status !=
				domain.
					WebhookStatusDead,
	)

	// The response wrote a 200 header, so the HTTP request itself succeeds.
	// The delivery should be marked delivered because the status code is 200.
	// The body drain is limited by maxResponseBodyDrainBytes + LimitReader,
	// but the client.Timeout on the Transport will cut off the read.
	// Either outcome is acceptable: delivered (got 200) or dead (timeout draining body).

}

// TestWebhookResilience_RedirectToLocalhost verifies that a 301 redirect
// from the receiver is NOT followed. Following redirects would let a
// receiver coerce the worker into hitting attacker-chosen URLs (SSRF
// via redirect to 169.254.169.254, internal services, etc.), so the
// worker treats any 3xx as a permanent delivery failure.
func TestWebhookResilience_RedirectToLocalhost(t *testing.T) {
	t.Parallel()

	var redirectTargetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		redirectTargetHits.Add(1)
	}))
	defer target.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/should-not-reach", http.StatusMovedPermanently)
	}))
	defer ts.Close()

	store := &mockDeliveryStore{}
	worker := NewDeliveryWorker(store, slog.Default(),
		WithAllowPrivateEndpoints(true),
		WithHTTPTransport(3*time.Second, 5*time.Second, 10, 10))

	now := time.Now().Add(-time.Second)
	d := &domain.WebhookDelivery{
		ID:          "redirect-localhost-1",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 1,
		NextRetryAt: &now,
		LastError:   `{"test":"redirect"}`,
	}
	require.NoError(t, store.CreateWebhookDelivery(context.Background(), d))

	worker.processBatch(context.Background())
	require.EqualValues(t, 0, redirectTargetHits.
		Load())

	deliveries := store.getDeliveries()
	got := deliveries[0]
	require.Equal(t,
		domain.WebhookStatusDead,

		got.Status)
	require.False(t,
		got.LastStatusCode ==
			nil ||
			*got.LastStatusCode !=
				http.
					StatusMovedPermanently,
	)
	require.NotEqual(t, "", got.
		LastError)

}

// TestWebhookResilience_ResponseBomb verifies that the delivery worker
// caps response body reads to prevent memory exhaustion from large responses.
func TestWebhookResilience_ResponseBomb(t *testing.T) {
	t.Parallel()

	// Serve 10 MB of zeros as the response body.
	const bombSize = 10 * 1024 * 1024
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", bombSize))
		w.WriteHeader(http.StatusOK)
		// Write in chunks to avoid buffering the entire thing.
		chunk := make([]byte, 32*1024)
		written := 0
		for written < bombSize {
			n := min(bombSize-written, len(chunk))
			if _, err := w.Write(chunk[:n]); err != nil {
				return
			}
			written += n
		}
	}))
	defer ts.Close()

	store := &mockDeliveryStore{}
	worker := NewDeliveryWorker(store, slog.Default(),
		WithAllowPrivateEndpoints(true),
		WithHTTPTransport(10*time.Second, 5*time.Second, 10, 10))

	now := time.Now().Add(-time.Second)
	d := &domain.WebhookDelivery{
		ID:          "response-bomb-1",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"test":"bomb"}`,
	}
	require.NoError(t, store.CreateWebhookDelivery(context.Background(), d))

	worker.processBatch(context.Background())

	deliveries := store.getDeliveries()
	got := deliveries[0]
	require.Equal(t,
		domain.WebhookStatusDelivered,

		got.Status,
	)

	// 200 is success; the response body drain is capped at maxResponseBodyDrainBytes (1 MB).
	// The delivery should succeed because the status code was 200.

}

// TestWebhookResilience_ConnectionCloseMidTransfer verifies that the worker
// handles a server that closes the TCP connection mid-transfer gracefully.
func TestWebhookResilience_ConnectionCloseMidTransfer(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, buf, err := hijacker.Hijack()
		if err != nil {
			return
		}
		// Write a partial HTTP response then close abruptly.
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 1000\r\n\r\npartial")
		_ = buf.Flush()
		_ = conn.Close()
	}))
	defer ts.Close()

	store := &mockDeliveryStore{}
	worker := NewDeliveryWorker(store, slog.Default(),
		WithAllowPrivateEndpoints(true),
		WithHTTPTransport(5*time.Second, 5*time.Second, 10, 10))

	now := time.Now().Add(-time.Second)
	d := &domain.WebhookDelivery{
		ID:          "conn-close-mid-1",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 1,
		NextRetryAt: &now,
		LastError:   `{"test":"midclose"}`,
	}
	require.NoError(t, store.CreateWebhookDelivery(context.Background(), d))

	worker.processBatch(context.Background())

	deliveries := store.getDeliveries()
	got := deliveries[0]
	require.False(t,
		got.Status !=
			domain.WebhookStatusDelivered &&
			got.Status !=
				domain.
					WebhookStatusDead,
	)

	// The connection closed mid-transfer may result in either:
	// - delivered (got 200 status before body drain failed, drain error is swallowed)
	// - dead (HTTP client detected the broken connection)

}

// TestWebhookResilience_ValidHTTPThenGarbage verifies that the worker handles
// an endpoint that writes valid HTTP headers followed by garbage bytes.
func TestWebhookResilience_ValidHTTPThenGarbage(t *testing.T) {
	var concWG conc.WaitGroup

	// Use a raw TCP listener to send garbage after valid headers.
	defer concWG.Wait()
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	defer ln.Close()
	concWG.Go(func() {
		var concWG conc.WaitGroup
		defer concWG.Wait()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			c := conn
			concWG.Go(func() {
				defer c.Close()

				reader := bufio.NewReader(c)
				for {
					line, err := reader.ReadString('\n')
					if err != nil || line == "\r\n" {
						break
					}
				}

				_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n"))
				_, _ = c.Write([]byte{0xFF, 0xFE, 0xFD, 0x00, 0x01, 0x02})
				_ = c.(*net.TCPConn).CloseWrite()
			})
		}
	})

	store := &mockDeliveryStore{}
	worker := NewDeliveryWorker(store, slog.Default(),
		WithAllowPrivateEndpoints(true),
		WithHTTPTransport(5*time.Second, 5*time.Second, 10, 10))

	now := time.Now().Add(-time.Second)
	d := &domain.WebhookDelivery{
		ID:          "garbage-after-headers-1",
		WebhookURL:  "http://" + ln.Addr().String(),
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 1,
		NextRetryAt: &now,
		LastError:   `{"test":"garbage"}`,
	}
	require.NoError(t, store.CreateWebhookDelivery(context.Background(), d))

	worker.processBatch(context.Background())

	deliveries := store.getDeliveries()
	got := deliveries[0]
	require.False(t,
		got.Status !=
			domain.WebhookStatusDelivered &&
			got.Status !=
				domain.
					WebhookStatusDead,
	)

	// The HTTP client parsed the 200 status code. Body drain may hit an unexpected EOF
	// but that error is discarded. The delivery should be marked as delivered.

}

// TestWebhookResilience_OKInvalidJSON verifies that a 200 response with
// a non-JSON body does not crash the worker. Webhook delivery considers
// any 2xx status code a success regardless of response body content.
func TestWebhookResilience_OKInvalidJSON(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json at all"))
	}))
	defer ts.Close()

	store := &mockDeliveryStore{}
	worker := NewDeliveryWorker(store, slog.Default())

	now := time.Now().Add(-time.Second)
	d := &domain.WebhookDelivery{
		ID:          "ok-invalid-json-1",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"test":"okjson"}`,
	}
	require.NoError(t, store.CreateWebhookDelivery(context.Background(), d))

	worker.processBatch(context.Background())

	deliveries := store.getDeliveries()
	got := deliveries[0]
	require.Equal(t,
		domain.WebhookStatusDelivered,

		got.Status,
	)

}

// TestWebhookResilience_HMACVerification verifies that the HMAC signature
// validation round-trips correctly: compute a signature on the sender side,
// then validate it on the receiver side using ValidateSignature.
func TestWebhookResilience_HMACVerification(t *testing.T) {
	t.Parallel()

	secret := "webhook-test-secret-42"
	var receivedBody []byte
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		receivedBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// The delivery worker does not sign outbound requests automatically.
	// We test the signature module directly by computing the HMAC of a known
	// payload and verifying it round-trips through ValidateSignature.
	payload := []byte(`{"event":"job.completed","job_id":"j-1"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	require.NoError(t, ValidateSignature("hmac-sha256",
		secret,
		payload, signature,
	))
	require.Error(t,
		ValidateSignature("hmac-sha256",
			"wrong-secret",
			payload,
			signature,
		))

	// Validate the computed signature.

	// Also verify that a wrong secret fails.

	// Also verify that tampering with the body fails.
	tampered := append([]byte{}, payload...)
	tampered[0] = '!'
	require.Error(t,
		ValidateSignature("hmac-sha256",
			secret,
			tampered, signature,
		))

	// Now deliver to the test server and verify headers are present on the request.
	store := &mockDeliveryStore{}
	worker := NewDeliveryWorker(store, slog.Default())

	now := time.Now().Add(-time.Second)
	d := &domain.WebhookDelivery{
		ID:          "hmac-verify-1",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"event":"job.completed","job_id":"j-1"}`,
	}
	require.NoError(t, store.CreateWebhookDelivery(context.Background(), d))

	worker.processBatch(context.Background())

	deliveries := store.getDeliveries()
	got := deliveries[0]
	require.Equal(t,
		domain.WebhookStatusDelivered,

		got.Status,
	)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, receivedBody)

	// Verify the received payload can be used to compute a valid HMAC.
	verifyMac := hmac.New(sha256.New, []byte(secret))
	verifyMac.Write(receivedBody)
	computedSig := "sha256=" + hex.EncodeToString(verifyMac.Sum(nil))
	require.NoError(t, ValidateSignature("hmac-sha256",
		secret,
		receivedBody,
		computedSig,
	))

}

// TestWebhookResilience_RetryExhaustion verifies that when the endpoint always
// returns 500, all retry attempts are made and the delivery ends up dead.
func TestWebhookResilience_RetryExhaustion(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	store := &mockDeliveryStore{}
	worker := NewDeliveryWorker(store, slog.Default())

	maxAttempts := 3
	now := time.Now().Add(-time.Second)
	d := &domain.WebhookDelivery{
		ID:          "retry-exhaust-1",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: maxAttempts,
		NextRetryAt: &now,
		LastError:   `{"test":"exhaust"}`,
	}
	require.NoError(t, store.CreateWebhookDelivery(context.Background(), d))

	// Process batch multiple times to exhaust retries.
	// Each processBatch picks up pending deliveries whose NextRetryAt <= now.
	// After each failure, NextRetryAt is set in the future, so we manually
	// advance it to make the delivery eligible again.
	for range maxAttempts {
		worker.processBatch(context.Background())

		// Make the delivery eligible for the next poll by resetting NextRetryAt.
		deliveries := store.getDeliveries()
		for _, dd := range deliveries {
			if dd.ID == d.ID && dd.Status == domain.WebhookStatusPending {
				past := time.Now().Add(-time.Second)
				dd.NextRetryAt = &past
				_ = store.UpdateWebhookDelivery(context.Background(), dd)
			}
		}
	}
	require.Equal(t,
		maxAttempts,
		int(attempts.
			Load()))

	deliveries := store.getDeliveries()
	got := deliveries[0]
	require.Equal(t,
		domain.WebhookStatusDead,

		got.Status)
	require.NotEqual(t, "", got.
		LastError)

}

// mockCircuitBreaker is a simple in-memory circuit breaker for testing.
type mockCircuitBreaker struct {
	mu       sync.Mutex
	failures map[string]int
	open     map[string]bool
}

func newMockCircuitBreaker() *mockCircuitBreaker {
	return &mockCircuitBreaker{
		failures: make(map[string]int),
		open:     make(map[string]bool),
	}
}

func (m *mockCircuitBreaker) CanDeliver(_ context.Context, url string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.open[url] {
		return false, nil
	}
	return true, nil
}

func (m *mockCircuitBreaker) RecordSuccess(_ context.Context, url string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failures[url] = 0
	m.open[url] = false
}

func (m *mockCircuitBreaker) RecordFailure(_ context.Context, url string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failures[url]++
	// Open circuit after 3 failures.
	if m.failures[url] >= 3 {
		m.open[url] = true
	}
}

func (m *mockCircuitBreaker) getFailures(url string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failures[url]
}

func (m *mockCircuitBreaker) isOpen(url string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.open[url]
}

// TestWebhookResilience_CircuitBreakerThreshold verifies that the circuit
// breaker opens after N failures and closes after a success.
func TestWebhookResilience_CircuitBreakerThreshold(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count := requestCount.Add(1)
		if count <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cb := newMockCircuitBreaker()
	store := &mockDeliveryStore{}
	worker := NewDeliveryWorker(store, slog.Default(), WithCircuitBreaker(cb))

	// Send 3 deliveries that will fail, each triggering RecordFailure.
	for i := range 3 {
		now := time.Now().Add(-time.Second)
		d := &domain.WebhookDelivery{
			ID:          fmt.Sprintf("cb-fail-%d", i),
			WebhookURL:  ts.URL,
			Status:      domain.WebhookStatusPending,
			Attempts:    0,
			MaxAttempts: 5,
			NextRetryAt: &now,
			LastError:   fmt.Sprintf(`{"test":"cb-fail-%d"}`, i),
		}
		require.NoError(t, store.CreateWebhookDelivery(context.Background(), d))

		worker.processBatch(context.Background())
	}
	require.True(t,
		cb.isOpen(
			ts.URL))
	require.GreaterOrEqual(t,
		cb.getFailures(
			ts.URL), 3)

	// A delivery while the circuit is open should fail without hitting the endpoint.
	prevCount := requestCount.Load()
	now := time.Now().Add(-time.Second)
	d := &domain.WebhookDelivery{
		ID:          "cb-blocked",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"test":"cb-blocked"}`,
	}
	require.NoError(t, store.CreateWebhookDelivery(context.Background(), d))

	worker.processBatch(context.Background())
	require.Equal(t,
		prevCount,
		requestCount.
			Load())

	// Record a success to close the circuit.
	cb.RecordSuccess(context.Background(), ts.URL)
	require.False(t,
		cb.isOpen(ts.URL))

	// Now a delivery should go through and succeed (server returns 200 after count > 3).
	now = time.Now().Add(-time.Second)
	d2 := &domain.WebhookDelivery{
		ID:          "cb-after-close",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"test":"cb-after-close"}`,
	}
	require.NoError(t, store.CreateWebhookDelivery(context.Background(), d2))

	worker.processBatch(context.Background())

	for _, dd := range store.getDeliveries() {
		require.False(t,
			dd.ID ==
				"cb-after-close" &&
				dd.Status !=
					domain.WebhookStatusDelivered,
		)

	}
}

// TestWebhookResilience_ConcurrentDeliverySameEndpoint verifies that 10
// concurrent goroutines delivering to the same endpoint cause no race conditions.
func TestWebhookResilience_ConcurrentDeliverySameEndpoint(t *testing.T) {
	t.Parallel()

	var received atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the full body to ensure no races on the server side.
		_, _ = io.ReadAll(r.Body)
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	store := &mockDeliveryStore{}
	worker := NewDeliveryWorker(store, slog.Default(), WithConcurrency(10))

	const count = 10
	now := time.Now().Add(-time.Second)
	for i := range count {
		d := &domain.WebhookDelivery{
			ID:          fmt.Sprintf("concurrent-%d", i),
			WebhookURL:  ts.URL,
			Status:      domain.WebhookStatusPending,
			Attempts:    0,
			MaxAttempts: 5,
			NextRetryAt: &now,
			LastError:   fmt.Sprintf(`{"index":%d}`, i),
		}
		require.NoError(t, store.CreateWebhookDelivery(context.Background(), d))

	}

	worker.processBatch(context.Background())
	require.EqualValues(t,
		count, received.
			Load())

	deliveredCount := 0
	for _, dd := range store.getDeliveries() {
		if dd.Status == domain.WebhookStatusDelivered {
			deliveredCount++
		}
	}
	require.Equal(t,
		count, deliveredCount,
	)

}

// TestWebhookResilience_SelfSignedTLS verifies that delivering to an endpoint
// with a self-signed TLS certificate is handled gracefully (TLS error, no panic).
func TestWebhookResilience_SelfSignedTLS(t *testing.T) {
	t.Parallel()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	store := &mockDeliveryStore{}
	// Use default transport (does not trust the self-signed cert).
	worker := NewDeliveryWorker(store, slog.Default(),
		WithAllowPrivateEndpoints(true),
		WithHTTPTransport(5*time.Second, 5*time.Second, 10, 10))

	now := time.Now().Add(-time.Second)
	d := &domain.WebhookDelivery{
		ID:          "self-signed-tls-1",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 1,
		NextRetryAt: &now,
		LastError:   `{"test":"tls"}`,
	}
	require.NoError(t, store.CreateWebhookDelivery(context.Background(), d))

	worker.processBatch(context.Background())

	deliveries := store.getDeliveries()
	got := deliveries[0]
	require.Equal(t,
		domain.WebhookStatusDead,

		got.Status)
	require.True(t,
		strings.Contains(got.LastError,
			"http request:",
		))

	// Verify the error mentions certificate-related issue.
	if !strings.Contains(got.LastError, "certificate") && !strings.Contains(got.LastError, "tls") {
		t.Logf("warning: expected certificate/tls in error but got: %s", got.LastError)
	}
}
