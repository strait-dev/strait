package testutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sourcegraph/conc"
)

// TestAllGenerators_NeverEmpty verifies no generator returns an empty string.
func TestAllGenerators_NeverEmpty(t *testing.T) {
	t.Parallel()
	generators := map[string]func() string{
		"Secret16":        func() string { return GenerateTestSecret(16) },
		"WebhookSecret":   GenerateTestWebhookSecret,
		"JWTKey":          GenerateTestJWTKey,
		"InternalSecret":  GenerateTestInternalSecret,
		"APIKey":          GenerateTestAPIKey,
		"EncryptionKey":   GenerateTestEncryptionKey,
		"DeviceCode":      GenerateTestDeviceCode,
		"UserCode":        GenerateTestUserCode,
		"SignatureSecret": GenerateTestSignatureSecret,
		"ClaimToken":      GenerateTestClaimToken,
		"KeyHash":         GenerateTestKeyHash,
		"DatabaseURL":     GenerateTestDatabaseURL,
		"RedisURL":        GenerateTestRedisURL,
	}
	for name, gen := range generators {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := gen()
			if s == "" {
				t.Errorf("%s returned empty string", name)
			}
		})
	}
}

// TestAllHexGenerators_NoCollisionIn1000 checks that hex-based generators
// don't produce duplicates in 1000 calls (probabilistically impossible for
// 128+ bits of entropy).
func TestAllHexGenerators_NoCollisionIn1000(t *testing.T) {
	t.Parallel()
	generators := map[string]func() string{
		"Secret32":       func() string { return GenerateTestSecret(32) },
		"JWTKey":         GenerateTestJWTKey,
		"InternalSecret": GenerateTestInternalSecret,
		"APIKey":         GenerateTestAPIKey,
		"EncryptionKey":  GenerateTestEncryptionKey,
		"DeviceCode":     GenerateTestDeviceCode,
		"ClaimToken":     GenerateTestClaimToken,
		"KeyHash":        GenerateTestKeyHash,
	}
	for name, gen := range generators {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			seen := make(map[string]bool, 1000)
			for range 1000 {
				s := gen()
				if seen[s] {
					t.Fatalf("%s produced duplicate after <1000 calls: %s", name, s)
				}
				seen[s] = true
			}
		})
	}
}

// TestGenerateTestSecret_ConcurrentSafe verifies no races when called from
// multiple goroutines simultaneously.
func TestGenerateTestSecret_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	var wg conc.WaitGroup
	results := make([]string, 100)
	for i := range 100 {
		wg.Go(func() {
			results[i] = GenerateTestSecret(32)
		})
	}
	wg.Wait()

	seen := make(map[string]bool)
	for _, s := range results {
		if s == "" {
			t.Fatal("empty result from concurrent call")
		}
		if seen[s] {
			t.Fatal("duplicate from concurrent calls")
		}
		seen[s] = true
	}
}

// TestGenerateTestUserCode_ConcurrentSafe verifies user code generation is
// safe under concurrent access.
func TestGenerateTestUserCode_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	var wg conc.WaitGroup
	results := make([]string, 100)
	for i := range 100 {
		wg.Go(func() {
			results[i] = GenerateTestUserCode()
		})
	}
	wg.Wait()

	for _, code := range results {
		if len(code) != 8 {
			t.Fatalf("wrong length %d from concurrent call", len(code))
		}
	}
}

// TestGenerateTestEncryptionKey_ValidForAES256 verifies the key can actually
// be used to create an AES-256 cipher.
func TestGenerateTestEncryptionKey_ValidForAES256(t *testing.T) {
	t.Parallel()
	keyHex := GenerateTestEncryptionKey()
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		t.Fatalf("key is not valid hex: %v", err)
	}
	if len(keyBytes) != 32 {
		t.Fatalf("key bytes length = %d, want 32 for AES-256", len(keyBytes))
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		t.Fatalf("AES cipher creation failed: %v", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("GCM creation failed: %v", err)
	}

	// Encrypt and decrypt a test message.
	plaintext := []byte("sensitive data for testing")
	nonce := make([]byte, gcm.NonceSize())
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	decrypted, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

// TestGenerateTestSignatureSecret_ValidForHMAC verifies the secret can be
// used for HMAC-SHA256 signing and verification.
func TestGenerateTestSignatureSecret_ValidForHMAC(t *testing.T) {
	t.Parallel()
	secret := GenerateTestSignatureSecret()
	keyBytes, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		t.Fatalf("secret is not valid base64: %v", err)
	}

	message := []byte("webhook payload body")
	mac := hmac.New(sha256.New, keyBytes)
	mac.Write(message)
	signature := mac.Sum(nil)

	// Verify the signature.
	mac2 := hmac.New(sha256.New, keyBytes)
	mac2.Write(message)
	if !hmac.Equal(signature, mac2.Sum(nil)) {
		t.Fatal("HMAC verification failed")
	}

	// Tamper with message and verify it fails.
	mac3 := hmac.New(sha256.New, keyBytes)
	mac3.Write([]byte("tampered payload"))
	if hmac.Equal(signature, mac3.Sum(nil)) {
		t.Fatal("HMAC should fail for tampered message")
	}
}

// TestGenerateTestJWTKey_SignVerifyRoundtrip verifies the key works for
// a full JWT sign -> parse -> verify cycle with all claim types.
func TestGenerateTestJWTKey_SignVerifyRoundtrip(t *testing.T) {
	t.Parallel()
	key := GenerateTestJWTKey()

	claims := jwt.MapClaims{
		"sub":  "user-123",
		"aud":  "api.strait.dev",
		"iss":  "strait",
		"role": "admin",
		"tags": []string{"prod", "us-east"},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(key))
	if err != nil {
		t.Fatalf("signing failed: %v", err)
	}

	parsed, err := jwt.Parse(signed, func(_ *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	if err != nil {
		t.Fatalf("parsing failed: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("token should be valid")
	}

	mapClaims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("claims should be MapClaims")
	}
	if mapClaims["sub"] != "user-123" {
		t.Errorf("sub = %v, want user-123", mapClaims["sub"])
	}
}

// TestGenerateTestRunToken_EmptyRunID verifies tokens work with edge-case IDs.
func TestGenerateTestRunToken_EmptyRunID(t *testing.T) {
	t.Parallel()
	key := GenerateTestJWTKey()
	token := GenerateTestRunToken("", key)
	if token == "" {
		t.Fatal("should generate token even with empty run ID")
	}

	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("token with empty run ID should still be valid: %v", err)
	}
	if claims.Subject != "" {
		t.Errorf("subject should be empty, got %q", claims.Subject)
	}
}

// TestGenerateTestRunToken_SpecialCharsInRunID verifies tokens handle special
// characters in the run ID without corruption.
func TestGenerateTestRunToken_SpecialCharsInRunID(t *testing.T) {
	t.Parallel()
	key := GenerateTestJWTKey()
	specialIDs := []string{
		"run-with-dashes",
		"run/with/slashes",
		"run with spaces",
		"run\nwith\nnewlines",
		"run\x00with\x00nulls",
		strings.Repeat("x", 1000),
		"emoji-\U0001F680-run",
	}

	for _, id := range specialIDs {
		token := GenerateTestRunToken(id, key)
		claims := &jwt.RegisteredClaims{}
		parsed, err := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
			return []byte(key), nil
		})
		if err != nil || !parsed.Valid {
			t.Fatalf("token with run ID %q failed: %v", id[:min(len(id), 20)], err)
		}
		if claims.Subject != id {
			t.Errorf("subject roundtrip failed for ID %q", id[:min(len(id), 20)])
		}
	}
}

// TestGenerateTestSSEToken_EmptyScopes verifies SSE tokens work with nil/empty scopes.
func TestGenerateTestSSEToken_EmptyScopes(t *testing.T) {
	t.Parallel()
	key := GenerateTestJWTKey()

	for _, scopes := range [][]string{nil, {}, {"single"}} {
		token := GenerateTestSSEToken("proj-1", scopes, key)
		if token == "" {
			t.Fatal("should generate token with empty scopes")
		}
	}
}

// TestGenerateTestSSEToken_CrossKeyRejection verifies that SSE tokens signed
// with one key cannot be verified with a different key.
func TestGenerateTestSSEToken_CrossKeyRejection(t *testing.T) {
	t.Parallel()
	key1 := GenerateTestJWTKey()
	key2 := GenerateTestJWTKey()
	token := GenerateTestSSEToken("proj-1", []string{"runs:read"}, key1)

	_, err := jwt.Parse(token, func(_ *jwt.Token) (any, error) {
		return []byte(key2), nil
	})
	if err == nil {
		t.Fatal("SSE token should not verify with wrong key")
	}
}

// TestGenerateTestUserCode_Distribution verifies that all alphabet characters
// appear at least once in a large sample, confirming no systematic bias.
func TestGenerateTestUserCode_Distribution(t *testing.T) {
	t.Parallel()
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	charSeen := make(map[byte]bool)

	for range 500 {
		code := GenerateTestUserCode()
		for i := range len(code) {
			charSeen[code[i]] = true
		}
	}

	for i := range len(alphabet) {
		if !charSeen[alphabet[i]] {
			t.Errorf("character %q never appeared in 500 user codes", alphabet[i])
		}
	}
}

// TestGenerateTestAPIKey_PrefixNotInRandomPart verifies the random hex portion
// doesn't accidentally contain the "strait_" prefix again.
func TestGenerateTestAPIKey_PrefixNotDuplicated(t *testing.T) {
	t.Parallel()
	for range 100 {
		key := GenerateTestAPIKey()
		after := key[7:] // skip "strait_"
		if strings.Contains(after, "strait_") {
			t.Fatalf("random part contains prefix: %s", key)
		}
	}
}

// TestGenerateTestAPIKey_HexAfterPrefix verifies the portion after "strait_"
// is valid lowercase hex.
func TestGenerateTestAPIKey_HexAfterPrefix(t *testing.T) {
	t.Parallel()
	key := GenerateTestAPIKey()
	hexPart := key[7:]
	if _, err := hex.DecodeString(hexPart); err != nil {
		t.Fatalf("portion after strait_ is not valid hex: %v", err)
	}
}

// TestGenerateTestWebhookSecret_HexAfterPrefix verifies the portion after
// "whsec_" is valid lowercase hex.
func TestGenerateTestWebhookSecret_HexAfterPrefix(t *testing.T) {
	t.Parallel()
	s := GenerateTestWebhookSecret()
	hexPart := s[6:]
	if _, err := hex.DecodeString(hexPart); err != nil {
		t.Fatalf("portion after whsec_ is not valid hex: %v", err)
	}
}

// TestGenerateTestDatabaseURL_ContainsRequiredParts verifies all URL components
// are present and well-formed.
func TestGenerateTestDatabaseURL_ContainsRequiredParts(t *testing.T) {
	t.Parallel()
	url := GenerateTestDatabaseURL()

	required := []string{"postgres://", "testuser", "testpass", "localhost", "5432", "test_", "sslmode=disable"}
	for _, part := range required {
		if !strings.Contains(url, part) {
			t.Errorf("URL %q missing required part %q", url, part)
		}
	}
}

// TestGenerateTestDatabaseURL_RandomDBName verifies the database name changes
// between calls.
func TestGenerateTestDatabaseURL_RandomDBName(t *testing.T) {
	t.Parallel()
	urls := make(map[string]bool)
	for range 50 {
		url := GenerateTestDatabaseURL()
		if urls[url] {
			t.Fatalf("duplicate URL in 50 calls: %s", url)
		}
		urls[url] = true
	}
}

// FuzzGenerateTestSecret_NoPanic verifies GenerateTestSecret never panics
// for any positive byte length.
func FuzzGenerateTestSecret_NoPanic(f *testing.F) {
	f.Add(1)
	f.Add(16)
	f.Add(32)
	f.Add(64)
	f.Add(256)
	f.Add(1024)

	f.Fuzz(func(t *testing.T, byteLen int) {
		if byteLen <= 0 || byteLen > 4096 {
			return // skip invalid/unreasonable inputs
		}
		s := GenerateTestSecret(byteLen)
		if len(s) != byteLen*2 {
			t.Errorf("len = %d, want %d", len(s), byteLen*2)
		}
	})
}

// FuzzGenerateTestRunToken_NoPanic verifies run token generation never panics
// regardless of input.
func FuzzGenerateTestRunToken_NoPanic(f *testing.F) {
	f.Add("run-1", "0123456789abcdef0123456789abcdef")
	f.Add("", "key")
	f.Add(strings.Repeat("x", 1000), "k")
	f.Add("run\x00null", "key-with-special\x00chars")

	f.Fuzz(func(t *testing.T, runID, key string) {
		if key == "" {
			return // empty key causes jwt library error
		}
		token := GenerateTestRunToken(runID, key)
		if token == "" {
			t.Error("empty token")
		}
	})
}
