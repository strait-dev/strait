package domain

import "testing"

func TestExecutionMode_IsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mode ExecutionMode
		want bool
	}{
		{ExecutionModeHTTP, true},
		{ExecutionModeWorker, true},
		{"http", true},
		{"worker", true},
		{"", false},
		{"managed", false},
		{"docker", false},
		{"unknown", false},
	}
	for _, tt := range tests {
		if got := tt.mode.IsValid(); got != tt.want {
			t.Errorf("ExecutionMode(%q).IsValid() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestExecutionMode_Constants(t *testing.T) {
	t.Parallel()
	if ExecutionModeHTTP != "http" {
		t.Errorf("ExecutionModeHTTP = %q, want http", ExecutionModeHTTP)
	}
	if ExecutionModeWorker != "worker" {
		t.Errorf("ExecutionModeWorker = %q, want worker", ExecutionModeWorker)
	}
}
