package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignWebhookRequest_GoldenHeaders(t *testing.T) {
	t.Parallel()

	secret := []byte("test-secret")
	body := []byte(`{"event":"api_key.auto_rotated"}`)
	deliveryID := "01HXYZ0123456789ABCDEFGHJK"
	timestamp := "2026-05-11T12:00:00Z"

	req := httptest.NewRequest(http.MethodPost, "https://example.com/hook", nil)
	SignWebhookRequest(req, secret, body, deliveryID, timestamp)

	// X-Strait-Signature is v1=<hex> over timestamp + "." + body, matching every
	// other Strait signing path (delivery id is a header, not part of the HMAC).
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	wantSig := hex.EncodeToString(mac.Sum(nil))

	// The sha256= header is a body-only HMAC (GitHub convention).
	bodyMac := hmac.New(sha256.New, secret)
	bodyMac.Write(body)
	wantBodySig := hex.EncodeToString(bodyMac.Sum(nil))

	assert.Equal(t, timestamp, req.Header.Get("X-Strait-Timestamp"))
	assert.Equal(t, deliveryID, req.Header.Get("X-Strait-Delivery-ID"))
	assert.Equal(t, "v1="+wantSig, req.Header.Get("X-Strait-Signature"))
	assert.Equal(t, "sha256="+wantBodySig, req.Header.Get("X-Strait-Signature-256"))
	assert.NotEqual(t, wantSig, wantBodySig, "timestamped and body-only signatures must differ")
}

func TestSignWebhookRequest_NoSecretIsNoop(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/hook", nil)
	SignWebhookRequest(req, nil, []byte("body"), "id", "ts")
	SignWebhookRequest(req, []byte{}, []byte("body"), "id", "ts")
	for _, h := range []string{"X-Strait-Timestamp", "X-Strait-Delivery-ID", "X-Strait-Signature", "X-Strait-Signature-256"} {
		assert.Empty(t, req.Header.Get(h), "header %s", h)
	}
}

func TestSignWebhookRequest_NilRequestIsNoop(t *testing.T) {
	t.Parallel()

	SignWebhookRequest(nil, []byte("s"), []byte("b"), "id", "ts")
}

func TestSignWebhookRequest_SignatureHeaderFormat(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/hook", nil)
	SignWebhookRequest(req, []byte("s"), []byte("b"), "deliv", "2026-01-01T00:00:00Z")

	// Regression guard for signing consistency: X-Strait-Signature is the same
	// v1=<hex> single-token format used by every other Strait signing path, not a
	// compound t=,d=,v1= value.
	sig := req.Header.Get("X-Strait-Signature")
	require.True(t, strings.HasPrefix(sig, "v1="))
	require.NotContains(t, sig, ",", "signature must be a single v1= token")
	require.NotContains(t, sig, "t=")
}

func TestSignWebhookRequest_ReplacesExistingHeaderValues(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/hook", nil)
	req.Header[headerStraitTimestamp] = []string{"old", "duplicate"}
	req.Header[headerStraitDeliveryID] = []string{"old", "duplicate"}
	req.Header[headerStraitSignature] = []string{"old", "duplicate"}
	req.Header[headerStraitSignature256] = []string{"old", "duplicate"}

	SignWebhookRequest(req, []byte("s"), []byte("b"), "deliv", "2026-01-01T00:00:00Z")

	assert.Equal(t, []string{"2026-01-01T00:00:00Z"}, req.Header.Values(headerStraitTimestamp))
	assert.Equal(t, []string{"deliv"}, req.Header.Values(headerStraitDeliveryID))
	assert.Len(t, req.Header.Values(headerStraitSignature), 1)
	assert.Len(t, req.Header.Values(headerStraitSignature256), 1)
}

func BenchmarkSignWebhookRequest(b *testing.B) {
	req := httptest.NewRequest(http.MethodPost, "https://example.com/hook", nil)
	secret := []byte("test-secret")
	body := []byte(`{"event":"api_key.auto_rotated","id":"01HXYZ0123456789ABCDEFGHJK"}`)
	deliveryID := "01HXYZ0123456789ABCDEFGHJK"
	timestamp := "2026-05-11T12:00:00Z"

	b.ReportAllocs()
	for b.Loop() {
		SignWebhookRequest(req, secret, body, deliveryID, timestamp)
		if req.Header.Get("X-Strait-Signature-256") == "" {
			b.Fatal("missing signature header")
		}
	}
}
