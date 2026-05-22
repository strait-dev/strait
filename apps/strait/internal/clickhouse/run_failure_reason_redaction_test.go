package clickhouse

import (
	"strings"
	"testing"
)

func TestSafeRunFailureReason_DoesNotReturnRawMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "bearer token",
			message: "request failed Authorization: Bearer secret-token-123",
			want:    "application_error",
		},
		{
			name:    "callback url",
			message: "webhook failed https://user:pass@example.com/path/token?api_key=secret",
			want:    "application_error",
		},
		{
			name:    "timeout",
			message: "deadline exceeded while calling worker with password=hunter2",
			want:    "timeout",
		},
		{
			name:    "network",
			message: "connection refused by backend token=abc",
			want:    "network_error",
		},
		{
			name:    "empty",
			message: "",
			want:    "unknown_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := safeRunFailureReason(tt.message)
			if got != tt.want {
				t.Fatalf("safeRunFailureReason() = %q, want %q", got, tt.want)
			}
			for _, leaked := range []string{"secret-token", "user:pass", "api_key", "hunter2", "token=abc"} {
				if strings.Contains(got, leaked) {
					t.Fatalf("safe failure reason leaked %q in %q", leaked, got)
				}
			}
		})
	}
}
