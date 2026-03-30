package config

import (
	"testing"
)

func TestCORS_WildcardWithCredentials_Rejected(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	t.Setenv("INTERNAL_SECRET", "test-secret-value-long-enough")
	t.Setenv("JWT_SIGNING_KEY", "01234567890123456789012345678901")
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
	t.Setenv("JWT_SIGNING_KEY", "01234567890123456789012345678901")
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "false")

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
	t.Setenv("JWT_SIGNING_KEY", "01234567890123456789012345678901")
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
	t.Setenv("JWT_SIGNING_KEY", "01234567890123456789012345678901")

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
	t.Setenv("JWT_SIGNING_KEY", "01234567890123456789012345678901")

	_, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInternalSecret_Long_Accepted(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	t.Setenv("INTERNAL_SECRET", "this-is-a-very-long-secret-value-for-testing")
	t.Setenv("JWT_SIGNING_KEY", "01234567890123456789012345678901")

	_, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCORS_ExplicitOrigins_Allowed(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	t.Setenv("INTERNAL_SECRET", "test-secret-value-long-enough")
	t.Setenv("JWT_SIGNING_KEY", "01234567890123456789012345678901")
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
