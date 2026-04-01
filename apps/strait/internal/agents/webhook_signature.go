package agents

import (
	"crypto/hmac"
	"crypto/rand"
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
	// Allocate a new buffer to avoid mutating the caller's body slice.
	prefix := []byte(ts + ".")
	signedPayload := make([]byte, 0, len(prefix)+len(body))
	signedPayload = append(signedPayload, prefix...)
	signedPayload = append(signedPayload, body...)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(signedPayload)
	sig := hex.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("t=%s,v1=%s", ts, sig)
}

// GenerateWebhookSecret creates a new webhook secret with the "whsec_" prefix.
func GenerateWebhookSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return "whsec_" + hex.EncodeToString(b)
}

// SetWebhookSecret merges a webhook secret into the agent config JSON.
// Returns the updated config. Creates a new object if config is nil/empty.
func SetWebhookSecret(config json.RawMessage, secret string) json.RawMessage {
	var cfg map[string]any
	if len(config) > 0 {
		_ = json.Unmarshal(config, &cfg)
	}
	if cfg == nil {
		cfg = make(map[string]any)
	}
	cfg["webhook_secret"] = secret
	raw, _ := json.Marshal(cfg)
	return raw
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
