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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			assert.NotEqual(t, "",
				s)

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
				require.False(t, seen[s])

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
		require.NotEqual(t, "",
			s)
		require.False(t, seen[s])

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
		require.Len(t, code,
			8)

	}
}

// TestGenerateTestEncryptionKey_ValidForAES256 verifies the key can actually
// be used to create an AES-256 cipher.
func TestGenerateTestEncryptionKey_ValidForAES256(t *testing.T) {
	t.Parallel()
	keyHex := GenerateTestEncryptionKey()
	keyBytes, err := hex.DecodeString(keyHex)
	require.NoError(t, err)
	require.Len(t, keyBytes,
		32)

	block, err := aes.NewCipher(keyBytes)
	require.NoError(t, err)

	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)

	// Encrypt and decrypt a test message.
	plaintext := []byte("sensitive data for testing")
	nonce := make([]byte, gcm.NonceSize())
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	decrypted, err := gcm.Open(nil, nonce, ciphertext, nil)
	require.NoError(t, err)
	require.Equal(t, string(plaintext), string(decrypted))

}

// TestGenerateTestSignatureSecret_ValidForHMAC verifies the secret can be
// used for HMAC-SHA256 signing and verification.
func TestGenerateTestSignatureSecret_ValidForHMAC(t *testing.T) {
	t.Parallel()
	secret := GenerateTestSignatureSecret()
	keyBytes, err := base64.StdEncoding.DecodeString(secret)
	require.NoError(t, err)

	message := []byte("webhook payload body")
	mac := hmac.New(sha256.New, keyBytes)
	mac.Write(message)
	signature := mac.Sum(nil)

	// Verify the signature.
	mac2 := hmac.New(sha256.New, keyBytes)
	mac2.Write(message)
	require.True(t, hmac.
		Equal(signature,
			mac2.Sum(nil)))

	// Tamper with message and verify it fails.
	mac3 := hmac.New(sha256.New, keyBytes)
	mac3.Write([]byte("tampered payload"))
	require.False(t, hmac.
		Equal(signature,
			mac3.Sum(nil)),
	)

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
	require.NoError(t, err)

	parsed, err := jwt.Parse(signed, func(_ *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	require.NoError(t, err)
	require.True(t, parsed.
		Valid)

	mapClaims, ok := parsed.Claims.(jwt.MapClaims)
	require.True(t, ok)
	assert.Equal(t, "user-123",
		mapClaims["sub"])

}

// TestGenerateTestRunToken_EmptyRunID verifies tokens work with edge-case IDs.
func TestGenerateTestRunToken_EmptyRunID(t *testing.T) {
	t.Parallel()
	key := GenerateTestJWTKey()
	token := GenerateTestRunToken("", key)
	require.NotEqual(t, "",
		token)

	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	require.False(t, err !=
		nil || !parsed.
		Valid)
	assert.Equal(t, "", claims.
		Subject)

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
		require.False(t, err !=
			nil || !parsed.
			Valid)
		assert.Equal(t, id, claims.
			Subject)

	}
}

// TestGenerateTestSSEToken_EmptyScopes verifies SSE tokens work with nil/empty scopes.
func TestGenerateTestSSEToken_EmptyScopes(t *testing.T) {
	t.Parallel()
	key := GenerateTestJWTKey()

	for _, scopes := range [][]string{nil, {}, {"single"}} {
		token := GenerateTestSSEToken("proj-1", scopes, key)
		require.NotEqual(t, "",
			token)

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
	require.Error(t, err)

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
		assert.True(t, charSeen[alphabet[i]])

	}
}

// TestGenerateTestAPIKey_PrefixNotInRandomPart verifies the random hex portion
// doesn't accidentally contain the "strait_" prefix again.
func TestGenerateTestAPIKey_PrefixNotDuplicated(t *testing.T) {
	t.Parallel()
	for range 100 {
		key := GenerateTestAPIKey()
		after := key[7:]
		require.False(t, strings.Contains(after,
			"strait_"))

		// skip "strait_"

	}
}

// TestGenerateTestAPIKey_HexAfterPrefix verifies the portion after "strait_"
// is valid lowercase hex.
func TestGenerateTestAPIKey_HexAfterPrefix(t *testing.T) {
	t.Parallel()
	key := GenerateTestAPIKey()
	hexPart := key[7:]
	_, err := hex.DecodeString(hexPart)
	require.NoError(t, err)
}

// TestGenerateTestWebhookSecret_HexAfterPrefix verifies the portion after
// "whsec_" is valid lowercase hex.
func TestGenerateTestWebhookSecret_HexAfterPrefix(t *testing.T) {
	t.Parallel()
	s := GenerateTestWebhookSecret()
	hexPart := s[6:]
	_, err := hex.DecodeString(hexPart)
	require.NoError(t, err)
}

// TestGenerateTestDatabaseURL_ContainsRequiredParts verifies all URL components
// are present and well-formed.
func TestGenerateTestDatabaseURL_ContainsRequiredParts(t *testing.T) {
	t.Parallel()
	url := GenerateTestDatabaseURL()

	required := []string{"postgres://", "testuser", "testpass", "localhost", "5432", "test_", "sslmode=disable"}
	for _, part := range required {
		assert.True(t, strings.Contains(url, part))

	}
}

// TestGenerateTestDatabaseURL_RandomDBName verifies the database name changes
// between calls.
func TestGenerateTestDatabaseURL_RandomDBName(t *testing.T) {
	t.Parallel()
	urls := make(map[string]bool)
	for range 50 {
		url := GenerateTestDatabaseURL()
		require.False(t, urls[url])

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
		assert.Len(t, s, byteLen*
			2)

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
		assert.NotEqual(t, "",
			token)

	})
}
