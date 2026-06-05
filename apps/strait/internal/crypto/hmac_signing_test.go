package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSignWebhookRequest_GoldenHeaders(t *testing.T) {
	t.Parallel()

	secret := []byte("test-secret")
	body := []byte(`{"event":"api_key.auto_rotated"}`)
	deliveryID := "01HXYZ0123456789ABCDEFGHJK"
	timestamp := "2026-05-11T12:00:00Z"

	req := httptest.NewRequest(http.MethodPost, "https://example.com/hook", nil)
	SignWebhookRequest(req, secret, body, deliveryID, timestamp)

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write([]byte(deliveryID))
	mac.Write([]byte("."))
	mac.Write(body)
	wantSig := hex.EncodeToString(mac.Sum(nil))

	if got := req.Header.Get("X-Strait-Timestamp"); got != timestamp {
		t.Fatalf("X-Strait-Timestamp: got %q, want %q", got, timestamp)
	}
	if got := req.Header.Get("X-Strait-Delivery-ID"); got != deliveryID {
		t.Fatalf("X-Strait-Delivery-ID: got %q, want %q", got, deliveryID)
	}
	wantStructured := "t=" + timestamp + ",d=" + deliveryID + ",v1=" + wantSig
	if got := req.Header.Get("X-Strait-Signature"); got != wantStructured {
		t.Fatalf("X-Strait-Signature: got %q, want %q", got, wantStructured)
	}
	if got := req.Header.Get("X-Strait-Signature-256"); got != "sha256="+wantSig {
		t.Fatalf("X-Strait-Signature-256: got %q, want %q", got, "sha256="+wantSig)
	}
	if got := req.Header.Get("X-Signature-256"); got != "" {
		t.Fatalf("legacy X-Signature-256 should be unset, got %q", got)
	}
}

func TestSignWebhookRequest_NoSecretIsNoop(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/hook", nil)
	SignWebhookRequest(req, nil, []byte("body"), "id", "ts")
	SignWebhookRequest(req, []byte{}, []byte("body"), "id", "ts")
	for _, h := range []string{"X-Strait-Timestamp", "X-Strait-Delivery-ID", "X-Strait-Signature", "X-Strait-Signature-256", "X-Signature-256"} {
		if v := req.Header.Get(h); v != "" {
			t.Fatalf("expected %s unset, got %q", h, v)
		}
	}
}

func TestSignWebhookRequest_NilRequestIsNoop(t *testing.T) {
	t.Parallel()

	SignWebhookRequest(nil, []byte("s"), []byte("b"), "id", "ts")
}

func TestSignWebhookRequest_StructuredHeaderFormat(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/hook", nil)
	SignWebhookRequest(req, []byte("s"), []byte("b"), "deliv", "2026-01-01T00:00:00Z")

	sig := req.Header.Get("X-Strait-Signature")
	parts := strings.Split(sig, ",")
	if len(parts) != 3 {
		t.Fatalf("expected 3 comma-separated parts, got %d in %q", len(parts), sig)
	}
	if !strings.HasPrefix(parts[0], "t=") {
		t.Fatalf("expected first part t=..., got %q", parts[0])
	}
	if !strings.HasPrefix(parts[1], "d=") {
		t.Fatalf("expected second part d=..., got %q", parts[1])
	}
	if !strings.HasPrefix(parts[2], "v1=") {
		t.Fatalf("expected third part v1=..., got %q", parts[2])
	}
}
