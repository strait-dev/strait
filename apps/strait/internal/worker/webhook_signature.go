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
	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	payload := append([]byte(ts+"."), body...)
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	_, _ = mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))

	req.Header.Set("X-Strait-Timestamp", ts)
	req.Header.Set("X-Strait-Signature", "v1="+sig)
	req.Header.Set("X-Webhook-Signature", "v1="+sig)
}
