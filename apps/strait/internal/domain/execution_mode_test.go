package domain

import "testing"

func TestExecutionMode_IsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mode ExecutionMode
		want bool
	}{
		{ExecutionModeHTTP, true},
		{ExecutionModeManaged, true},
		{"http", true},
		{"managed", true},
		{"", false},
		{"docker", false},
		{"unknown", false},
	}
	for _, tt := range tests {
		if got := tt.mode.IsValid(); got != tt.want {
			t.Errorf("ExecutionMode(%q).IsValid() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestMachinePreset_IsValid(t *testing.T) {
	t.Parallel()
	valid := []MachinePreset{
		PresetMicro, PresetSmall1x, PresetSmall2x,
		PresetMedium1x, PresetMedium2x, PresetLarge1x, PresetLarge2x,
	}
	for _, p := range valid {
		if !p.IsValid() {
			t.Errorf("MachinePreset(%q).IsValid() = false, want true", p)
		}
	}

	invalid := []MachinePreset{"", "tiny", "xlarge", "micro2x"}
	for _, p := range invalid {
		if p.IsValid() {
			t.Errorf("MachinePreset(%q).IsValid() = true, want false", p)
		}
	}
}

func TestExecutionMode_Constants(t *testing.T) {
	t.Parallel()
	if ExecutionModeHTTP != "http" {
		t.Errorf("ExecutionModeHTTP = %q, want http", ExecutionModeHTTP)
	}
	if ExecutionModeManaged != "managed" {
		t.Errorf("ExecutionModeManaged = %q, want managed", ExecutionModeManaged)
	}
}

func TestMachinePreset_AllPresetsCount(t *testing.T) {
	t.Parallel()
	all := []MachinePreset{
		PresetMicro, PresetSmall1x, PresetSmall2x,
		PresetMedium1x, PresetMedium2x, PresetLarge1x, PresetLarge2x,
	}
	if len(all) != 7 {
		t.Errorf("expected 7 presets, got %d", len(all))
	}
}
