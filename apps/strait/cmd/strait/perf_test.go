package main

import (
	"testing"
)

func TestParsePerfPeriodHours(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"", 24, false},
		{"24h", 24, false},
		{"72h", 72, false},
		{"7d", 168, false},
		{"30d", 720, false},
		{"90d", 2160, false},
		{"7D", 168, false},
		{" 7d ", 168, false},
		{"invalid", 0, true},
		{"1w", 0, true},
	}

	for _, tc := range tests {
		t.Run("period="+tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := parsePerfPeriodHours(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got %d", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("parsePerfPeriodHours(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestFailureRate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		total  int
		failed int
		want   float64
	}{
		{100, 5, 0.05},
		{0, 0, 0.0},
		{1, 1, 1.0},
		{1000, 0, 0.0},
	}

	for _, tc := range tests {
		got := failureRate(tc.total, tc.failed)
		if got != tc.want {
			t.Errorf("failureRate(%d, %d) = %f, want %f", tc.total, tc.failed, got, tc.want)
		}
	}
}
