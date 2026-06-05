package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

// SignWebhookRequest adds the canonical X-Strait-{Timestamp,Delivery-ID,Signature}
// headers (plus X-Strait-Signature-256 for sha256 receivers) to req. The signature is
// computed as HMAC-SHA256(secret, timestamp + "." + deliveryID + "." + body)
// and emitted as the structured header "t=<ts>,d=<id>,v1=<hex>".
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
	req.Header.Set("X-Strait-Timestamp", timestamp)
	req.Header.Set("X-Strait-Delivery-ID", deliveryID)
	req.Header.Set("X-Strait-Signature", "t="+timestamp+",d="+deliveryID+",v1="+sig)
	req.Header.Set("X-Strait-Signature-256", "sha256="+sig)
}
