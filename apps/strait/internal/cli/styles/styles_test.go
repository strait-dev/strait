package styles

import (
	"testing"
)

func TestStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status string
	}{
		{name: "completed", status: "completed"},
		{name: "failed", status: "failed"},
		{name: "system_failed", status: "system_failed"},
		{name: "crashed", status: "crashed"},
		{name: "executing", status: "executing"},
		{name: "queued", status: "queued"},
		{name: "dequeued", status: "dequeued"},
		{name: "delayed", status: "delayed"},
		{name: "waiting", status: "waiting"},
		{name: "canceled", status: "canceled"},
		{name: "expired", status: "expired"},
		{name: "timed_out", status: "timed_out"},
		{name: "unknown", status: "unknown_status"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := Status(tt.status)
			if result == "" {
				t.Fatalf("Status(%q) returned empty string", tt.status)
			}
			// Unknown statuses should be returned as-is (no ANSI codes added)
			if tt.status == "unknown_status" && result != tt.status {
				// With color disabled, result equals input; with color enabled,
				// it should still contain the original text.
				if len(result) < len(tt.status) {
					t.Fatalf("Status(%q) = %q, expected to contain original text", tt.status, result)
				}
			}
		})
	}
}

func TestEnabled(t *testing.T) {
	t.Parallel()

	t.Run("enabled true", func(t *testing.T) {
		t.Parallel()
		result := Enabled(true)
		if result == "" {
			t.Fatal("Enabled(true) returned empty string")
		}
	})

	t.Run("enabled false", func(t *testing.T) {
		t.Parallel()
		result := Enabled(false)
		if result == "" {
			t.Fatal("Enabled(false) returned empty string")
		}
	})
}

func TestForceNoColor(t *testing.T) {
	// Not parallel — modifies global state
	ForceNoColor()

	// After forcing no color, Status should return plain text
	got := Status("completed")
	if got != "completed" {
		t.Fatalf("after ForceNoColor(), Status('completed') = %q, want 'completed'", got)
	}

	got = Enabled(true)
	if got != "true" {
		t.Fatalf("after ForceNoColor(), Enabled(true) = %q, want 'true'", got)
	}

	got = Enabled(false)
	if got != "false" {
		t.Fatalf("after ForceNoColor(), Enabled(false) = %q, want 'false'", got)
	}
}
