package config

import "testing"

// requiredEnv sets the minimum env vars Load() needs to succeed. t.Setenv is
// incompatible with t.Parallel, so these tests run serially.
func requiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("INTERNAL_SECRET", "test-secret-value")
	t.Setenv("JWT_SIGNING_KEY", "this-is-a-very-long-key-for-jwt-signing-1234")
}

// TestConfig_MaxWorkflowContinueDepth_Default verifies the continue-as-new depth
// cap defaults to the documented runaway guard when the env var is unset.
func TestConfig_MaxWorkflowContinueDepth_Default(t *testing.T) {
	requiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MaxWorkflowContinueDepth != 100000 {
		t.Errorf("MaxWorkflowContinueDepth = %d, want 100000", cfg.MaxWorkflowContinueDepth)
	}
}

// TestConfig_MaxWorkflowContinueDepth_Override verifies an operator can lower the
// cap via MAX_WORKFLOW_CONTINUE_DEPTH.
func TestConfig_MaxWorkflowContinueDepth_Override(t *testing.T) {
	requiredEnv(t)
	t.Setenv("MAX_WORKFLOW_CONTINUE_DEPTH", "42")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MaxWorkflowContinueDepth != 42 {
		t.Errorf("MaxWorkflowContinueDepth = %d, want 42", cfg.MaxWorkflowContinueDepth)
	}
}

// TestConfig_MaxWorkflowContinueDepth_Invalid verifies a non-numeric value is
// rejected by the loader rather than silently coerced.
func TestConfig_MaxWorkflowContinueDepth_Invalid(t *testing.T) {
	requiredEnv(t)
	t.Setenv("MAX_WORKFLOW_CONTINUE_DEPTH", "not-a-number")

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for non-numeric MAX_WORKFLOW_CONTINUE_DEPTH, got nil")
	}
}
