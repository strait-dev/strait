package notification

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"

	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/jarcoal/httpmock"
)

func newTestChannel(url, secret string) *domain.NotificationChannel {
	cfg, _ := json.Marshal(webhookConfig{URL: url, Secret: secret})
	return &domain.NotificationChannel{
		ID:          "ch-1",
		ChannelType: "webhook",
		Config:      cfg,
		Enabled:     true,
	}
}

func newTestDelivery(eventType string, payload json.RawMessage) *domain.NotificationDelivery {
	return &domain.NotificationDelivery{
		ID:        "del-1",
		EventType: eventType,
		Payload:   payload,
	}
}

func newMockClient(t *testing.T) (*http.Client, *httpmock.MockTransport) {
	t.Helper()
	transport := httpmock.NewMockTransport()
	client := &http.Client{Transport: transport}
	return client, transport
}

func TestWebhookSender_Success(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://example.com/hook",
		httpmock.NewStringResponder(200, "ok"))

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{"run_id":"r-1"}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestWebhookSender_NonOKStatus(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://example.com/hook",
		httpmock.NewStringResponder(500, "internal error"))

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.failed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
}

func TestWebhookSender_4xxStatus(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://example.com/hook",
		httpmock.NewStringResponder(400, "bad request"))

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.failed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected error for 400 status")
	}
}

func TestWebhookSender_NetworkError(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://example.com/hook",
		httpmock.NewErrorResponder(http.ErrHandlerTimeout))

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.failed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestWebhookSender_NetworkErrorRedactsSecretURL(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	rawURL := "https://user:password@example.com/hooks/token-123?api_key=secret-value"
	transport.RegisterResponder("POST", rawURL,
		httpmock.NewErrorResponder(&url.Error{
			Op:  "Post",
			URL: rawURL,
			Err: errors.New("dial tcp: connection refused"),
		}))

	sender := NewWebhookSender(client)
	ch := newTestChannel(rawURL, "")
	del := newTestDelivery("run.failed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected error for network failure")
	}
	errText := err.Error()
	for _, leaked := range []string{"password", "token-123", "secret-value", "api_key"} {
		if strings.Contains(errText, leaked) {
			t.Fatalf("error leaked URL secret %q: %s", leaked, errText)
		}
	}
	if !strings.Contains(errText, "connection refused") {
		t.Fatalf("error omitted sanitized transport reason: %s", errText)
	}
}

func TestWebhookSender_HMACSignature(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	secret := "my-webhook-secret"
	payload := json.RawMessage(`{"run_id":"r-1","status":"completed"}`)

	var capturedSig string
	var capturedStraitSig string
	var capturedTimestamp string
	var capturedDeliveryID string
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(req *http.Request) (*http.Response, error) {
			capturedSig = req.Header.Get("X-Strait-Signature-256")
			capturedStraitSig = req.Header.Get("X-Strait-Signature")
			capturedTimestamp = req.Header.Get("X-Strait-Timestamp")
			capturedDeliveryID = req.Header.Get("X-Strait-Delivery-ID")
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", secret)
	del := newTestDelivery("run.completed", payload)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if capturedTimestamp == "" {
		t.Fatal("expected X-Strait-Timestamp header")
	}
	if _, parseErr := time.Parse(time.RFC3339, capturedTimestamp); parseErr != nil {
		t.Fatalf("timestamp header is not RFC3339: %v", parseErr)
	}
	if capturedDeliveryID != del.ID {
		t.Fatalf("delivery id header = %q, want %q", capturedDeliveryID, del.ID)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(capturedTimestamp))
	mac.Write([]byte("."))
	mac.Write([]byte(del.ID))
	mac.Write([]byte("."))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if capturedSig != expected {
		t.Fatalf("signature mismatch:\n  got:  %s\n  want: %s", capturedSig, expected)
	}
	if capturedStraitSig == "" || !strings.Contains(capturedStraitSig, "t="+capturedTimestamp) || !strings.Contains(capturedStraitSig, "d="+del.ID) {
		t.Fatalf("structured Strait signature missing timestamp/delivery id: %q", capturedStraitSig)
	}
}

func TestWebhookSender_HMACSignatureChangesWithDeliveryID(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	secret := "my-webhook-secret"
	payload := json.RawMessage(`{"run_id":"r-1","status":"completed"}`)
	var signatures []string
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(req *http.Request) (*http.Response, error) {
			signatures = append(signatures, req.Header.Get("X-Strait-Signature-256"))
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", secret)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sender.Send(ctx, ch, &domain.NotificationDelivery{ID: "del-1", EventType: "run.completed", Payload: payload}); err != nil {
		t.Fatalf("first Send failed: %v", err)
	}
	if err := sender.Send(ctx, ch, &domain.NotificationDelivery{ID: "del-2", EventType: "run.completed", Payload: payload}); err != nil {
		t.Fatalf("second Send failed: %v", err)
	}

	if len(signatures) != 2 {
		t.Fatalf("captured signatures = %d, want 2", len(signatures))
	}
	if signatures[0] == signatures[1] {
		t.Fatal("signatures for different delivery ids should differ")
	}
}

func TestWebhookSender_HMACSignature_NoSecret(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var hasSigHeader bool
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(req *http.Request) (*http.Response, error) {
			hasSigHeader = req.Header.Get("X-Strait-Signature-256") != "" || req.Header.Get("X-Signature-256") != ""
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = sender.Send(ctx, ch, del)

	if hasSigHeader {
		t.Fatal("expected no signature header when secret is empty")
	}
}

func TestWebhookSender_EventTypeHeader(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var capturedEventType string
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(req *http.Request) (*http.Response, error) {
			capturedEventType = req.Header.Get("X-Event-Type")
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = sender.Send(ctx, ch, del)

	if capturedEventType != "run.completed" {
		t.Fatalf("X-Event-Type = %q, want %q", capturedEventType, "run.completed")
	}
}

func TestWebhookSender_ContentTypeJSON(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var capturedContentType string
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(req *http.Request) (*http.Response, error) {
			capturedContentType = req.Header.Get("Content-Type")
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{"key":"value"}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = sender.Send(ctx, ch, del)

	if capturedContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", capturedContentType, "application/json")
	}
}

func TestWebhookSender_RequestBody(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var capturedBody []byte
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(req *http.Request) (*http.Response, error) {
			capturedBody, _ = io.ReadAll(req.Body)
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	payload := json.RawMessage(`{"run_id":"r-1","status":"completed"}`)
	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", payload)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = sender.Send(ctx, ch, del)

	if string(capturedBody) != string(payload) {
		t.Fatalf("body mismatch:\n  got:  %s\n  want: %s", capturedBody, payload)
	}
}

func TestWebhookSender_ContextCancellation(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://example.com/hook",
		func(req *http.Request) (*http.Response, error) {
			if err := req.Context().Err(); err != nil {
				return nil, err
			}
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestWebhookSender_EmptyURL(t *testing.T) {
	t.Parallel()

	sender := NewWebhookSender(nil)
	ch := newTestChannel("", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected error for empty webhook URL")
	}
}

func TestWebhookSender_NilClient(t *testing.T) {
	t.Parallel()

	sender := NewWebhookSender(nil)
	if sender.client == nil {
		t.Fatal("expected default client when nil is passed")
	}
}

func TestWebhookSender_EmptyPayload(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var capturedBody []byte
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(req *http.Request) (*http.Response, error) {
			capturedBody, _ = io.ReadAll(req.Body)
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err != nil {
		t.Fatalf("Send with nil payload failed: %v", err)
	}

	if string(capturedBody) != "{}" {
		t.Fatalf("expected {} for nil payload, got: %s", capturedBody)
	}
}

func TestWebhookSender_ServerHitCount(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var hits atomic.Int32
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(_ *http.Request) (*http.Response, error) {
			hits.Add(1)
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = sender.Send(ctx, ch, del)
	_ = sender.Send(ctx, ch, del)

	if hits.Load() != 2 {
		t.Fatalf("server hits = %d, want 2", hits.Load())
	}
}

func testRetryPolicy() retrypolicy.RetryPolicy[*http.Response] {
	return retrypolicy.NewBuilder[*http.Response]().
		WithMaxRetries(2).
		WithBackoff(time.Millisecond, 10*time.Millisecond).
		HandleIf(func(resp *http.Response, err error) bool {
			if err != nil {
				return true
			}
			return resp.StatusCode == 429 || resp.StatusCode >= 500
		}).
		Build()
}

func TestWebhookSender_RetriesOn503(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var hits atomic.Int32
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(_ *http.Request) (*http.Response, error) {
			n := hits.Add(1)
			if n == 1 {
				return httpmock.NewStringResponse(503, "service unavailable"), nil
			}
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	rp := testRetryPolicy()
	sender := NewWebhookSender(client, WithWebhookRetryPolicy(rp))
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("server hits = %d, want 2", hits.Load())
	}
}

func TestWebhookSender_RetriesOn500(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var hits atomic.Int32
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(_ *http.Request) (*http.Response, error) {
			n := hits.Add(1)
			if n == 1 {
				return httpmock.NewStringResponse(500, "internal server error"), nil
			}
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	rp := testRetryPolicy()
	sender := NewWebhookSender(client, WithWebhookRetryPolicy(rp))
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("server hits = %d, want 2", hits.Load())
	}
}

func TestWebhookSender_NoRetryOn400(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var hits atomic.Int32
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(_ *http.Request) (*http.Response, error) {
			hits.Add(1)
			return httpmock.NewStringResponse(400, "bad request"), nil
		})

	rp := testRetryPolicy()
	sender := NewWebhookSender(client, WithWebhookRetryPolicy(rp))
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected error for 400 status")
	}
	if hits.Load() != 1 {
		t.Fatalf("server hits = %d, want 1 (no retry on 4xx)", hits.Load())
	}
}

func TestWebhookSender_RetriesOn429(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var hits atomic.Int32
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(_ *http.Request) (*http.Response, error) {
			n := hits.Add(1)
			if n == 1 {
				return httpmock.NewStringResponse(429, "too many requests"), nil
			}
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	rp := testRetryPolicy()
	sender := NewWebhookSender(client, WithWebhookRetryPolicy(rp))
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err != nil {
		t.Fatalf("expected success after retry on 429, got: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("server hits = %d, want 2 (1 initial 429 + 1 retry success)", hits.Load())
	}
}

func TestWebhookSender_NoRetryOn200(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var hits atomic.Int32
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(_ *http.Request) (*http.Response, error) {
			hits.Add(1)
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	rp := testRetryPolicy()
	sender := NewWebhookSender(client, WithWebhookRetryPolicy(rp))
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("server hits = %d, want 1 (no retry on 200)", hits.Load())
	}
}

func TestWebhookSender_ExhaustsRetries(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var hits atomic.Int32
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(_ *http.Request) (*http.Response, error) {
			hits.Add(1)
			return httpmock.NewStringResponse(503, "service unavailable"), nil
		})

	rp := testRetryPolicy()
	sender := NewWebhookSender(client, WithWebhookRetryPolicy(rp))
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if hits.Load() != 3 {
		t.Fatalf("server hits = %d, want 3 (1 initial + 2 retries)", hits.Load())
	}
}

func TestWebhookSender_StatusBoundary200(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://example.com/hook",
		httpmock.NewStringResponder(200, "ok"))

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err != nil {
		t.Fatalf("status 200 should succeed, got: %v", err)
	}
}

func TestWebhookSender_StatusBoundary199(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://example.com/hook",
		httpmock.NewStringResponder(199, ""))

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("status 199 should be rejected")
	}
}

func TestWebhookSender_StatusBoundary299(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://example.com/hook",
		httpmock.NewStringResponder(299, ""))

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err != nil {
		t.Fatalf("status 299 should succeed, got: %v", err)
	}
}

func TestWebhookSender_StatusBoundary300(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://example.com/hook",
		httpmock.NewStringResponder(300, ""))

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("status 300 should be rejected")
	}
}

func TestWebhookSender_DefaultRetryPolicy_500IsRetried(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var hits atomic.Int32
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(_ *http.Request) (*http.Response, error) {
			n := hits.Add(1)
			if n == 1 {
				return httpmock.NewStringResponse(500, "internal server error"), nil
			}
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err != nil {
		t.Fatalf("expected success after retry on 500 with default policy, got: %v", err)
	}
	if hits.Load() < 2 {
		t.Fatalf("server hits = %d, want >= 2 (default policy should retry on 500)", hits.Load())
	}
}

func TestWebhookSender_DefaultRetryPolicy_499NotRetried(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var hits atomic.Int32
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(_ *http.Request) (*http.Response, error) {
			hits.Add(1)
			return httpmock.NewStringResponse(499, "client error"), nil
		})

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected error for 499 status")
	}
	if hits.Load() != 1 {
		t.Fatalf("server hits = %d, want 1 (default policy should not retry on 499)", hits.Load())
	}
}

func TestWebhookSender_DefaultRetryPolicy_429IsRetried(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var hits atomic.Int32
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(_ *http.Request) (*http.Response, error) {
			n := hits.Add(1)
			if n == 1 {
				return httpmock.NewStringResponse(429, "too many requests"), nil
			}
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err != nil {
		t.Fatalf("expected success after retry on 429 with default policy, got: %v", err)
	}
	if hits.Load() < 2 {
		t.Fatalf("server hits = %d, want >= 2 (default policy should retry on 429)", hits.Load())
	}
}

func TestWebhookSender_DefaultClientBlocksDNSRebindingAtSendTime(t *testing.T) {
	var lookups atomic.Int32
	restore := httputil.SetLookupHostForTest(func(host string) ([]string, error) {
		if host != "rebind.test" {
			return nil, fmt.Errorf("unexpected host lookup: %s", host)
		}
		if lookups.Add(1) == 1 {
			return []string{"93.184.216.34"}, nil
		}
		return []string{"127.0.0.1"}, nil
	})
	t.Cleanup(restore)

	sender := NewWebhookSender(nil, WithWebhookRetryPolicy(nil))
	ch := newTestChannel("http://rebind.test/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{"ok":true}`))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected DNS rebinding attempt to be blocked")
	}
	if !strings.Contains(err.Error(), "private address") && !strings.Contains(err.Error(), "resolves to private") {
		t.Fatalf("expected private-address rejection, got %v", err)
	}
	if lookups.Load() < 2 {
		t.Fatalf("expected validation and dial-time DNS lookups, got %d", lookups.Load())
	}
}

func TestWebhookSender_BlocksPrivateEndpointByDefault(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := NewWebhookSender(nil, WithWebhookRetryPolicy(nil))
	ch := newTestChannel(server.URL, "")
	del := newTestDelivery("run.completed", json.RawMessage(`{"ok":true}`))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected private webhook endpoint to be blocked")
	}
	if hits.Load() != 0 {
		t.Fatalf("private webhook endpoint received %d requests, want 0", hits.Load())
	}
}

func TestWebhookSender_AllowsPrivateEndpointWhenConfigured(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := NewWebhookSender(nil, WithWebhookRetryPolicy(nil), WithWebhookAllowPrivateEndpoints(true))
	ch := newTestChannel(server.URL, "")
	del := newTestDelivery("run.completed", json.RawMessage(`{"ok":true}`))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := sender.Send(ctx, ch, del); err != nil {
		t.Fatalf("Send with private endpoints enabled failed: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("private webhook endpoint received %d requests, want 1", hits.Load())
	}
}
