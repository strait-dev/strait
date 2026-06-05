package config

import (
	"testing"
)

func setRequiredRuntimeEnv(t *testing.T) {
	t.Helper()
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("SEQUIN_BASE_URL", "http://localhost:7376")
	t.Setenv("SEQUIN_CONSUMER_NAME", "strait-cdc")
	t.Setenv("SEQUIN_API_TOKEN", "sequin-api-token")
	t.Setenv("SEQUIN_WEBHOOK_SECRET", "sequin-webhook-secret")
}

func TestCORS_WildcardWithCredentials_Rejected(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	t.Setenv("INTERNAL_SECRET", "test-secret-value-long-enough")
	t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
	setRequiredRuntimeEnv(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "true")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for wildcard CORS with credentials, got nil")
	}
	want := "config CORS_ALLOWED_ORIGINS: wildcard origin (*) is not allowed when CORS_ALLOW_CREDENTIALS is true"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestCORS_WildcardWithoutCredentials_Allowed(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	t.Setenv("INTERNAL_SECRET", "test-secret-value-long-enough")
	t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
	setRequiredRuntimeEnv(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "false")
	t.Setenv("STRAIT_ENV", "development")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "*" {
		t.Errorf("CORSAllowedOrigins = %v, want [*]", cfg.CORSAllowedOrigins)
	}
}

func TestCORS_EmptyOrigins_Allowed(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	t.Setenv("INTERNAL_SECRET", "test-secret-value-long-enough")
	t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
	setRequiredRuntimeEnv(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CORSAllowedOrigins) != 0 {
		t.Errorf("CORSAllowedOrigins = %v, want empty", cfg.CORSAllowedOrigins)
	}
}

func TestInternalSecret_TooShort_Rejected(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	t.Setenv("INTERNAL_SECRET", "short-15-chars!") // exactly 15 chars
	t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
	setRequiredRuntimeEnv(t)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for short internal secret, got nil")
	}
	want := "config INTERNAL_SECRET: must be at least 16 characters"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestInternalSecret_MinLength_Accepted(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	t.Setenv("INTERNAL_SECRET", "exactly-16-chars") // exactly 16 chars
	t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
	setRequiredRuntimeEnv(t)

	_, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInternalSecret_Long_Accepted(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	t.Setenv("INTERNAL_SECRET", "this-is-a-very-long-secret-value-for-testing")
	t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
	setRequiredRuntimeEnv(t)

	_, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCORS_Wildcard_RejectedInProduction(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	t.Setenv("INTERNAL_SECRET", "test-secret-value-long-enough")
	t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
	setRequiredRuntimeEnv(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	t.Setenv("STRAIT_ENV", "production")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for wildcard CORS in production, got nil")
	}
	want := "config CORS_ALLOWED_ORIGINS: wildcard origin (*) is not allowed in non-development environments"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestSSLMode_Disable_RejectedInProduction(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test?sslmode=disable")
	t.Setenv("INTERNAL_SECRET", "test-secret-value-long-enough")
	t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
	setRequiredRuntimeEnv(t)
	t.Setenv("STRAIT_ENV", "production")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for sslmode=disable in production, got nil")
	}
	want := "config DATABASE_URL: sslmode=disable is not allowed in non-development environments"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestSSLMode_Disable_AllowedInDev(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test?sslmode=disable")
	t.Setenv("INTERNAL_SECRET", "test-secret-value-long-enough")
	t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
	setRequiredRuntimeEnv(t)
	t.Setenv("STRAIT_ENV", "development")

	_, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCORS_ExplicitOrigins_Allowed(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	t.Setenv("INTERNAL_SECRET", "test-secret-value-long-enough")
	t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
	setRequiredRuntimeEnv(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.strait.dev,https://dashboard.strait.dev")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Errorf("CORSAllowedOrigins length = %d, want 2", len(cfg.CORSAllowedOrigins))
	}
}
