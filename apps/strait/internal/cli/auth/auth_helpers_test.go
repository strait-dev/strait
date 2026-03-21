package auth

import (
	"testing"
)

func TestDashboardURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"http://localhost:8080", "http://localhost:5173"},
		{"https://api.example.com", "https://app.example.com"},
		{"https://my-api.example.com:8080", "https://my-app.example.com:5173"},
		{"https://plain.example.com", "https://plain.example.com"},
		{"", ""},
		{"http://localhost:3000", "http://localhost:3000"},
	}

	for _, tc := range tests {
		t.Run("input="+tc.input, func(t *testing.T) {
			t.Parallel()
			got := DashboardURL(tc.input)
			if got != tc.want {
				t.Fatalf("DashboardURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestMaskAPIKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"strait_live_abc123def456", "...f456"},
		{"ab", "***"},
		{"abcd", "***"},
		{"abcde", "...bcde"},
		{"", "***"},
	}

	for _, tc := range tests {
		t.Run("input="+tc.input, func(t *testing.T) {
			t.Parallel()
			got := MaskAPIKey(tc.input)
			if got != tc.want {
				t.Fatalf("MaskAPIKey(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
