package compute

import "testing"

func TestPresetFromName_AllValid(t *testing.T) {
	t.Parallel()
	valid := []string{"micro", "small-1x", "small-2x", "medium-1x", "medium-2x", "large-1x", "large-2x"}
	for _, name := range valid {
		p, err := PresetFromName(name)
		if err != nil {
			t.Errorf("PresetFromName(%q) error = %v", name, err)
			continue
		}
		if p.CPUs == 0 {
			t.Errorf("preset %q CPUs = 0", name)
		}
		if p.MemoryMB == 0 {
			t.Errorf("preset %q MemoryMB = 0", name)
		}
		if p.FlyGuestSize == "" {
			t.Errorf("preset %q FlyGuestSize empty", name)
		}
		if p.CostPerSecond == 0 {
			t.Errorf("preset %q CostPerSecond = 0", name)
		}
	}
}

func TestPresetFromName_Invalid(t *testing.T) {
	t.Parallel()
	invalid := []string{"", "invalid", "tiny", "xlarge", "micro2x"}
	for _, name := range invalid {
		_, err := PresetFromName(name)
		if err == nil {
			t.Errorf("PresetFromName(%q) expected error", name)
		}
	}
}

func TestPresetFromName_CostRates(t *testing.T) {
	t.Parallel()
	expected := map[string]int64{
		"micro":     17,
		"small-1x":  34,
		"small-2x":  68,
		"medium-1x": 85,
		"medium-2x": 170,
		"large-1x":  340,
		"large-2x":  680,
	}
	for name, cost := range expected {
		p, _ := PresetFromName(name)
		if p.CostPerSecond != cost {
			t.Errorf("preset %q CostPerSecond = %d, want %d", name, p.CostPerSecond, cost)
		}
	}
}

func TestPresetFromName_FlyMapping(t *testing.T) {
	t.Parallel()
	expected := map[string]string{
		"micro":     "shared-cpu-1x",
		"small-1x":  "shared-cpu-1x",
		"small-2x":  "shared-cpu-2x",
		"medium-1x": "performance-1x",
		"medium-2x": "performance-2x",
		"large-1x":  "performance-4x",
		"large-2x":  "performance-8x",
	}
	for name, size := range expected {
		p, _ := PresetFromName(name)
		if p.FlyGuestSize != size {
			t.Errorf("preset %q FlyGuestSize = %q, want %q", name, p.FlyGuestSize, size)
		}
	}
}

func TestNextPreset(t *testing.T) {
	t.Parallel()
	tests := []struct {
		current  string
		wantNext string
		wantOK   bool
	}{
		{"micro", "small-1x", true},
		{"small-1x", "small-2x", true},
		{"small-2x", "medium-1x", true},
		{"medium-1x", "medium-2x", true},
		{"medium-2x", "large-1x", true},
		{"large-1x", "large-2x", true},
		{"large-2x", "", false},
		{"unknown", "", false},
	}
	for _, tt := range tests {
		next, ok := NextPreset(tt.current)
		if ok != tt.wantOK || next != tt.wantNext {
			t.Errorf("NextPreset(%q) = (%q, %v), want (%q, %v)", tt.current, next, ok, tt.wantNext, tt.wantOK)
		}
	}
}

func TestIsMaxPreset(t *testing.T) {
	t.Parallel()
	if !IsMaxPreset("large-2x") {
		t.Error("large-2x should be max preset")
	}
	if IsMaxPreset("micro") {
		t.Error("micro should not be max preset")
	}
	if IsMaxPreset("unknown") {
		t.Error("unknown should not be max preset")
	}
}

func TestPresetIndex(t *testing.T) {
	t.Parallel()
	for i, name := range PresetOrder {
		if got := PresetIndex(name); got != i {
			t.Errorf("PresetIndex(%q) = %d, want %d", name, got, i)
		}
	}
	if got := PresetIndex("unknown"); got != -1 {
		t.Errorf("PresetIndex(unknown) = %d, want -1", got)
	}
}

func TestPresetMemoryMB(t *testing.T) {
	t.Parallel()
	expected := map[string]int{
		"micro":     256,
		"small-1x":  512,
		"small-2x":  1024,
		"medium-1x": 4096,
		"medium-2x": 8192,
		"large-1x":  16384,
		"large-2x":  32768,
	}
	for name, mb := range expected {
		if got := PresetMemoryMB(name); got != mb {
			t.Errorf("PresetMemoryMB(%q) = %d, want %d", name, got, mb)
		}
	}
	if got := PresetMemoryMB("unknown"); got != 0 {
		t.Errorf("PresetMemoryMB(unknown) = %d, want 0", got)
	}
}
