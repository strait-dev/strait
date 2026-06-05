package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Redacted_MasksSecrets(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		InternalSecret:      "super-secret-key",
		ProfilingSecret:     "pprof-secret-key",
		JWTSigningKey:       "jwt-key",
		StripeSecretKey:     "sk_test_123",
		StripeWebhookSecret: "whsec_test",
		ResendAPIKey:        "re_test",
		PostHogAPIKey:       "phc_test",
	}

	r := cfg.Redacted()
	for key, val := range r {
		str, ok := val.(string)
		if !ok {
			continue
		}
		assert.NotContains(t, []string{
			"super-secret-key",
			"pprof-secret-key",
			"jwt-key",
			"sk_test_123",
			"whsec_test",
			"re_test",
			"phc_test",
		}, str, "key %q", key)
	}
}

func TestConfig_Redacted_PreservesPublicFields(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Mode:    "all",
		Port:    8080,
		Edition: "cloud",
	}

	r := cfg.Redacted()
	assert.Equal(t, "all", r["Mode"])
	assert.Equal(t, 8080, r["Port"])
	assert.Equal(t, "cloud", r["Edition"])
}

func TestConfig_String_NoSecrets(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Mode:                "all",
		Port:                8080,
		InternalSecret:      "my-secret-value",
		ProfilingSecret:     "pprof-secret-value",
		JWTSigningKey:       "jwt-secret-123",
		StripeSecretKey:     "sk_test_secret456",
		StripeWebhookSecret: "whsec_secret789",
		ResendAPIKey:        "re_secret",
	}

	str := cfg.String()
	secrets := []string{"my-secret-value", "pprof-secret-value", "jwt-secret-123", "sk_test_secret456", "whsec_secret789", "re_secret"}
	for _, secret := range secrets {
		assert.NotContains(t, str, secret)
	}
}

func TestConfig_String_NoLeadingSpace(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Mode: "all",
		Port: 8080,
	}
	str := cfg.String()
	assert.False(t, strings.HasPrefix(str, " "))
	require.NotEmpty(t, str)
}

func FuzzConfig_String(f *testing.F) {
	f.Add("secret1", "secret2", "secret3")

	f.Fuzz(func(t *testing.T, s1, s2, s3 string) {
		cfg := &Config{
			InternalSecret:  s1,
			JWTSigningKey:   s2,
			StripeSecretKey: s3,
		}
		str := cfg.String()
		if s1 != "" {
			assert.NotContains(t, str, s1)
		}
		if s2 != "" {
			assert.NotContains(t, str, s2)
		}
		if s3 != "" {
			assert.NotContains(t, str, s3)
		}
	})
}
