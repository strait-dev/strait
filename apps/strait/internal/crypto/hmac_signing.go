package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

// SignWebhookRequest adds the canonical X-Strait-{Timestamp,Delivery-ID,Signature}
// headers (plus X-Strait-Signature-256 for sha256 receivers) to req.
//
// X-Strait-Signature uses the same scheme as every other Strait signing path
// (worker HTTP dispatch, subscription/event webhooks, the test webhook):
// "v1=<hex>" where the HMAC covers timestamp + "." + body. The timestamp and a
// per-delivery id are carried in their own headers (X-Strait-Timestamp and
// X-Strait-Delivery-ID), so a single verification routine works across all
// webhook types. Replay resistance comes from the receiver checking the
// timestamp freshness, exactly as the other paths intend.
//
// X-Strait-Signature-256 follows the widely used GitHub "sha256=<hex>"
// convention, which signs the raw body alone: HMAC-SHA256(secret, body).
//
// Callers choose the timestamp and a per-delivery ID. Passing an empty secret is
// a programming error: SignWebhookRequest returns immediately without setting
// headers so the caller can decide whether to fail or to deliver unsigned.
func SignWebhookRequest(req *http.Request, secret []byte, body []byte, deliveryID, timestamp string) {
	if req == nil || len(secret) == 0 {
		return
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	bodyMac := hmac.New(sha256.New, secret)
	bodyMac.Write(body)
	bodySig := hex.EncodeToString(bodyMac.Sum(nil))

	req.Header.Set("X-Strait-Timestamp", timestamp)
	req.Header.Set("X-Strait-Delivery-ID", deliveryID)
	req.Header.Set("X-Strait-Signature", "v1="+sig)
	req.Header.Set("X-Strait-Signature-256", "sha256="+bodySig)
}
