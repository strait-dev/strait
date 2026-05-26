package testutil

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateTestSecret_Length(t *testing.T) {
	t.Parallel()
	for _, byteLen := range []int{8, 16, 32, 64} {
		s := GenerateTestSecret(byteLen)
		if len(s) != byteLen*2 {
			t.Errorf("GenerateTestSecret(%d) len = %d, want %d", byteLen, len(s), byteLen*2)
		}
	}
}

func TestGenerateTestSecret_Unique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for range 100 {
		s := GenerateTestSecret(16)
		if seen[s] {
			t.Fatalf("duplicate secret generated: %s", s)
		}
		seen[s] = true
	}
}

func TestGenerateTestSecret_ValidHex(t *testing.T) {
	t.Parallel()
	s := GenerateTestSecret(32)
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Fatalf("non-hex character %q in secret %q", c, s)
		}
	}
}

func TestGenerateTestSecret_PanicsOnZero(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for byteLen=0")
		}
	}()
	GenerateTestSecret(0)
}

func TestGenerateTestSecret_PanicsOnNegative(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for byteLen=-1")
		}
	}()
	GenerateTestSecret(-1)
}

func TestGenerateTestWebhookSecret_Format(t *testing.T) {
	t.Parallel()
	s := GenerateTestWebhookSecret()
	if !strings.HasPrefix(s, "whsec_") {
		t.Errorf("should start with whsec_, got %q", s)
	}
	if len(s) != 6+32 { // "whsec_" + 16 bytes hex
		t.Errorf("len = %d, want %d", len(s), 38)
	}
}

func TestGenerateTestWebhookSecret_Unique(t *testing.T) {
	t.Parallel()
	a := GenerateTestWebhookSecret()
	b := GenerateTestWebhookSecret()
	if a == b {
		t.Error("two calls should produce different secrets")
	}
}

func TestGenerateTestJWTKey_Length(t *testing.T) {
	t.Parallel()
	s := GenerateTestJWTKey()
	if len(s) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("len = %d, want 64", len(s))
	}
}

func TestGenerateTestJWTKey_ValidForHMAC(t *testing.T) {
	t.Parallel()
	key := GenerateTestJWTKey()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: "test"})
	signed, err := token.SignedString([]byte(key))
	if err != nil {
		t.Fatalf("JWT signing failed: %v", err)
	}
	if signed == "" {
		t.Fatal("signed token is empty")
	}

	// Verify it can be parsed back.
	parsed, err := jwt.Parse(signed, func(_ *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("JWT verification failed: %v", err)
	}
}

func TestGenerateTestInternalSecret_MinLength(t *testing.T) {
	t.Parallel()
	s := GenerateTestInternalSecret()
	if len(s) < 16 {
		t.Errorf("internal secret length %d < 16 (minimum required by config)", len(s))
	}
}

func TestGenerateTestAPIKey_Format(t *testing.T) {
	t.Parallel()
	s := GenerateTestAPIKey()
	if !strings.HasPrefix(s, "strait_") {
		t.Errorf("should start with strait_, got %q", s)
	}
	if len(s) != 7+64 { // "strait_" + 32 bytes hex
		t.Errorf("len = %d, want %d", len(s), 71)
	}
}

func TestGenerateTestAPIKey_Unique(t *testing.T) {
	t.Parallel()
	a := GenerateTestAPIKey()
	b := GenerateTestAPIKey()
	if a == b {
		t.Error("two calls should produce different keys")
	}
}

func TestGenerateTestEncryptionKey_Length(t *testing.T) {
	t.Parallel()
	s := GenerateTestEncryptionKey()
	if len(s) != 64 { // 32 bytes for AES-256
		t.Errorf("len = %d, want 64", len(s))
	}
}

func TestGenerateTestDeviceCode_Length(t *testing.T) {
	t.Parallel()
	s := GenerateTestDeviceCode()
	if len(s) != 64 { // 32 bytes hex
		t.Errorf("len = %d, want 64", len(s))
	}
}

func TestGenerateTestDeviceCode_ValidHex(t *testing.T) {
	t.Parallel()
	s := GenerateTestDeviceCode()
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Fatalf("non-hex character %q in device code", c)
		}
	}
}

func TestGenerateTestUserCode_Length(t *testing.T) {
	t.Parallel()
	s := GenerateTestUserCode()
	if len(s) != 8 {
		t.Errorf("len = %d, want 8", len(s))
	}
}

func TestGenerateTestUserCode_ValidAlphabet(t *testing.T) {
	t.Parallel()
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	for range 50 {
		code := GenerateTestUserCode()
		for _, c := range code {
			if !strings.ContainsRune(alphabet, c) {
				t.Fatalf("invalid character %q in user code %q (not in alphabet)", c, code)
			}
		}
	}
}

func TestGenerateTestUserCode_NoConfusingChars(t *testing.T) {
	t.Parallel()
	for range 100 {
		code := GenerateTestUserCode()
		for _, c := range "01IO" {
			if strings.ContainsRune(code, c) {
				t.Fatalf("user code %q contains confusing character %q", code, c)
			}
		}
	}
}

func TestGenerateTestUserCode_Unique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for range 100 {
		code := GenerateTestUserCode()
		if seen[code] {
			t.Fatalf("duplicate user code: %s", code)
		}
		seen[code] = true
	}
}

func TestGenerateTestSignatureSecret_ValidBase64(t *testing.T) {
	t.Parallel()
	s := GenerateTestSignatureSecret()
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("not valid base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("decoded length = %d, want 32", len(decoded))
	}
}

func TestGenerateTestSignatureSecret_Unique(t *testing.T) {
	t.Parallel()
	a := GenerateTestSignatureSecret()
	b := GenerateTestSignatureSecret()
	if a == b {
		t.Error("two calls should produce different secrets")
	}
}

func TestGenerateTestRunToken_Valid(t *testing.T) {
	t.Parallel()
	key := GenerateTestJWTKey()
	token := GenerateTestRunToken("run-123", key)

	if token == "" {
		t.Fatal("token is empty")
	}

	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("token verification failed: %v", err)
	}
	if claims.Subject != "run-123" {
		t.Errorf("subject = %q, want %q", claims.Subject, "run-123")
	}
}

func TestGenerateTestRunToken_WrongKey_Fails(t *testing.T) {
	t.Parallel()
	key1 := GenerateTestJWTKey()
	key2 := GenerateTestJWTKey()
	token := GenerateTestRunToken("run-123", key1)

	_, err := jwt.Parse(token, func(_ *jwt.Token) (any, error) {
		return []byte(key2), nil
	})
	if err == nil {
		t.Fatal("expected verification to fail with wrong key")
	}
}

func TestGenerateTestSSEToken_Valid(t *testing.T) {
	t.Parallel()
	key := GenerateTestJWTKey()
	token := GenerateTestSSEToken("proj-1", []string{"runs:read", "jobs:read"}, key)

	if token == "" {
		t.Fatal("token is empty")
	}

	type sseClaims struct {
		jwt.RegisteredClaims
		ProjectID string   `json:"pid"`
		Scopes    []string `json:"scp,omitempty"`
	}
	claims := &sseClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("token verification failed: %v", err)
	}
	if claims.Issuer != "strait:sse" {
		t.Errorf("issuer = %q, want %q", claims.Issuer, "strait:sse")
	}
	if claims.ProjectID != "proj-1" {
		t.Errorf("project_id = %q, want %q", claims.ProjectID, "proj-1")
	}
	if len(claims.Scopes) != 2 {
		t.Errorf("scopes len = %d, want 2", len(claims.Scopes))
	}
}

func TestGenerateTestSSEToken_Expires(t *testing.T) {
	t.Parallel()
	key := GenerateTestJWTKey()
	token := GenerateTestSSEToken("proj-1", nil, key)

	claims := &jwt.RegisteredClaims{}
	parsed, _ := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	if !parsed.Valid {
		t.Fatal("token should be valid")
	}
	if claims.ExpiresAt == nil {
		t.Fatal("token should have expiry")
	}
}

func TestGenerateTestClaimToken_Length(t *testing.T) {
	t.Parallel()
	s := GenerateTestClaimToken()
	if len(s) != 64 { // 32 bytes hex
		t.Errorf("len = %d, want 64", len(s))
	}
}

func TestGenerateTestDatabaseURL_Format(t *testing.T) {
	t.Parallel()
	url := GenerateTestDatabaseURL()
	if !strings.HasPrefix(url, "postgres://") {
		t.Errorf("should start with postgres://, got %q", url)
	}
	if !strings.Contains(url, "sslmode=disable") {
		t.Error("test DB URL should contain sslmode=disable")
	}
	if !strings.Contains(url, "test_") {
		t.Error("test DB URL should contain random database name with test_ prefix")
	}
}

func TestGenerateTestDatabaseURL_Unique(t *testing.T) {
	t.Parallel()
	a := GenerateTestDatabaseURL()
	b := GenerateTestDatabaseURL()
	if a == b {
		t.Error("two calls should produce different URLs")
	}
}

func TestGenerateTestRedisURL_Format(t *testing.T) {
	t.Parallel()
	url := GenerateTestRedisURL()
	if !strings.HasPrefix(url, "redis://") {
		t.Errorf("should start with redis://, got %q", url)
	}
}
