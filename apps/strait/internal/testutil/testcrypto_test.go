package testutil

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTestSecret_Length(t *testing.T) {
	t.Parallel()
	for _, byteLen := range []int{8, 16, 32, 64} {
		s := GenerateTestSecret(byteLen)
		assert.Len(t, s, byteLen*
			2)

	}
}

func TestGenerateTestSecret_Unique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for range 100 {
		s := GenerateTestSecret(16)
		require.False(t, seen[s])

		seen[s] = true
	}
}

func TestGenerateTestSecret_ValidHex(t *testing.T) {
	t.Parallel()
	s := GenerateTestSecret(32)
	for _, c := range s {
		require.False(t, (c < '0' ||
			c > '9') &&
			(c < 'a' ||
				c > 'f'))

	}
}

func TestGenerateTestSecret_PanicsOnZero(t *testing.T) {
	t.Parallel()
	defer func() {
		require.NotNil(t, recover())
	}()
	GenerateTestSecret(0)
}

func TestGenerateTestSecret_PanicsOnNegative(t *testing.T) {
	t.Parallel()
	defer func() {
		require.NotNil(t, recover())
	}()
	GenerateTestSecret(-1)
}

func TestGenerateTestWebhookSecret_Format(t *testing.T) {
	t.Parallel()
	s := GenerateTestWebhookSecret()
	assert.True(t, strings.HasPrefix(s, "whsec_"))
	assert.Len(t, s, 6+32)

	// "whsec_" + 16 bytes hex

}

func TestGenerateTestWebhookSecret_Unique(t *testing.T) {
	t.Parallel()
	a := GenerateTestWebhookSecret()
	b := GenerateTestWebhookSecret()
	assert.NotEqual(t, b, a)

}

func TestGenerateTestJWTKey_Length(t *testing.T) {
	t.Parallel()
	s := GenerateTestJWTKey()
	assert.Len(t, s, 64)

	// 32 bytes = 64 hex chars

}

func TestGenerateTestJWTKey_ValidForHMAC(t *testing.T) {
	t.Parallel()
	key := GenerateTestJWTKey()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: "test"})
	signed, err := token.SignedString([]byte(key))
	require.NoError(t, err)
	require.NotEqual(t, "", signed)

	// Verify it can be parsed back.
	parsed, err := jwt.Parse(signed, func(_ *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	require.False(t, err != nil ||
		!parsed.
			Valid)

}

func TestGenerateTestInternalSecret_MinLength(t *testing.T) {
	t.Parallel()
	s := GenerateTestInternalSecret()
	assert.GreaterOrEqual(t, len(s), 16)

}

func TestGenerateTestAPIKey_Format(t *testing.T) {
	t.Parallel()
	s := GenerateTestAPIKey()
	assert.True(t, strings.HasPrefix(s, "strait_"))
	assert.Len(t, s, 7+64)

	// "strait_" + 32 bytes hex

}

func TestGenerateTestAPIKey_Unique(t *testing.T) {
	t.Parallel()
	a := GenerateTestAPIKey()
	b := GenerateTestAPIKey()
	assert.NotEqual(t, b, a)

}

func TestGenerateTestEncryptionKey_Length(t *testing.T) {
	t.Parallel()
	s := GenerateTestEncryptionKey()
	assert.Len(t, s, 64)

	// 32 bytes for AES-256

}

func TestGenerateTestDeviceCode_Length(t *testing.T) {
	t.Parallel()
	s := GenerateTestDeviceCode()
	assert.Len(t, s, 64)

	// 32 bytes hex

}

func TestGenerateTestDeviceCode_ValidHex(t *testing.T) {
	t.Parallel()
	s := GenerateTestDeviceCode()
	for _, c := range s {
		require.False(t, (c < '0' ||
			c > '9') &&
			(c < 'a' ||
				c > 'f'))

	}
}

func TestGenerateTestUserCode_Length(t *testing.T) {
	t.Parallel()
	s := GenerateTestUserCode()
	assert.Len(t, s, 8)

}

func TestGenerateTestUserCode_ValidAlphabet(t *testing.T) {
	t.Parallel()
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	for range 50 {
		code := GenerateTestUserCode()
		for _, c := range code {
			require.True(t, strings.ContainsRune(
				alphabet, c,
			),
			)

		}
	}
}

func TestGenerateTestUserCode_NoConfusingChars(t *testing.T) {
	t.Parallel()
	for range 100 {
		code := GenerateTestUserCode()
		for _, c := range "01IO" {
			require.False(t, strings.ContainsRune(code, c))

		}
	}
}

func TestGenerateTestUserCode_Unique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for range 100 {
		code := GenerateTestUserCode()
		require.False(t, seen[code])

		seen[code] = true
	}
}

func TestGenerateTestSignatureSecret_ValidBase64(t *testing.T) {
	t.Parallel()
	s := GenerateTestSignatureSecret()
	decoded, err := base64.StdEncoding.DecodeString(s)
	require.NoError(t, err)
	assert.Len(t, decoded, 32)

}

func TestGenerateTestSignatureSecret_Unique(t *testing.T) {
	t.Parallel()
	a := GenerateTestSignatureSecret()
	b := GenerateTestSignatureSecret()
	assert.NotEqual(t, b, a)

}

func TestGenerateTestRunToken_Valid(t *testing.T) {
	t.Parallel()
	key := GenerateTestJWTKey()
	token := GenerateTestRunToken("run-123", key)
	require.NotEqual(t, "", token)

	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	require.False(t, err != nil ||
		!parsed.
			Valid)
	assert.Equal(t, "run-123",
		claims.Subject,
	)

}

func TestGenerateTestRunToken_WrongKey_Fails(t *testing.T) {
	t.Parallel()
	key1 := GenerateTestJWTKey()
	key2 := GenerateTestJWTKey()
	token := GenerateTestRunToken("run-123", key1)

	_, err := jwt.Parse(token, func(_ *jwt.Token) (any, error) {
		return []byte(key2), nil
	})
	require.Error(t, err)

}

func TestGenerateTestSSEToken_Valid(t *testing.T) {
	t.Parallel()
	key := GenerateTestJWTKey()
	token := GenerateTestSSEToken("proj-1", []string{"runs:read", "jobs:read"}, key)
	require.NotEqual(t, "", token)

	type sseClaims struct {
		jwt.RegisteredClaims
		ProjectID string   `json:"pid"`
		Scopes    []string `json:"scp,omitempty"`
	}
	claims := &sseClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	require.False(t, err != nil ||
		!parsed.
			Valid)
	assert.Equal(t, "strait:sse",
		claims.
			Issuer)
	assert.Equal(t, "proj-1",
		claims.ProjectID,
	)
	assert.Len(t, claims.Scopes,
		2)

}

func TestGenerateTestSSEToken_Expires(t *testing.T) {
	t.Parallel()
	key := GenerateTestJWTKey()
	token := GenerateTestSSEToken("proj-1", nil, key)

	claims := &jwt.RegisteredClaims{}
	parsed, _ := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	require.True(t, parsed.Valid)
	require.NotNil(t, claims.ExpiresAt)

}

func TestGenerateTestClaimToken_Length(t *testing.T) {
	t.Parallel()
	s := GenerateTestClaimToken()
	assert.Len(t, s, 64)

	// 32 bytes hex

}

func TestGenerateTestDatabaseURL_Format(t *testing.T) {
	t.Parallel()
	url := GenerateTestDatabaseURL()
	assert.True(t, strings.HasPrefix(url,
		"postgres://",
	))
	assert.True(t, strings.Contains(url,
		"sslmode=disable",
	))
	assert.True(t, strings.Contains(url,
		"test_"))

}

func TestGenerateTestDatabaseURL_Unique(t *testing.T) {
	t.Parallel()
	a := GenerateTestDatabaseURL()
	b := GenerateTestDatabaseURL()
	assert.NotEqual(t, b, a)

}

func TestGenerateTestRedisURL_Format(t *testing.T) {
	t.Parallel()
	url := GenerateTestRedisURL()
	assert.True(t, strings.HasPrefix(url,
		"redis://",
	),
	)

}
