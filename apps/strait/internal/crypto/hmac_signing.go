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

type hmacSignatureHex [64]byte

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
	mac.Write(hmacSigningSeparator)
	mac.Write(body)
	var sum [sha256.Size]byte
	mac.Sum(sum[:0])
	var sigHex hmacSignatureHex
	hex.Encode(sigHex[:], sum[:])

	bodyMac := hmac.New(sha256.New, secret)
	bodyMac.Write(body)
	var bodySum [sha256.Size]byte
	bodyMac.Sum(bodySum[:0])
	var bodySigHex hmacSignatureHex
	hex.Encode(bodySigHex[:], bodySum[:])

	setCanonicalHeader(req.Header, headerStraitTimestamp, timestamp)
	setCanonicalHeader(req.Header, headerStraitDeliveryID, deliveryID)
	setCanonicalHeader(req.Header, headerStraitSignature, webhookV1SignatureHeader(sigHex))
	setCanonicalHeader(req.Header, headerStraitSignature256, webhookSHA256SignatureHeader(bodySigHex))
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

func webhookV1SignatureHeader(sigHex hmacSignatureHex) string {
	return "v1=" + string(sigHex[:])
}

func webhookSHA256SignatureHeader(sigHex hmacSignatureHex) string {
	return "sha256=" + string(sigHex[:])
}
