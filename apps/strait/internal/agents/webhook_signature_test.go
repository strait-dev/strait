package agents

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"strait/internal/testutil"
	"strait/internal/webhook"
)

func TestSignWebhookPayload_ProducesValidSignature(t *testing.T) {
	t.Parallel()
	secret := testutil.GenerateTestWebhookSecret()
	body := []byte(`{"event":"agent.run.terminal","run_id":"run-1"}`)
	ts := time.Now()

	sig := SignWebhookPayload(secret, body, ts)
	if sig == "" {
		t.Fatal("expected non-empty signature")
	}

	// Verify using the existing Stripe-v1 validator (same format).
	err := webhook.ValidateSignature("stripe-v1", secret, body, sig)
	if err != nil {
		t.Fatalf("signature validation failed: %v", err)
	}
}

func TestSignWebhookPayload_DifferentBodies(t *testing.T) {
	t.Parallel()
	secret := testutil.GenerateTestWebhookSecret()
	ts := time.Now()

	sig1 := SignWebhookPayload(secret, []byte(`{"a":1}`), ts)
	sig2 := SignWebhookPayload(secret, []byte(`{"b":2}`), ts)

	if sig1 == sig2 {
		t.Fatal("different bodies should produce different signatures")
	}
}

func TestSignWebhookPayload_DifferentSecrets(t *testing.T) {
	t.Parallel()
	body := []byte(`{"data":"test"}`)
	ts := time.Now()

	sig1 := SignWebhookPayload("secret-A", body, ts)
	sig2 := SignWebhookPayload("secret-B", body, ts)

	if sig1 == sig2 {
		t.Fatal("different secrets should produce different signatures")
	}
}

func TestSignWebhookPayload_IncludesTimestamp(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	sig := SignWebhookPayload("secret", []byte("body"), ts)

	if !strings.Contains(sig, "t=") {
		t.Fatalf("signature should contain t= component: %q", sig)
	}
	if !strings.Contains(sig, "v1=") {
		t.Fatalf("signature should contain v1= component: %q", sig)
	}
}

func TestSignWebhookPayload_EmptySecret(t *testing.T) {
	t.Parallel()
	sig := SignWebhookPayload("", []byte("body"), time.Now())
	if sig != "" {
		t.Fatalf("expected empty signature for empty secret, got %q", sig)
	}
}

func TestSignWebhookPayload_WhitespaceSecret(t *testing.T) {
	t.Parallel()
	sig := SignWebhookPayload("   ", []byte("body"), time.Now())
	if sig != "" {
		t.Fatalf("expected empty signature for whitespace secret, got %q", sig)
	}
}

func TestSignWebhookPayload_NilBody(t *testing.T) {
	t.Parallel()
	sig := SignWebhookPayload("secret", nil, time.Now())
	if sig == "" {
		t.Fatal("nil body should still produce a signature")
	}
	// Should be verifiable.
	err := webhook.ValidateSignature("stripe-v1", "secret", nil, sig)
	if err != nil {
		t.Fatalf("nil body signature validation failed: %v", err)
	}
}

func TestSignWebhookPayload_LargeBody(t *testing.T) {
	t.Parallel()
	body := make([]byte, 5*1024*1024) // 5MB.
	sig := SignWebhookPayload("secret", body, time.Now())
	if sig == "" {
		t.Fatal("large body should still produce a signature")
	}
	if !strings.Contains(sig, "v1=") {
		t.Fatal("signature format incorrect for large body")
	}
}

// -- ExtractWebhookSecret tests.

func TestExtractWebhookSecret_ValidSecret(t *testing.T) {
	t.Parallel()
	config := json.RawMessage(`{"webhook_url":"https://example.com","webhook_secret":"whsec_abc123"}`)
	secret := ExtractWebhookSecret(config)
	if secret != "whsec_abc123" {
		t.Fatalf("secret = %q, want whsec_abc123", secret)
	}
}

func TestExtractWebhookSecret_NoSecret(t *testing.T) {
	t.Parallel()
	config := json.RawMessage(`{"webhook_url":"https://example.com"}`)
	secret := ExtractWebhookSecret(config)
	if secret != "" {
		t.Fatalf("secret = %q, want empty", secret)
	}
}

func TestExtractWebhookSecret_EmptyConfig(t *testing.T) {
	t.Parallel()
	secret := ExtractWebhookSecret(nil)
	if secret != "" {
		t.Fatalf("secret = %q, want empty", secret)
	}
}

func TestExtractWebhookSecret_InvalidJSON(t *testing.T) {
	t.Parallel()
	secret := ExtractWebhookSecret(json.RawMessage(`not json`))
	if secret != "" {
		t.Fatalf("secret = %q, want empty", secret)
	}
}

// Fix 1 regression: verify SignWebhookPayload does not mutate the input body.
func TestSignWebhookPayload_DoesNotMutateBody(t *testing.T) {
	t.Parallel()
	secret := testutil.GenerateTestWebhookSecret()
	body := []byte(`{"event":"test"}`)
	original := make([]byte, len(body))
	copy(original, body)

	_ = SignWebhookPayload(secret, body, time.Now())

	if string(body) != string(original) {
		t.Fatalf("body was mutated: got %q, want %q", body, original)
	}
}

// Verify body with extra capacity is not corrupted by append.
func TestSignWebhookPayload_BodyWithExtraCapacity(t *testing.T) {
	t.Parallel()
	secret := testutil.GenerateTestWebhookSecret()
	// Create a backing array with extra capacity and a sentinel byte after the body.
	backing := make([]byte, 32, 1024)
	copy(backing, `{"event":"test"}`)
	body := backing[:16]
	sentinel := byte(0xAA)
	backing[16] = sentinel // byte just past the body length within the backing.

	_ = SignWebhookPayload(secret, body, time.Now())

	if backing[16] != sentinel {
		t.Fatalf("backing array was corrupted: byte at [16] = %x, want %x", backing[16], sentinel)
	}
}

// -- GenerateWebhookSecret tests.

func TestGenerateWebhookSecret_Format(t *testing.T) {
	t.Parallel()
	secret := GenerateWebhookSecret()
	if !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("secret should start with whsec_, got %q", secret)
	}
	if len(secret) != 6+64 { // "whsec_" + 32 bytes hex.
		t.Fatalf("secret length = %d, want 70", len(secret))
	}
}

func TestGenerateWebhookSecret_Unique(t *testing.T) {
	t.Parallel()
	s1 := GenerateWebhookSecret()
	s2 := GenerateWebhookSecret()
	if s1 == s2 {
		t.Fatal("two generated secrets should be different")
	}
}

// -- SetWebhookSecret tests.

func TestSetWebhookSecret_MergesIntoConfig(t *testing.T) {
	t.Parallel()
	config := json.RawMessage(`{"webhook_url":"https://example.com","temperature":0.2}`)
	updated := SetWebhookSecret(config, "whsec_new123")

	var cfg map[string]any
	if err := json.Unmarshal(updated, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if cfg["webhook_secret"] != "whsec_new123" {
		t.Fatalf("webhook_secret = %v", cfg["webhook_secret"])
	}
	if cfg["webhook_url"] != "https://example.com" {
		t.Fatal("existing webhook_url should be preserved")
	}
	if cfg["temperature"] != 0.2 {
		t.Fatal("existing temperature should be preserved")
	}
}

func TestSetWebhookSecret_EmptyConfig(t *testing.T) {
	t.Parallel()
	updated := SetWebhookSecret(nil, "whsec_abc")
	var cfg map[string]any
	if err := json.Unmarshal(updated, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if cfg["webhook_secret"] != "whsec_abc" {
		t.Fatalf("webhook_secret = %v", cfg["webhook_secret"])
	}
}

func TestSetWebhookSecret_OverwritesExisting(t *testing.T) {
	t.Parallel()
	config := json.RawMessage(`{"webhook_secret":"old_secret"}`)
	updated := SetWebhookSecret(config, "whsec_new")
	var cfg map[string]any
	_ = json.Unmarshal(updated, &cfg)
	if cfg["webhook_secret"] != "whsec_new" {
		t.Fatalf("webhook_secret = %v, want whsec_new", cfg["webhook_secret"])
	}
}

// -- Fuzz test.

func FuzzSignWebhookPayload(f *testing.F) {
	f.Add("whsec_test", []byte(`{"event":"test"}`))
	f.Add("", []byte{})
	f.Add("secret", []byte(`null`))

	f.Fuzz(func(t *testing.T, secret string, body []byte) {
		if len(body) > 1024*1024 {
			t.Skip()
		}
		sig := SignWebhookPayload(secret, body, time.Now())
		// Should never panic. If secret is non-empty, signature should be non-empty.
		if strings.TrimSpace(secret) != "" && sig == "" {
			t.Fatal("expected non-empty signature for non-empty secret")
		}
	})
}
