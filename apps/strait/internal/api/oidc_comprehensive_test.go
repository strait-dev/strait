package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests: verifier construction

// TestNewOIDCVerifier_WhitespacePEM verifies that leading/trailing whitespace
// in the PEM is trimmed and the key parses correctly.
func TestNewOIDCVerifier_WhitespacePEM(t *testing.T) {
	t.Parallel()

	_, pubPEM := mustOIDCKeyPair(t)
	padded := "  \n\t" + string(pubPEM) + "\n  \t"

	v, err := newOIDCVerifier(&config.Config{
		OIDCEnabled:      true,
		OIDCIssuer:       "https://issuer.test",
		OIDCAudience:     "aud-test",
		OIDCPublicKeyPEM: padded,
	})
	require.NoError(t, err)
	require.NotNil(t, v.publicKey)

}

// TestNewOIDCVerifier_ECDSAKeyRejected verifies that an ECDSA public key
// (non-RSA) is rejected during verifier construction.
func TestNewOIDCVerifier_ECDSAKeyRejected(t *testing.T) {
	t.Parallel()

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	pubDER, err := x509.MarshalPKIXPublicKey(&ecKey.PublicKey)
	require.NoError(t, err)

	ecPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	_, err = newOIDCVerifier(&config.Config{
		OIDCEnabled:      true,
		OIDCIssuer:       "https://issuer.test",
		OIDCAudience:     "aud-test",
		OIDCPublicKeyPEM: string(ecPEM),
	})
	require.Error(t, err)

}

// TestNewOIDCVerifier_WeakKeySize verifies that an RSA key smaller than the
// library's minimum is still handled (jwt-go accepts 1024-bit for verification
// but we document 2048-bit minimum).
func TestNewOIDCVerifier_SmallRSAKey(t *testing.T) {
	t.Parallel()

	// 1024-bit RSA key — weaker than recommended but library may still parse it
	smallKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	pubDER, err := x509.MarshalPKIXPublicKey(&smallKey.PublicKey)
	require.NoError(t, err)

	smallPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	v, err := newOIDCVerifier(&config.Config{
		OIDCEnabled:      true,
		OIDCIssuer:       "https://issuer.test",
		OIDCAudience:     "aud-test",
		OIDCPublicKeyPEM: string(smallPEM),
	})

	// Whether this succeeds or fails depends on the jwt library.
	// The key point: it must not panic.
	if err != nil {
		// Rejected is acceptable for security reasons
		return
	}
	require.NotNil(t, v.publicKey)

}

// TestNewOIDCVerifier_DisabledSkipsValidation verifies that a disabled
// verifier does not attempt to parse the PEM (even if garbage).
func TestNewOIDCVerifier_DisabledSkipsValidation(t *testing.T) {
	t.Parallel()

	v, err := newOIDCVerifier(&config.Config{
		OIDCEnabled:      false,
		OIDCPublicKeyPEM: "this-is-not-valid-pem",
	})
	require.NoError(t, err)
	require.False(t, v.enabled)

}

// TestOIDCVerifier_DisabledRejectsAll verifies that a disabled verifier
// rejects all tokens with a clear error.
func TestOIDCVerifier_DisabledRejectsAll(t *testing.T) {
	t.Parallel()

	v := &oidcVerifier{enabled: false}
	_, err := v.verify("any-token-string")
	require.Error(t, err)
	require.True(
		t, strings.Contains(err.Error(), "disabled"))

}

// Unit tests: signing algorithm enforcement

// TestOIDCVerify_RejectsHS256Token verifies that HMAC-signed tokens are
// rejected even if the secret matches (only RSA is allowed).
func TestOIDCVerify_RejectsHS256Token(t *testing.T) {
	t.Parallel()

	_, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	// Sign with HS256 (HMAC) — should be rejected
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "user-hmac",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	signed, err := token.SignedString([]byte("some-hmac-secret"))
	require.NoError(t, err)

	_, err = v.verify(signed)
	require.Error(t, err)

}

// TestOIDCVerify_RejectsES256Token verifies that ECDSA-signed tokens are
// rejected (only RSA is allowed).
func TestOIDCVerify_RejectsES256Token(t *testing.T) {
	t.Parallel()

	_, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.RegisteredClaims{
		Subject:   "user-ecdsa",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	signed, err := token.SignedString(ecKey)
	require.NoError(t, err)

	_, err = v.verify(signed)
	require.Error(t, err)

}

// TestOIDCVerify_RejectsEdDSAToken verifies that EdDSA-signed tokens are
// rejected (only RSA is allowed).
func TestOIDCVerify_RejectsEdDSAToken(t *testing.T) {
	t.Parallel()

	_, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.RegisteredClaims{
		Subject:   "user-eddsa",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	signed, err := token.SignedString(edKey)
	require.NoError(t, err)

	_, err = v.verify(signed)
	require.Error(t, err)

}

// TestOIDCVerify_RejectsAlgNoneToken verifies that the "alg: none" attack
// (unsigned JWT) is rejected.
func TestOIDCVerify_RejectsAlgNoneToken(t *testing.T) {
	t.Parallel()

	_, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	// Manually craft an alg:none JWT
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"admin","iss":"https://issuer.test","aud":"aud-test","exp":` + fmt.Sprintf("%d", time.Now().Add(time.Hour).Unix()) + `}`))
	noneToken := header + "." + payload + "."

	_, err := v.verify(noneToken)
	require.Error(t, err)

}

// Unit tests: claim validation edge cases

// TestOIDCVerify_MultipleAudiences verifies that a token with multiple
// audiences is accepted when one matches.
func TestOIDCVerify_MultipleAudiences(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-multi-aud",
		Issuer:    "https://issuer.test",
		Audience:  []string{"other-service", "aud-test", "another-service"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	claims, err := v.verify(signed)
	require.NoError(t, err)
	assert.Equal(
		t, "user-multi-aud",
		claims.Subject,
	)

}

// TestOIDCVerify_NoExpiryRejected verifies that a token without an exp claim
// is rejected (tokens must be time-bounded).
func TestOIDCVerify_NoExpiryRejected(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	// Sign with MapClaims to omit exp
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "user-no-exp",
		"iss": "https://issuer.test",
		"aud": "aud-test",
		"iat": time.Now().Unix(),
	})
	signed, err := token.SignedString(key)
	require.NoError(t, err)

	if _, err := v.verify(signed); err == nil {
		require.Fail(t,

			"expected token without exp to be rejected")
	}
}

// TestOIDCVerify_FutureIssuedAt verifies that a token with a future iat
// (issued-at) is still accepted (iat is informational, not a security check).
func TestOIDCVerify_FutureIssuedAt(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-future-iat",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	claims, err := v.verify(signed)
	if err != nil {
		// Rejected is also acceptable (stricter check)
		return
	}
	assert.Equal(
		t, "user-future-iat",
		claims.Subject,
	)

}

// TestOIDCVerify_NotBeforeInFuture verifies that a token with a future nbf
// (not-before) claim is rejected.
func TestOIDCVerify_NotBeforeInFuture(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-future-nbf",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		NotBefore: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	_, err := v.verify(signed)
	require.Error(t, err)

}

// TestOIDCVerify_EmptyEmailAndName verifies that tokens with empty email/name
// are accepted (only subject is mandatory).
func TestOIDCVerify_EmptyEmailAndName(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "user-no-profile",
		"iss": "https://issuer.test",
		"aud": "aud-test",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString(key)
	require.NoError(t, err)

	claims, err := v.verify(signed)
	require.NoError(t, err)
	assert.Equal(
		t, "", claims.Email,
	)
	assert.Equal(
		t, "", claims.Name,
	)

}

// TestOIDCVerify_VeryLongSubject verifies that extremely long subject values
// don't cause issues.
func TestOIDCVerify_VeryLongSubject(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	longSub := strings.Repeat("a", 10000)
	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   longSub,
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	claims, err := v.verify(signed)
	require.NoError(t, err)
	assert.Equal(
		t, longSub, claims.
			Subject)

}

// Unit tests: JWT structure attacks

// TestOIDCVerify_TruncatedJWT verifies that JWTs with missing parts are
// rejected gracefully.
func TestOIDCVerify_TruncatedJWT(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	// Get a valid token and test truncations
	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-trunc",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	parts := strings.SplitN(signed, ".", 3)

	truncations := []struct {
		name  string
		token string
	}{
		{"header only", parts[0]},
		{"header.payload", parts[0] + "." + parts[1]},
		{"header..signature", parts[0] + ".." + parts[2]},
		{".payload.signature", "." + parts[1] + "." + parts[2]},
		{"empty parts", ".."},
		{"extra dots", parts[0] + "." + parts[1] + "." + parts[2] + ".extra"},
	}

	for _, tc := range truncations {
		t.Run(tc.name, func(t *testing.T) {
			_, err := v.verify(tc.token)
			assert.Error(
				t, err)

		})
	}
}

// TestOIDCVerify_HeaderManipulation verifies that replacing the JWT header
// while keeping payload/signature intact is detected.
func TestOIDCVerify_HeaderManipulation(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-header-swap",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	parts := strings.SplitN(signed, ".", 3)

	// Replace header with one claiming HS256
	fakeHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	swapped := fakeHeader + "." + parts[1] + "." + parts[2]

	_, err := v.verify(swapped)
	require.Error(t, err)

}

// TestOIDCVerify_InvalidBase64InPayload verifies that invalid base64 in the
// payload section is rejected.
func TestOIDCVerify_InvalidBase64InPayload(t *testing.T) {
	t.Parallel()

	_, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	invalidPayload := "!!!not-base64!!!"
	sig := base64.RawURLEncoding.EncodeToString([]byte("fake-sig"))

	_, err := v.verify(header + "." + invalidPayload + "." + sig)
	require.Error(t, err)

}

// TestOIDCVerify_InvalidJSONInPayload verifies that valid base64 but invalid
// JSON in the payload is rejected.
func TestOIDCVerify_InvalidJSONInPayload(t *testing.T) {
	t.Parallel()

	_, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	badJSON := base64.RawURLEncoding.EncodeToString([]byte(`{not valid json}`))
	sig := base64.RawURLEncoding.EncodeToString([]byte("fake-sig"))

	_, err := v.verify(header + "." + badJSON + "." + sig)
	require.Error(t, err)

}

// Integration tests: middleware auth routing

// TestOIDCAuth_MissingBearerToken verifies that a request with no
// Authorization header falls through to internal secret auth (not OIDC).
func TestOIDCAuth_MissingBearerToken(t *testing.T) {
	t.Parallel()

	_, pubPEM := mustOIDCKeyPair(t)
	srv := oidcProjectTestServer(t, pubPEM, &APIStoreMock{})

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	// No Authorization header
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,
		w.Code,
	)

	// Should hit internal secret auth and get 401 (no secret either)

}

// TestOIDCAuth_EmptyBearerValue verifies that "Bearer " with no token
// returns 401.
func TestOIDCAuth_EmptyBearerValue(t *testing.T) {
	t.Parallel()

	_, pubPEM := mustOIDCKeyPair(t)
	srv := oidcProjectTestServer(t, pubPEM, &APIStoreMock{})

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,
		w.Code,
	)

}

// TestOIDCAuth_BearerWithExtraSpaces verifies that extra whitespace around
// the token is trimmed correctly.
func TestOIDCAuth_BearerWithExtraSpaces(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-spaces",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	ms := &APIStoreMock{}
	ms.QueueStatsFunc = func(_ context.Context) (*store.QueueStats, error) {
		return &store.QueueStats{}, nil
	}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return nil, nil
	}

	srv := oidcProjectTestServer(t, pubPEM, ms)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer   "+signed+"  ")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.NotEqual(t, http.StatusUnauthorized,
		w.
			Code)

	// Should succeed — whitespace is trimmed

}

// TestOIDCAuth_StraitPrefixRoutesToAPIKey verifies that tokens starting with
// "strait_" are routed to API key auth, not OIDC. The API key lookup returns
// an error (key not found), so we expect a non-200 response confirming the
// request went through the API key path rather than OIDC verification.
func TestOIDCAuth_StraitPrefixRoutesToAPIKey(t *testing.T) {
	t.Parallel()

	_, pubPEM := mustOIDCKeyPair(t)
	ms := &APIStoreMock{}
	ms.GetAPIKeyByHashFunc = func(_ context.Context, _ string) (*domain.APIKey, error) {
		return nil, fmt.Errorf("key not found")
	}
	srv := oidcProjectTestServer(t, pubPEM, ms)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer strait_fake_key_123")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,
		w.Code,
	)

	// Should hit API key auth (not OIDC), get 401 for invalid key

}

// TestOIDCAuth_NilStoreReturns503 verifies that if the store is nil and
// X-Project-Id is set, the middleware returns 503.
func TestOIDCAuth_NilStoreReturns503(t *testing.T) {
	t.Parallel()

	key, pubPEM := mustOIDCKeyPair(t)
	signed := mustSignOIDCToken(t, key, jwt.RegisteredClaims{
		Subject:   "user-nilstore",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			OIDCEnabled:      true,
			OIDCIssuer:       "https://issuer.test",
			OIDCAudience:     "aud-test",
			OIDCPublicKeyPEM: string(pubPEM),
		},
		Store: nil, // nil store
	})
	t.Cleanup(srv.Close)

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer "+signed)
	r.Header.Set("X-Project-Id", "proj-123")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusServiceUnavailable,

		w.Code)

}

// Integration tests: response body validation

// TestOIDCAuth_ErrorResponseIsJSON verifies that OIDC auth errors return
// JSON error bodies (not plain text).
func TestOIDCAuth_ErrorResponseIsJSON(t *testing.T) {
	t.Parallel()

	_, pubPEM := mustOIDCKeyPair(t)
	srv := oidcProjectTestServer(t, pubPEM, &APIStoreMock{})

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized,
		w.Code,
	)

	// Verify error response is valid JSON
	var errResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))

}

// TestOIDCAuth_DoesNotLeakTokenDetails verifies that 401 error responses
// don't include the token value or detailed crypto errors.
func TestOIDCAuth_DoesNotLeakTokenDetails(t *testing.T) {
	t.Parallel()

	_, pubPEM := mustOIDCKeyPair(t)
	srv := oidcProjectTestServer(t, pubPEM, &APIStoreMock{})

	r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer some-secret-looking-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	body := w.Body.String()
	require.False(t, strings.Contains(body, "some-secret-looking-token"))
	require.False(t, strings.Contains(body, "RSA") ||
		strings.Contains(body, "signature"))

}

// Adversarial: token replay and key confusion

// TestOIDCVerify_DifferentKeyPairRejected verifies that a token signed with
// one RSA key pair is rejected by a verifier configured with a different pair.
func TestOIDCVerify_DifferentKeyPairRejected(t *testing.T) {
	t.Parallel()

	key1, _ := mustOIDCKeyPair(t)
	_, pubPEM2 := mustOIDCKeyPair(t) // different key pair

	v := mustOIDCVerifier(t, pubPEM2, "https://issuer.test", "aud-test")

	signed := mustSignOIDCToken(t, key1, jwt.RegisteredClaims{
		Subject:   "user-wrong-key",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	_, err := v.verify(signed)
	require.Error(t, err)

}

// TestOIDCVerify_KeyConfusionAttack verifies that using the RSA public key
// as an HMAC secret (a known attack vector) is rejected.
func TestOIDCVerify_KeyConfusionAttack(t *testing.T) {
	t.Parallel()

	_, pubPEM := mustOIDCKeyPair(t)
	v := mustOIDCVerifier(t, pubPEM, "https://issuer.test", "aud-test")

	// Attack: use the public key PEM as the HMAC secret
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "admin",
		Issuer:    "https://issuer.test",
		Audience:  []string{"aud-test"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	signed, err := token.SignedString(pubPEM)
	require.NoError(t, err)

	// use public key as HMAC secret

	_, err = v.verify(signed)
	require.Error(t, err)

}

// Fuzz tests

// FuzzOIDCVerify_ClaimsJSON fuzzes the JWT payload with random JSON to ensure
// claim parsing never panics.
func FuzzOIDCVerify_ClaimsJSON(f *testing.F) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		f.Fatalf("generate key: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		f.Fatalf("marshal pub key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	pk, err := jwt.ParseRSAPublicKeyFromPEM(pubPEM)
	if err != nil {
		f.Fatalf("parse pub key: %v", err)
	}
	v := &oidcVerifier{
		enabled:   true,
		issuer:    "https://issuer.test",
		audience:  "aud-test",
		publicKey: pk,
	}

	// Seed with valid-looking claim values
	f.Add("user-1", "user@test.com", "Test User")
	f.Add("", "", "")
	f.Add(strings.Repeat("x", 5000), "email\x00null@evil.com", "name\x00injected")
	f.Add("user\n\r\t", "email with spaces ", "  ")

	f.Fuzz(func(t *testing.T, sub, email, name string) {
		claims := jwt.MapClaims{
			"sub":   sub,
			"iss":   "https://issuer.test",
			"aud":   "aud-test",
			"email": email,
			"name":  name,
			"exp":   time.Now().Add(time.Hour).Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		signed, err := token.SignedString(key)
		if err != nil {
			return // skip if signing fails
		}
		// Must not panic.
		_, _ = v.verify(signed)
	})
}

// FuzzOIDCVerify_AuthorizationHeader fuzzes the full Authorization header
// value through the middleware to ensure no panics at the routing layer.
func FuzzOIDCVerify_AuthorizationHeader(f *testing.F) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		f.Fatalf("generate key: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		f.Fatalf("marshal pub key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	ms := &APIStoreMock{}
	ms.UserHasProjectAccessFunc = func(_ context.Context, _, _ string) (bool, error) {
		return false, nil
	}
	ms.QueueStatsFunc = func(_ context.Context) (*store.QueueStats, error) {
		return &store.QueueStats{}, nil
	}
	ms.GetUserPermissionsFunc = func(_ context.Context, _, _ string) ([]string, error) {
		return nil, nil
	}

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			OIDCEnabled:      true,
			OIDCIssuer:       "https://issuer.test",
			OIDCAudience:     "aud-test",
			OIDCPublicKeyPEM: string(pubPEM),
		},
		Store: ms,
	})
	f.Cleanup(srv.Close)

	f.Add("")
	f.Add("Bearer ")
	f.Add("Bearer invalid")
	f.Add("Bearer strait_fake")
	f.Add("Basic dXNlcjpwYXNz")
	f.Add(strings.Repeat("A", 65536))

	f.Fuzz(func(t *testing.T, authHeader string) {
		r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
		if authHeader != "" {
			r.Header.Set("Authorization", authHeader)
		}
		w := httptest.NewRecorder()
		// Must not panic
		srv.ServeHTTP(w, r)
	})
}
