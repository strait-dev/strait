package config

import (
	"testing"
)

// setRequiredAuditEnv sets the minimum required env vars plus valid audit defaults.
func setRequiredAuditEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("INTERNAL_SECRET", "test-secret-value")
	t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
	t.Setenv("AUDIT_RETENTION_DEFAULT_DAYS", "365")
	t.Setenv("AUDIT_ASYNC_BUFFER_SIZE", "4096")
}

func TestValidate_AuditRetentionNegative(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_RETENTION_DEFAULT_DAYS", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative audit retention")
	}
}

func TestValidate_AuditBufferTooSmall(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_ASYNC_BUFFER_SIZE", "100")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for buffer size < 256")
	}
}

func TestValidate_AuditSIEMEndpointWithoutToken(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_SIEM_ENDPOINT", "https://siem.example.com/audit")
	t.Setenv("AUDIT_SIEM_AUTH_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for SIEM endpoint without token")
	}
}

func TestValidate_AuditSIEMEndpointWithToken(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_SIEM_ENDPOINT", "https://siem.example.com/audit")
	t.Setenv("AUDIT_SIEM_AUTH_TOKEN", "secret-bearer-token")

	_, err := Load()
	if err != nil {
		t.Fatalf("unexpected error for valid SIEM config: %v", err)
	}
}

func TestValidate_AuditDefaultsValid(t *testing.T) {
	setRequiredAuditEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error with valid audit defaults: %v", err)
	}
	if cfg.AuditRetentionDefaultDays != 365 {
		t.Errorf("AuditRetentionDefaultDays = %d, want 365", cfg.AuditRetentionDefaultDays)
	}
	if cfg.AuditAsyncBufferSize != 4096 {
		t.Errorf("AuditAsyncBufferSize = %d, want 4096", cfg.AuditAsyncBufferSize)
	}
}
