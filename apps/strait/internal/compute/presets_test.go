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
