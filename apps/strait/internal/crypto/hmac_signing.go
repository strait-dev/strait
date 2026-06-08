package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

const (
	headerStraitTimestamp    = "X-Strait-Timestamp"
	headerStraitDeliveryID   = "X-Strait-Delivery-Id"
	headerStraitSignature    = "X-Strait-Signature"
	headerStraitSignature256 = "X-Strait-Signature-256"
)

var hmacSigningSeparator = []byte(".")

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
	mac.Write(hmacSigningSeparator)
	mac.Write([]byte(deliveryID))
	mac.Write(hmacSigningSeparator)
	mac.Write(body)
	var sum [sha256.Size]byte
	mac.Sum(sum[:0])
	var sigHex [sha256.Size * 2]byte
	hex.Encode(sigHex[:], sum[:])
	setCanonicalHeader(req.Header, headerStraitTimestamp, timestamp)
	setCanonicalHeader(req.Header, headerStraitDeliveryID, deliveryID)
	setCanonicalHeader(req.Header, headerStraitSignature, webhookStructuredSignatureHeader(timestamp, deliveryID, sigHex))
	setCanonicalHeader(req.Header, headerStraitSignature256, webhookSHA256SignatureHeader(sigHex))
}

func setCanonicalHeader(header http.Header, key string, value string) {
	values := header[key]
	if len(values) == 0 {
		header[key] = []string{value}
		return
	}
	values[0] = value
	header[key] = values[:1]
}

func webhookStructuredSignatureHeader(timestamp, deliveryID string, sigHex [sha256.Size * 2]byte) string {
	const prefixLen = len("t=") + len(",d=") + len(",v1=")
	size := prefixLen + len(timestamp) + len(deliveryID) + len(sigHex)
	var stack [160]byte
	out := stack[:0]
	if size > len(stack) {
		out = make([]byte, 0, size)
	}
	out = append(out, "t="...)
	out = append(out, timestamp...)
	out = append(out, ",d="...)
	out = append(out, deliveryID...)
	out = append(out, ",v1="...)
	out = append(out, sigHex[:]...)
	return string(out)
}

func webhookSHA256SignatureHeader(sigHex [sha256.Size * 2]byte) string {
	var stack [len("sha256=") + sha256.Size*2]byte
	out := append(stack[:0], "sha256="...)
	out = append(out, sigHex[:]...)
	return string(out)
}
