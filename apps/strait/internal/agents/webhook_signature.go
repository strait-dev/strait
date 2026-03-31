package agents

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SignWebhookPayload computes an HMAC-SHA256 signature for a webhook payload.
// Returns the X-Strait-Signature header value in the format: t=<unix_ts>,v1=<hex_hmac>
// Compatible with ValidateSignature("stripe-v1", ...) from the webhook package.
// Returns empty string if secret is empty (no signing).
func SignWebhookPayload(secret string, body []byte, timestamp time.Time) string {
	if strings.TrimSpace(secret) == "" {
		return ""
	}

	ts := fmt.Sprintf("%d", timestamp.Unix())

	// Signed payload: <timestamp>.<body> (same format as Stripe).
	signedPayload := append([]byte(ts+"."), body...)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(signedPayload)
	sig := hex.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("t=%s,v1=%s", ts, sig)
}

// ExtractWebhookSecret reads the webhook_secret from agent config.
// Returns empty string if not configured.
func ExtractWebhookSecret(config json.RawMessage) string {
	if len(config) == 0 {
		return ""
	}
	var cfg map[string]any
	if err := json.Unmarshal(config, &cfg); err != nil {
		return ""
	}
	if secret, ok := cfg["webhook_secret"].(string); ok {
		return strings.TrimSpace(secret)
	}
	return ""
}
