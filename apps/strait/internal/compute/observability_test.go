package compute

import "testing"

func TestClassifyFlyError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status    int
		retryable bool
		fatal     bool
		backoff   int
	}{
		{200, false, false, 0},
		{201, false, false, 0},
		{429, true, false, 10},
		{503, true, false, 30},
		{422, false, true, 0},
		{500, true, false, 5},
		{502, true, false, 5},
	}
	for _, tt := range tests {
		retryable, fatal, backoff := ClassifyFlyError(tt.status)
		if retryable != tt.retryable {
			t.Errorf("status %d: retryable = %v, want %v", tt.status, retryable, tt.retryable)
		}
		if fatal != tt.fatal {
			t.Errorf("status %d: fatal = %v, want %v", tt.status, fatal, tt.fatal)
		}
		if backoff != tt.backoff {
			t.Errorf("status %d: backoff = %d, want %d", tt.status, backoff, tt.backoff)
		}
	}
}
