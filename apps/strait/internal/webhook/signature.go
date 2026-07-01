package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"hash"
	"math"
	"strconv"
	"strings"
	"time"
)

var timestampedHMACSeparator = []byte{'.'}

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

	if !equalHMACHexString(expected, mac.Sum(nil)) {
		return fmt.Errorf("hmac-sha256 signature mismatch")
	}
	return nil
}

func equalHMACHexString(expected string, digest []byte) bool {
	const encodedSHA256Len = sha256.Size * 2
	if len(expected) != encodedSHA256Len || len(digest) != sha256.Size {
		return false
	}

	var encoded [encodedSHA256Len]byte
	hex.Encode(encoded[:], digest)

	var result byte
	for i := range encoded {
		result |= encoded[i] ^ expected[i]
	}
	return subtle.ConstantTimeByteEq(result, 0) == 1
}

func hmacHexString(mac hash.Hash) string {
	var digest [sha256.Size]byte
	sum := mac.Sum(digest[:0])
	var out [sha256.Size * 2]byte
	hex.Encode(out[:], sum)
	return string(out[:])
}

// validateStripeV1 verifies a Stripe webhook signature header.
// Header format: t=<timestamp>,v1=<hmac>
// Signed payload: <timestamp>.<body>
// Rejects signatures older than 5 minutes.
func validateStripeV1(secret string, body []byte, headerValue string) error {
	var ts string
	var firstSignature string
	var extraSignatures []string

	for part := range strings.SplitSeq(headerValue, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			ts = value
		case "v1":
			if firstSignature == "" {
				firstSignature = value
			} else {
				extraSignatures = append(extraSignatures, value)
			}
		}
	}

	if ts == "" || firstSignature == "" {
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

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(ts))
	_, _ = mac.Write(timestampedHMACSeparator)
	_, _ = mac.Write(body)
	var digest [sha256.Size]byte
	sum := mac.Sum(digest[:0])

	if equalHMACHexString(firstSignature, sum) {
		return nil
	}
	for _, sig := range extraSignatures {
		if equalHMACHexString(sig, sum) {
			return nil
		}
	}
	return fmt.Errorf("stripe-v1 signature mismatch")
}

// validateGitHubSHA256 checks a GitHub X-Hub-Signature-256 header (sha256=<hex>).
func validateGitHubSHA256(secret string, body []byte, headerValue string) error {
	return validateHMACSHA256(secret, body, headerValue)
}

// ComputeHMACSHA256 computes a HMAC-SHA256 signature and returns the hex digest.
func ComputeHMACSHA256(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmacHexString(mac)
}

// ComputeTimestampedHMACSHA256 signs "timestamp.body" so receivers can reject
// stale replay attempts while still validating the exact delivered bytes.
func ComputeTimestampedHMACSHA256(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write(timestampedHMACSeparator)
	_, _ = mac.Write(body)
	return hmacHexString(mac)
}
