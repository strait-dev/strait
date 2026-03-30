package testutil

import (
	"strings"
	"testing"
)

func TestGenerateTestSecret_Length(t *testing.T) {
	t.Parallel()

	tests := []struct {
		byteLen    int
		wantHexLen int
	}{
		{16, 32},
		{32, 64},
		{64, 128},
	}

	for _, tc := range tests {
		s := GenerateTestSecret(tc.byteLen)
		if len(s) != tc.wantHexLen {
			t.Errorf("GenerateTestSecret(%d) len = %d, want %d", tc.byteLen, len(s), tc.wantHexLen)
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

func TestGenerateTestWebhookSecret(t *testing.T) {
	t.Parallel()

	s := GenerateTestWebhookSecret()
	if !strings.HasPrefix(s, "whsec_") {
		t.Errorf("webhook secret should start with whsec_, got %q", s)
	}
	if len(s) < 38 { // "whsec_" (6) + 32 hex chars
		t.Errorf("webhook secret too short: %d chars", len(s))
	}
}

func TestGenerateTestJWTKey(t *testing.T) {
	t.Parallel()

	s := GenerateTestJWTKey()
	if len(s) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("JWT key length = %d, want 64", len(s))
	}
}

func TestGenerateTestInternalSecret(t *testing.T) {
	t.Parallel()

	s := GenerateTestInternalSecret()
	if len(s) < 16 {
		t.Errorf("internal secret length %d < 16 (minimum required)", len(s))
	}
}

func TestGenerateTestAPIKey(t *testing.T) {
	t.Parallel()

	s := GenerateTestAPIKey()
	if !strings.HasPrefix(s, "strait_") {
		t.Errorf("API key should start with strait_, got %q", s)
	}
	if len(s) < 71 { // "strait_" (7) + 64 hex chars
		t.Errorf("API key too short: %d chars", len(s))
	}
}
