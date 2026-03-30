package config

import (
	"strings"
	"testing"
)

func TestConfig_Redacted_MasksSecrets(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		InternalSecret:     "super-secret-key",
		JWTSigningKey:      "jwt-key",
		PolarAccessToken:   "polar-token",
		PolarWebhookSecret: "whsec_test",
		ResendAPIKey:       "re_test",
		PostHogAPIKey:      "phc_test",
	}

	r := cfg.Redacted()
	for key, val := range r {
		str, ok := val.(string)
		if !ok {
			continue
		}
		if str == "super-secret-key" || str == "jwt-key" || str == "polar-token" || str == "whsec_test" || str == "re_test" || str == "phc_test" {
			t.Errorf("secret leaked in Redacted() for key %q: %v", key, val)
		}
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
	if r["Mode"] != "all" {
		t.Errorf("Mode = %v, want 'all'", r["Mode"])
	}
	if r["Port"] != 8080 {
		t.Errorf("Port = %v, want 8080", r["Port"])
	}
	if r["Edition"] != "cloud" {
		t.Errorf("Edition = %v, want 'cloud'", r["Edition"])
	}
}

func TestConfig_String_NoSecrets(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Mode:               "all",
		Port:               8080,
		InternalSecret:     "my-secret-value",
		JWTSigningKey:      "jwt-secret-123",
		PolarAccessToken:   "polar-secret-456",
		PolarWebhookSecret: "whsec_secret789",
		ResendAPIKey:       "re_secret",
	}

	str := cfg.String()
	secrets := []string{"my-secret-value", "jwt-secret-123", "polar-secret-456", "whsec_secret789", "re_secret"}
	for _, secret := range secrets {
		if strings.Contains(str, secret) {
			t.Errorf("Config.String() contains secret: %q", secret)
		}
	}
}

func FuzzConfig_String(f *testing.F) {
	f.Add("secret1", "secret2", "secret3")

	f.Fuzz(func(t *testing.T, s1, s2, s3 string) {
		cfg := &Config{
			InternalSecret:   s1,
			JWTSigningKey:    s2,
			PolarAccessToken: s3,
		}
		str := cfg.String()
		if s1 != "" && strings.Contains(str, s1) {
			t.Errorf("String() leaks InternalSecret: %q", s1)
		}
		if s2 != "" && strings.Contains(str, s2) {
			t.Errorf("String() leaks JWTSigningKey: %q", s2)
		}
		if s3 != "" && strings.Contains(str, s3) {
			t.Errorf("String() leaks PolarAccessToken: %q", s3)
		}
	})
}
