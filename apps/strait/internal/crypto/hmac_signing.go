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
// The structured X-Strait-Signature is HMAC-SHA256(secret, timestamp + "." +
// deliveryID + "." + body), emitted as "t=<ts>,d=<id>,v1=<hex>"; binding the
// timestamp and delivery ID makes it replay-resistant.
//
// X-Strait-Signature-256 follows the widely used GitHub "sha256=<hex>"
// convention, which signs the raw body alone: HMAC-SHA256(secret, body). It must
// NOT carry the compound signature — a receiver copying GitHub's verification
// (HMAC over the body) would otherwise get a permanent mismatch.
//
// Callers are responsible for choosing the timestamp (RFC3339 UTC) and a
// per-delivery ID. Passing an empty secret is a programming error: SignWebhookRequest
// returns immediately without setting headers so the caller can decide whether
// to fail or to deliver unsigned.
func SignWebhookRequest(req *http.Request, secret []byte, body []byte, deliveryID, timestamp string) {
	if req == nil || len(secret) == 0 {
		return
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write([]byte(deliveryID))
	mac.Write([]byte("."))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	bodyMac := hmac.New(sha256.New, secret)
	bodyMac.Write(body)
	bodySig := hex.EncodeToString(bodyMac.Sum(nil))

	req.Header.Set("X-Strait-Timestamp", timestamp)
	req.Header.Set("X-Strait-Delivery-ID", deliveryID)
	req.Header.Set("X-Strait-Signature", "t="+timestamp+",d="+deliveryID+",v1="+sig)
	req.Header.Set("X-Strait-Signature-256", "sha256="+bodySig)
}
