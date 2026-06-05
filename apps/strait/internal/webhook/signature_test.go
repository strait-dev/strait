package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func computeHMACSHA256(secret string, data []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestValidateSignature_HMACSHA256(t *testing.T) {
	t.Parallel()
	secret := "test-secret-key"
	body := []byte(`{"event":"test"}`)
	sig := computeHMACSHA256(secret, body)

	t.Run("valid signature", func(t *testing.T) {
		t.Parallel()
		err := ValidateSignature("hmac-sha256", secret, body, "sha256="+sig)
		require.NoError(t, err)
	})

	t.Run("invalid signature", func(t *testing.T) {
		t.Parallel()
		err := ValidateSignature("hmac-sha256", secret, body, "sha256=deadbeef")
		require.Error(t, err)
	})

	t.Run("missing prefix", func(t *testing.T) {
		t.Parallel()
		err := ValidateSignature("hmac-sha256", secret, body, sig)
		require.Error(t, err)
	})

	t.Run("wrong secret", func(t *testing.T) {
		t.Parallel()
		err := ValidateSignature("hmac-sha256", "wrong-secret", body, "sha256="+sig)
		require.Error(t, err)
	})

	t.Run("empty body", func(t *testing.T) {
		t.Parallel()
		emptySig := computeHMACSHA256(secret, []byte{})
		err := ValidateSignature("hmac-sha256", secret, []byte{}, "sha256="+emptySig)
		require.NoError(t, err)
	})
}

func TestValidateSignature_StripeV1(t *testing.T) {
	t.Parallel()
	secret := "whsec_test123"
	body := []byte(`{"id":"evt_1234","type":"payment_intent.succeeded"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	signStripe := func(timestamp string, payload []byte) string {
		signed := append([]byte(timestamp+"."), payload...)
		return computeHMACSHA256(secret, signed)
	}

	t.Run("valid signature", func(t *testing.T) {
		t.Parallel()
		sig := signStripe(ts, body)
		header := fmt.Sprintf("t=%s,v1=%s", ts, sig)
		err := ValidateSignature("stripe-v1", secret, body, header)
		require.NoError(t, err)
	})

	t.Run("invalid signature", func(t *testing.T) {
		t.Parallel()
		header := fmt.Sprintf("t=%s,v1=deadbeef", ts)
		err := ValidateSignature("stripe-v1", secret, body, header)
		require.Error(t, err)
	})

	t.Run("expired timestamp", func(t *testing.T) {
		t.Parallel()
		oldTS := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
		sig := signStripe(oldTS, body)
		header := fmt.Sprintf("t=%s,v1=%s", oldTS, sig)
		err := ValidateSignature("stripe-v1", secret, body, header)
		require.Error(t, err)
	})

	t.Run("missing t component", func(t *testing.T) {
		t.Parallel()
		err := ValidateSignature("stripe-v1", secret, body, "v1=abc123")
		require.Error(t, err)
	})

	t.Run("missing v1 component", func(t *testing.T) {
		t.Parallel()
		err := ValidateSignature("stripe-v1", secret, body, "t=12345")
		require.Error(t, err)
	})

	t.Run("extra components ignored", func(t *testing.T) {
		t.Parallel()
		sig := signStripe(ts, body)
		header := fmt.Sprintf("t=%s,v1=%s,v0=oldval", ts, sig)
		err := ValidateSignature("stripe-v1", secret, body, header)
		require.NoError(t, err)
	})

	t.Run("accepts any matching v1 signature", func(t *testing.T) {
		t.Parallel()
		sig := signStripe(ts, body)
		header := fmt.Sprintf("t=%s,v1=%s,v1=deadbeef", ts, sig)
		err := ValidateSignature("stripe-v1", secret, body, header)
		require.NoError(t, err)
	})
}

func TestValidateSignature_GitHubSHA256(t *testing.T) {
	t.Parallel()
	secret := "github-webhook-secret"
	body := []byte(`{"action":"push","ref":"refs/heads/main"}`)
	sig := computeHMACSHA256(secret, body)

	t.Run("valid signature", func(t *testing.T) {
		t.Parallel()
		err := ValidateSignature("github-sha256", secret, body, "sha256="+sig)
		require.NoError(t, err)
	})

	t.Run("invalid signature", func(t *testing.T) {
		t.Parallel()
		err := ValidateSignature("github-sha256", secret, body, "sha256=badhex")
		require.Error(t, err)
	})
}

func TestValidateSignature_UnsupportedAlgorithm(t *testing.T) {
	t.Parallel()
	err := ValidateSignature("rsa-sha512", "key", []byte("body"), "sig")
	require.Error(t, err)
}

func TestValidateSignature_StripeV1_TimestampBoundary(t *testing.T) {
	t.Parallel()
	secret := "test"
	body := []byte(`{}`)

	t.Run("exactly 300 seconds old is accepted", func(t *testing.T) {
		t.Parallel()
		ts := strconv.FormatInt(time.Now().Add(-300*time.Second).Unix(), 10)
		signed := append([]byte(ts+"."), body...)
		sig := computeHMACSHA256(secret, signed)
		err := ValidateSignature("stripe-v1", secret, body, fmt.Sprintf("t=%s,v1=%s", ts, sig))
		require.NoError(t, err)
	})

	t.Run("301 seconds old is rejected", func(t *testing.T) {
		t.Parallel()
		ts := strconv.FormatInt(time.Now().Add(-301*time.Second).Unix(), 10)
		signed := append([]byte(ts+"."), body...)
		sig := computeHMACSHA256(secret, signed)
		err := ValidateSignature("stripe-v1", secret, body, fmt.Sprintf("t=%s,v1=%s", ts, sig))
		require.Error(t, err)
	})

	t.Run("future timestamp is accepted within window", func(t *testing.T) {
		t.Parallel()
		ts := strconv.FormatInt(time.Now().Add(60*time.Second).Unix(), 10)
		signed := append([]byte(ts+"."), body...)
		sig := computeHMACSHA256(secret, signed)
		err := ValidateSignature("stripe-v1", secret, body, fmt.Sprintf("t=%s,v1=%s", ts, sig))
		require.NoError(t, err)
	})
}

func TestValidateSignature_HMACSHA256_EmptySecret(t *testing.T) {
	t.Parallel()
	body := []byte(`test payload`)
	sig := computeHMACSHA256("", body)
	err := ValidateSignature("hmac-sha256", "", body, "sha256="+sig)
	require.NoError(t, err)
}

func TestValidateSignature_StripeV1_InvalidTimestamp(t *testing.T) {
	t.Parallel()
	err := ValidateSignature("stripe-v1", "secret", []byte(`{}`), "t=notanumber,v1=abc123")
	require.Error(t, err)
}

func TestValidateSignature_LargeBody(t *testing.T) {
	t.Parallel()
	secret := "test-key"
	body := make([]byte, 1<<20) // 1MB
	for i := range body {
		body[i] = byte('A' + (i % 26))
	}
	sig := computeHMACSHA256(secret, body)
	err := ValidateSignature("hmac-sha256", secret, body, "sha256="+sig)
	require.NoError(t, err)
}

func TestComputeHMACSHA256_KnownVector(t *testing.T) {
	t.Parallel()
	// Pre-computed: HMAC-SHA256("test-secret", "hello world")
	secret := "test-secret"
	body := []byte("hello world")
	got := ComputeHMACSHA256(secret, body)
	// Verify by computing independently.
	want := computeHMACSHA256(secret, body)
	require.Equal(t, want,
		got)
	require.Len(t, got, 64)

	// Must be 64-char hex.
}

func TestComputeHMACSHA256_EmptyBody(t *testing.T) {
	t.Parallel()
	got := ComputeHMACSHA256("secret", []byte{})
	require.Len(t, got, 64)
}

func TestComputeHMACSHA256_EmptySecret(t *testing.T) {
	t.Parallel()
	got := ComputeHMACSHA256("", []byte("body"))
	require.Len(t, got, 64)
}

func TestComputeHMACSHA256_DifferentSecretsProduceDifferentSignatures(t *testing.T) {
	t.Parallel()
	body := []byte(`{"event":"test"}`)
	sig1 := ComputeHMACSHA256("secret-a", body)
	sig2 := ComputeHMACSHA256("secret-b", body)
	require.NotEqual(t, sig2,
		sig1)
}

func TestComputeHMACSHA256_ValidatesCorrectly(t *testing.T) {
	t.Parallel()
	secret := "signing-key-42"
	body := []byte(`{"run_id":"run_abc","status":"completed"}`)
	sig := ComputeHMACSHA256(secret, body)
	// The produced signature should be verifiable by ValidateSignature.
	err := ValidateSignature("hmac-sha256", secret, body, "sha256="+sig)
	require.NoError(t, err)
}
