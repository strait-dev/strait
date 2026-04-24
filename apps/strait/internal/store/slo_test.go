package store

import (
	"testing"
)

func TestValidateSLOWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		window  int
		max     int
		wantErr bool
	}{
		{"exceeds max", 721, 720, true},
		{"at max", 720, 720, false},
		{"within max", 24, 720, false},
		{"zero window", 0, 720, false},
		{"max not set", 99999, 0, false},
		{"negative max", 100, -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateSLOWindow(tt.window, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSLOWindow(%d, %d) error = %v, wantErr %v", tt.window, tt.max, err, tt.wantErr)
			}
		})
	}
}
