package compute

import "testing"

func TestCalculateCost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		preset   string
		duration float64
		want     int64
	}{
		{"micro", 1.0, 17},
		{"micro", 60.0, 1020},
		{"small-1x", 30.0, 1020},
		{"small-2x", 30.0, 2040},
		{"medium-1x", 120.0, 10200},
		{"medium-2x", 120.0, 20400},
		{"large-1x", 60.0, 20400},
		{"large-2x", 60.0, 40800},
		{"micro", 0, 0},
		{"micro", 1.5, 26}, // 17 * 1.5 = 25.5 → rounds to 26
	}
	for _, tt := range tests {
		cost, err := CalculateCost(tt.preset, tt.duration)
		if err != nil {
			t.Errorf("CalculateCost(%q, %f) error = %v", tt.preset, tt.duration, err)
			continue
		}
		if cost != tt.want {
			t.Errorf("CalculateCost(%q, %f) = %d, want %d", tt.preset, tt.duration, cost, tt.want)
		}
	}
}

func TestCalculateCost_InvalidPreset(t *testing.T) {
	t.Parallel()
	_, err := CalculateCost("invalid", 60)
	if err == nil {
		t.Error("expected error for invalid preset")
	}
}

func TestEstimateCost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		preset  string
		timeout int
		want    int64
	}{
		{"small-1x", 300, 10200},
		{"micro", 0, 0},
	}
	for _, tt := range tests {
		cost, err := EstimateCost(tt.preset, tt.timeout)
		if err != nil {
			t.Errorf("EstimateCost(%q, %d) error = %v", tt.preset, tt.timeout, err)
			continue
		}
		if cost != tt.want {
			t.Errorf("EstimateCost(%q, %d) = %d, want %d", tt.preset, tt.timeout, cost, tt.want)
		}
	}
}
