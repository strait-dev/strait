package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"testing"
	"time"
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
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		t.Parallel()
		err := ValidateSignature("hmac-sha256", secret, body, "sha256=deadbeef")
		if err == nil {
			t.Fatal("expected error for invalid signature")
		}
	})

	t.Run("missing prefix", func(t *testing.T) {
		t.Parallel()
		err := ValidateSignature("hmac-sha256", secret, body, sig)
		if err == nil {
			t.Fatal("expected error for missing sha256= prefix")
		}
	})

	t.Run("wrong secret", func(t *testing.T) {
		t.Parallel()
		err := ValidateSignature("hmac-sha256", "wrong-secret", body, "sha256="+sig)
		if err == nil {
			t.Fatal("expected error for wrong secret")
		}
	})

	t.Run("empty body", func(t *testing.T) {
		t.Parallel()
		emptySig := computeHMACSHA256(secret, []byte{})
		err := ValidateSignature("hmac-sha256", secret, []byte{}, "sha256="+emptySig)
		if err != nil {
			t.Fatalf("expected no error for empty body, got %v", err)
		}
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
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		t.Parallel()
		header := fmt.Sprintf("t=%s,v1=deadbeef", ts)
		err := ValidateSignature("stripe-v1", secret, body, header)
		if err == nil {
			t.Fatal("expected error for invalid signature")
		}
	})

	t.Run("expired timestamp", func(t *testing.T) {
		t.Parallel()
		oldTS := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
		sig := signStripe(oldTS, body)
		header := fmt.Sprintf("t=%s,v1=%s", oldTS, sig)
		err := ValidateSignature("stripe-v1", secret, body, header)
		if err == nil {
			t.Fatal("expected error for expired timestamp")
		}
	})

	t.Run("missing t component", func(t *testing.T) {
		t.Parallel()
		err := ValidateSignature("stripe-v1", secret, body, "v1=abc123")
		if err == nil {
			t.Fatal("expected error for missing t component")
		}
	})

	t.Run("missing v1 component", func(t *testing.T) {
		t.Parallel()
		err := ValidateSignature("stripe-v1", secret, body, "t=12345")
		if err == nil {
			t.Fatal("expected error for missing v1 component")
		}
	})

	t.Run("extra components ignored", func(t *testing.T) {
		t.Parallel()
		sig := signStripe(ts, body)
		header := fmt.Sprintf("t=%s,v1=%s,v0=oldval", ts, sig)
		err := ValidateSignature("stripe-v1", secret, body, header)
		if err != nil {
			t.Fatalf("expected no error with extra components, got %v", err)
		}
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
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		t.Parallel()
		err := ValidateSignature("github-sha256", secret, body, "sha256=badhex")
		if err == nil {
			t.Fatal("expected error for invalid signature")
		}
	})
}

func TestValidateSignature_UnsupportedAlgorithm(t *testing.T) {
	t.Parallel()
	err := ValidateSignature("rsa-sha512", "key", []byte("body"), "sig")
	if err == nil {
		t.Fatal("expected error for unsupported algorithm")
	}
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
		if err != nil {
			t.Fatalf("300s should be accepted, got %v", err)
		}
	})

	t.Run("301 seconds old is rejected", func(t *testing.T) {
		t.Parallel()
		ts := strconv.FormatInt(time.Now().Add(-301*time.Second).Unix(), 10)
		signed := append([]byte(ts+"."), body...)
		sig := computeHMACSHA256(secret, signed)
		err := ValidateSignature("stripe-v1", secret, body, fmt.Sprintf("t=%s,v1=%s", ts, sig))
		if err == nil {
			t.Fatal("301s should be rejected")
		}
	})

	t.Run("future timestamp is accepted within window", func(t *testing.T) {
		t.Parallel()
		ts := strconv.FormatInt(time.Now().Add(60*time.Second).Unix(), 10)
		signed := append([]byte(ts+"."), body...)
		sig := computeHMACSHA256(secret, signed)
		err := ValidateSignature("stripe-v1", secret, body, fmt.Sprintf("t=%s,v1=%s", ts, sig))
		if err != nil {
			t.Fatalf("future timestamp within 5min should be accepted, got %v", err)
		}
	})
}

func TestValidateSignature_HMACSHA256_EmptySecret(t *testing.T) {
	t.Parallel()
	body := []byte(`test payload`)
	sig := computeHMACSHA256("", body)
	err := ValidateSignature("hmac-sha256", "", body, "sha256="+sig)
	if err != nil {
		t.Fatalf("empty secret should still validate, got %v", err)
	}
}

func TestValidateSignature_StripeV1_InvalidTimestamp(t *testing.T) {
	t.Parallel()
	err := ValidateSignature("stripe-v1", "secret", []byte(`{}`), "t=notanumber,v1=abc123")
	if err == nil {
		t.Fatal("expected error for non-numeric timestamp")
	}
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
	if err != nil {
		t.Fatalf("large body should validate, got %v", err)
	}
}
