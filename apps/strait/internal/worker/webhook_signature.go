package worker

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"
)

func applyWebhookSignature(req *http.Request, webhookSecret string, body []byte) {
	if webhookSecret == "" {
		return
	}
	var tsBuf [20]byte
	tsBytes := strconv.AppendInt(tsBuf[:0], time.Now().UTC().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	_, _ = mac.Write(tsBytes)
	_, _ = mac.Write(hmacSeparator)
	_, _ = mac.Write(body)

	var sum [sha256.Size]byte
	digest := mac.Sum(sum[:0])
	var sig [len("v1=") + sha256.Size*2]byte
	copy(sig[:], "v1=")
	hex.Encode(sig[len("v1="):], digest)
	ts := string(tsBytes)
	signature := string(sig[:])

	setWebhookHeader(req.Header, "X-Strait-Timestamp", ts)
	setWebhookHeader(req.Header, "X-Strait-Signature", signature)
	setWebhookHeader(req.Header, "X-Webhook-Signature", signature)
}

func setWebhookHeader(header http.Header, key string, value string) {
	values := header[key]
	if len(values) == 0 {
		header[key] = []string{value}
		return
	}
	values[0] = value
	header[key] = values[:1]
}
