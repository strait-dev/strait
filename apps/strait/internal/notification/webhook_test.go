package notification

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

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

	err := sender.Send(context.Background(), ch, del)
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

	err := sender.Send(context.Background(), ch, del)
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

	err := sender.Send(context.Background(), ch, del)
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

	err := sender.Send(context.Background(), ch, del)
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestWebhookSender_HMACSignature(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	secret := "my-webhook-secret"
	payload := json.RawMessage(`{"run_id":"r-1","status":"completed"}`)

	var capturedSig string
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(req *http.Request) (*http.Response, error) {
			capturedSig = req.Header.Get("X-Signature-256")
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", secret)
	del := newTestDelivery("run.completed", payload)

	err := sender.Send(context.Background(), ch, del)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if capturedSig != expected {
		t.Fatalf("signature mismatch:\n  got:  %s\n  want: %s", capturedSig, expected)
	}
}

func TestWebhookSender_HMACSignature_NoSecret(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	var hasSigHeader bool
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(req *http.Request) (*http.Response, error) {
			hasSigHeader = req.Header.Get("X-Signature-256") != ""
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	_ = sender.Send(context.Background(), ch, del)

	if hasSigHeader {
		t.Fatal("expected no X-Signature-256 header when secret is empty")
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

	_ = sender.Send(context.Background(), ch, del)

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

	_ = sender.Send(context.Background(), ch, del)

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

	_ = sender.Send(context.Background(), ch, del)

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

	err := sender.Send(context.Background(), ch, del)
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

	err := sender.Send(context.Background(), ch, del)
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

	_ = sender.Send(context.Background(), ch, del)
	_ = sender.Send(context.Background(), ch, del)

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

	err := sender.Send(context.Background(), ch, del)
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

	err := sender.Send(context.Background(), ch, del)
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

	err := sender.Send(context.Background(), ch, del)
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

	err := sender.Send(context.Background(), ch, del)
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

	err := sender.Send(context.Background(), ch, del)
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

	err := sender.Send(context.Background(), ch, del)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if hits.Load() != 3 {
		t.Fatalf("server hits = %d, want 3 (1 initial + 2 retries)", hits.Load())
	}
}
