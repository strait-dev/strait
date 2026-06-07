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

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write([]byte(deliveryID))
	mac.Write([]byte("."))
	mac.Write(body)
	wantSig := hex.EncodeToString(mac.Sum(nil))

	// The sha256= header must be a body-only HMAC (GitHub convention), distinct
	// from the compound structured signature.
	bodyMac := hmac.New(sha256.New, secret)
	bodyMac.Write(body)
	wantBodySig := hex.EncodeToString(bodyMac.Sum(nil))

	assert.Equal(t, timestamp, req.Header.Get("X-Strait-Timestamp"))
	assert.Equal(t, deliveryID, req.Header.Get("X-Strait-Delivery-ID"))
	wantStructured := "t=" + timestamp + ",d=" + deliveryID + ",v1=" + wantSig
	assert.Equal(t, wantStructured, req.Header.Get("X-Strait-Signature"))
	assert.Equal(t, "sha256="+wantBodySig, req.Header.Get("X-Strait-Signature-256"))
	assert.NotEqual(t, wantSig, wantBodySig, "compound and body-only signatures must differ")
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

func TestSignWebhookRequest_StructuredHeaderFormat(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/hook", nil)
	SignWebhookRequest(req, []byte("s"), []byte("b"), "deliv", "2026-01-01T00:00:00Z")

	sig := req.Header.Get("X-Strait-Signature")
	parts := strings.Split(sig, ",")
	require.Len(t, parts, 3)
	require.True(t, strings.HasPrefix(parts[0], "t="))
	require.True(t, strings.HasPrefix(parts[1], "d="))
	require.True(t, strings.HasPrefix(parts[2], "v1="))
}
