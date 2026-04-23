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

func TestValidate_AuditSIEMEndpointWithUserinfo(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_SIEM_ENDPOINT", "https://u:p@siem.example.com/audit")
	t.Setenv("AUDIT_SIEM_AUTH_TOKEN", "secret-bearer-token")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for SIEM endpoint containing userinfo")
	}
}

func TestValidate_AuditSIEMEndpointUnparseable(t *testing.T) {
	setRequiredAuditEnv(t)
	// An unparseable URL should be rejected with a clear field error,
	// not a silent fallthrough that tries to connect at runtime.
	t.Setenv("AUDIT_SIEM_ENDPOINT", "://not-a-valid-url")
	t.Setenv("AUDIT_SIEM_AUTH_TOKEN", "secret-bearer-token")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for unparseable SIEM endpoint")
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

func TestValidate_AuditDLQReclaimBatchZero(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_RECLAIM_BATCH", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for zero DLQ reclaim batch")
	}
}

func TestValidate_AuditDLQReclaimBatchOne(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_RECLAIM_BATCH", "1")

	_, err := Load()
	if err != nil {
		t.Fatalf("DLQ reclaim batch=1 should be valid: %v", err)
	}
}

func TestValidate_AuditBufferExactly256(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_ASYNC_BUFFER_SIZE", "256")

	_, err := Load()
	if err != nil {
		t.Fatalf("buffer size=256 should be valid: %v", err)
	}
}

func TestValidate_AuditBuffer255(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_ASYNC_BUFFER_SIZE", "255")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for buffer size=255")
	}
}

func TestValidate_AuditDLQMaxAgeDaysZero(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_MAX_AGE_DAYS", "0")

	_, err := Load()
	if err != nil {
		t.Fatalf("DLQ max age=0 should be valid (disables sweep): %v", err)
	}
}

func TestValidate_AuditDLQMaxReclaimAttemptsZero(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_MAX_RECLAIM_ATTEMPTS", "0")

	_, err := Load()
	if err != nil {
		t.Fatalf("DLQ max reclaim attempts=0 should be valid: %v", err)
	}
}

func TestValidate_AuditRetentionZero(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_RETENTION_DEFAULT_DAYS", "0")

	_, err := Load()
	if err != nil {
		t.Fatalf("retention=0 should be valid: %v", err)
	}
}

func TestValidate_JWTSigningKeyExactly32Chars(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("INTERNAL_SECRET", "test-secret-value")
	t.Setenv("JWT_SIGNING_KEY", "exactly-32-characters-key-value!")

	_, err := Load()
	if err != nil {
		t.Fatalf("32-char JWT key should be valid: %v", err)
	}
}

func TestValidate_JWTSigningKey31Chars(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("INTERNAL_SECRET", "test-secret-value")
	t.Setenv("JWT_SIGNING_KEY", "exactly-31-characters-key-valu")

	_, err := Load()
	if err == nil {
		t.Fatal("31-char JWT key should be rejected")
	}
}

func TestValidate_AuditDLQMaxAgeDaysNegative(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_MAX_AGE_DAYS", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative DLQ max age days")
	}
}

func TestValidate_AuditDLQMaxReclaimAttemptsNegative(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_MAX_RECLAIM_ATTEMPTS", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative DLQ max reclaim attempts")
	}
}

func TestValidate_AuditDLQReclaimBatchNegative(t *testing.T) {
	setRequiredAuditEnv(t)
	t.Setenv("AUDIT_DLQ_RECLAIM_BATCH", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative DLQ reclaim batch")
	}
}

func TestValidate_K8sRuntimeWithNamespace(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("COMPUTE_RUNTIME", "k8s")
	t.Setenv("K8S_NAMESPACE", "default")

	_, err := Load()
	if err != nil {
		t.Fatalf("k8s runtime with namespace should be valid: %v", err)
	}
}
