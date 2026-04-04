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
		"micro":     CostMicro,
		"small-1x":  CostSmall1x,
		"small-2x":  CostSmall2x,
		"medium-1x": CostMedium1x,
		"medium-2x": CostMedium2x,
		"large-1x":  CostLarge1x,
		"large-2x":  CostLarge2x,
	}
	for name, cost := range expected {
		p, _ := PresetFromName(name)
		if p.CostPerSecond != cost {
			t.Errorf("preset %q CostPerSecond = %d, want %d", name, p.CostPerSecond, cost)
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
