package compute

import "testing"

func TestCalculateCost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		preset   string
		duration float64
		want     int64
	}{
		{"micro", 1.0, CostMicro},
		{"micro", 60.0, CostMicro * 60},
		{"small-1x", 30.0, CostSmall1x * 30},
		{"small-2x", 30.0, CostSmall2x * 30},
		{"medium-1x", 120.0, CostMedium1x * 120},
		{"medium-2x", 120.0, CostMedium2x * 120},
		{"large-1x", 60.0, CostLarge1x * 60},
		{"large-2x", 60.0, CostLarge2x * 60},
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

func TestCalculateCost_NegativeDuration(t *testing.T) {
	t.Parallel()
	cost, err := CalculateCost("micro", -10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cost != 0 {
		t.Errorf("cost = %d, want 0 for negative duration", cost)
	}
}

func TestCalculateCost_VerySmallFraction(t *testing.T) {
	t.Parallel()
	// 17 * 0.001 = 0.017 → rounds to 0
	cost, err := CalculateCost("micro", 0.001)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cost != 0 {
		t.Errorf("cost = %d, want 0 for sub-penny fraction", cost)
	}
}

func TestCalculateCost_LargeDuration(t *testing.T) {
	t.Parallel()
	// large-2x for 1 hour
	want := CostLarge2x * 3600
	cost, err := CalculateCost("large-2x", 3600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cost != want {
		t.Errorf("cost = %d, want %d", cost, want)
	}
}

func TestEstimateCost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		preset  string
		timeout int
		want    int64
	}{
		{"small-1x", 300, CostSmall1x * 300},
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
