package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ValidateSignature verifies the HMAC signature of a webhook body.
// Supported algorithms: "hmac-sha256", "stripe-v1", "github-sha256".
func ValidateSignature(algorithm, secret string, body []byte, headerValue string) error {
	switch algorithm {
	case "hmac-sha256":
		return validateHMACSHA256(secret, body, headerValue)
	case "stripe-v1":
		return validateStripeV1(secret, body, headerValue)
	case "github-sha256":
		return validateGitHubSHA256(secret, body, headerValue)
	default:
		return fmt.Errorf("unsupported signature algorithm: %s", algorithm)
	}
}

// validateHMACSHA256 checks a standard sha256=<hex> HMAC signature.
func validateHMACSHA256(secret string, body []byte, headerValue string) error {
	expected, found := strings.CutPrefix(headerValue, "sha256=")
	if !found {
		return fmt.Errorf("invalid hmac-sha256 header format: missing sha256= prefix")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	computed := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(computed)) {
		return fmt.Errorf("hmac-sha256 signature mismatch")
	}
	return nil
}

// validateStripeV1 verifies a Stripe webhook signature header.
// Header format: t=<timestamp>,v1=<hmac>
// Signed payload: <timestamp>.<body>
// Rejects signatures older than 5 minutes.
func validateStripeV1(secret string, body []byte, headerValue string) error {
	var ts string
	var sig string

	for part := range strings.SplitSeq(headerValue, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts = kv[1]
		case "v1":
			sig = kv[1]
		}
	}

	if ts == "" || sig == "" {
		return fmt.Errorf("invalid stripe-v1 header: missing t or v1 components")
	}

	// Check timestamp freshness (5-minute tolerance).
	tsInt, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid stripe-v1 timestamp: %w", err)
	}
	age := math.Abs(float64(time.Now().Unix() - tsInt))
	if age > 300 {
		return fmt.Errorf("stripe-v1 timestamp too old: %ds", int(age))
	}

	// Compute expected signature: HMAC-SHA256(secret, timestamp.body)
	payload := append([]byte(ts+"."), body...)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	computed := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(computed)) {
		return fmt.Errorf("stripe-v1 signature mismatch")
	}
	return nil
}

// validateGitHubSHA256 checks a GitHub X-Hub-Signature-256 header (sha256=<hex>).
func validateGitHubSHA256(secret string, body []byte, headerValue string) error {
	return validateHMACSHA256(secret, body, headerValue)
}

// ComputeHMACSHA256 computes a HMAC-SHA256 signature and returns the hex digest.
func ComputeHMACSHA256(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
