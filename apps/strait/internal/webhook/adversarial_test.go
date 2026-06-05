package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// adversarialHMACSHA256 is a test helper that computes a sha256=<hex> HMAC header value.
func adversarialHMACSHA256(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// adversarialStripeSignature is a test helper that computes a Stripe-format signature header.
func adversarialStripeSignature(secret string, body []byte, ts int64) string {
	tsStr := fmt.Sprintf("%d", ts)
	payload := append([]byte(tsStr+"."), body...)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%s,v1=%s", tsStr, sig)
}

// TestSignature_NullBytesInPayload verifies that payloads containing null bytes validate correctly.
func TestSignature_NullBytesInPayload(t *testing.T) {
	t.Parallel()

	secret := "test-secret-null"
	body := []byte("before\x00after\x00\x00end")
	header := adversarialHMACSHA256(secret, body)

	err := ValidateSignature("hmac-sha256", secret, body, header)
	require.NoError(t, err)
}

// TestSignature_EmptyPayload verifies that an empty payload validates correctly.
func TestSignature_EmptyPayload(t *testing.T) {
	t.Parallel()

	secret := "test-secret-empty"
	body := []byte{}
	header := adversarialHMACSHA256(secret, body)

	err := ValidateSignature("hmac-sha256", secret, body, header)
	require.NoError(t, err)
}

// TestSignature_HugePayload verifies that a 10MB payload validates correctly.
func TestSignature_HugePayload(t *testing.T) {
	t.Parallel()

	secret := "test-secret-huge"
	body := make([]byte, 10*1024*1024)
	for i := range body {
		body[i] = byte(i % 256)
	}
	header := adversarialHMACSHA256(secret, body)

	err := ValidateSignature("hmac-sha256", secret, body, header)
	require.NoError(t, err)
}

// TestStripeSignature_FutureTimestamp verifies that a far-future timestamp (year 2100) is rejected.
func TestStripeSignature_FutureTimestamp(t *testing.T) {
	t.Parallel()

	secret := "test-secret-future"
	body := []byte(`{"event":"test"}`)
	// Year 2100 timestamp.
	futureTS := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	header := adversarialStripeSignature(secret, body, futureTS)

	err := ValidateSignature("stripe-v1", secret, body, header)
	require.Error(t, err)
	require.Contains(t, err.Error(), "timestamp too old")
}

// TestStripeSignature_NegativeTimestamp verifies that timestamp=-1 is rejected.
func TestStripeSignature_NegativeTimestamp(t *testing.T) {
	t.Parallel()

	secret := "test-secret-negative"
	body := []byte(`{"event":"test"}`)
	header := adversarialStripeSignature(secret, body, -1)

	err := ValidateSignature("stripe-v1", secret, body, header)
	require.Error(t, err)
	require.Contains(t, err.Error(), "timestamp too old")

	// The age will be very large, so it should be rejected as too old.
}

// TestStripeSignature_ZeroTimestamp verifies that timestamp=0 is rejected as too old.
func TestStripeSignature_ZeroTimestamp(t *testing.T) {
	t.Parallel()

	secret := "test-secret-zero"
	body := []byte(`{"event":"test"}`)
	header := adversarialStripeSignature(secret, body, 0)

	err := ValidateSignature("stripe-v1", secret, body, header)
	require.Error(t, err)
	require.Contains(t, err.Error(), "timestamp too old")
}

// TestStripeSignature_MaxIntTimestamp verifies that MaxInt64 timestamp is rejected.
func TestStripeSignature_MaxIntTimestamp(t *testing.T) {
	t.Parallel()

	secret := "test-secret-maxint"
	body := []byte(`{"event":"test"}`)
	header := adversarialStripeSignature(secret, body, math.MaxInt64)

	err := ValidateSignature("stripe-v1", secret, body, header)
	require.Error(t, err)
}

// FuzzHMACSHA256Adversarial fuzzes HMAC-SHA256 validation with arbitrary secrets, payloads, and signatures.
func FuzzHMACSHA256Adversarial(f *testing.F) {
	f.Add("secret", []byte("body"), "sha256=abc123")
	f.Add("", []byte(""), "sha256=")
	f.Add("key", []byte("\x00\x01\x02"), "sha256=deadbeef")

	f.Fuzz(func(t *testing.T, secret string, body []byte, sig string) {
		// Should never panic regardless of input.
		_ = ValidateSignature("hmac-sha256", secret, body, sig)
	})
}

// FuzzStripeSignatureManipulation fuzzes Stripe signature validation with arbitrary timestamps and payloads.
func FuzzStripeSignatureManipulation(f *testing.F) {
	f.Add(int64(1700000000), []byte(`{"test":true}`))
	f.Add(int64(0), []byte(""))
	f.Add(int64(-1), []byte("\x00"))

	f.Fuzz(func(t *testing.T, ts int64, body []byte) {
		header := fmt.Sprintf("t=%d,v1=deadbeef", ts)
		// Should never panic regardless of input.
		_ = ValidateSignature("stripe-v1", "fuzz-secret", body, header)
	})
}

// TestSigning_NullBytesInBody verifies that signing works with null bytes.
func TestSigning_NullBytesInBody(t *testing.T) {
	t.Parallel()
	body := []byte("before\x00middle\x00\x00after")
	sig := ComputeHMACSHA256("secret", body)
	require.Len(t, sig, 64)

	err := ValidateSignature("hmac-sha256", "secret", body, "sha256="+sig)
	require.NoError(t, err)
}

// TestSigning_HugeBody_10MB verifies signing a 10MB payload.
func TestSigning_HugeBody_10MB(t *testing.T) {
	t.Parallel()
	body := make([]byte, 10*1024*1024)
	for i := range body {
		body[i] = byte(i % 256)
	}
	sig := ComputeHMACSHA256("test-secret", body)
	require.Len(t, sig, 64)

	err := ValidateSignature("hmac-sha256", "test-secret", body, "sha256="+sig)
	require.NoError(t, err)
}

// TestSigning_UnicodeSecret verifies that Unicode secrets produce valid signatures.
func TestSigning_UnicodeSecret(t *testing.T) {
	t.Parallel()
	secret := "geheimnis-\u00e4\u00f6\u00fc-\u4e16\u754c-\U0001f512"
	body := []byte(`{"event":"unicode-test"}`)
	sig := ComputeHMACSHA256(secret, body)
	require.Len(t, sig, 64)

	err := ValidateSignature("hmac-sha256", secret, body, "sha256="+sig)
	require.NoError(t, err)
}

// TestSigning_BinaryPayload verifies signing of non-JSON binary data.
func TestSigning_BinaryPayload(t *testing.T) {
	t.Parallel()
	body := make([]byte, 512)
	for i := range body {
		body[i] = byte(i % 256)
	}
	sig := ComputeHMACSHA256("binary-secret", body)
	require.Len(t, sig, 64)

	err := ValidateSignature("hmac-sha256", "binary-secret", body, "sha256="+sig)
	require.NoError(t, err)
}

// TestSigning_Deterministic verifies that signing the same input twice gives the same result.
func TestSigning_Deterministic(t *testing.T) {
	t.Parallel()
	body := []byte(`{"stable":"true"}`)
	sig1 := ComputeHMACSHA256("stable-key", body)
	sig2 := ComputeHMACSHA256("stable-key", body)
	require.Equal(t, sig2,
		sig1)
}
