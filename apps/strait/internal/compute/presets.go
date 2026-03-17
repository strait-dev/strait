package compute

import "fmt"

// Preset defines the resource allocation for a machine preset.
type Preset struct {
	Name          string // e.g. "micro", "small-1x"
	CPUs          int    // Number of vCPUs
	MemoryMB      int    // Memory in megabytes
	FlyGuestSize  string // Fly Machines guest size string
	CostPerSecond int64  // Cost in micro-USD per second of wall time
}

// AllPresets is the canonical list of supported machine presets.
var AllPresets = map[string]Preset{
	"micro":     {Name: "micro", CPUs: 1, MemoryMB: 256, FlyGuestSize: "shared-cpu-1x", CostPerSecond: 17},
	"small-1x":  {Name: "small-1x", CPUs: 1, MemoryMB: 512, FlyGuestSize: "shared-cpu-1x", CostPerSecond: 34},
	"small-2x":  {Name: "small-2x", CPUs: 2, MemoryMB: 1024, FlyGuestSize: "shared-cpu-2x", CostPerSecond: 68},
	"medium-1x": {Name: "medium-1x", CPUs: 2, MemoryMB: 4096, FlyGuestSize: "performance-1x", CostPerSecond: 85},
	"medium-2x": {Name: "medium-2x", CPUs: 4, MemoryMB: 8192, FlyGuestSize: "performance-2x", CostPerSecond: 170},
	"large-1x":  {Name: "large-1x", CPUs: 8, MemoryMB: 16384, FlyGuestSize: "performance-4x", CostPerSecond: 340},
	"large-2x":  {Name: "large-2x", CPUs: 16, MemoryMB: 32768, FlyGuestSize: "performance-8x", CostPerSecond: 680},
}

// PresetOrder defines the canonical ordering of presets from smallest to largest.
var PresetOrder = []string{"micro", "small-1x", "small-2x", "medium-1x", "medium-2x", "large-1x", "large-2x"}

// PresetFromName returns the preset definition for a given name.
func PresetFromName(name string) (Preset, error) {
	p, ok := AllPresets[name]
	if !ok {
		return Preset{}, fmt.Errorf("unknown machine preset: %q", name)
	}
	return p, nil
}

// NextPreset returns the next larger preset in the ordering.
// Returns ("", false) if the preset is already the largest or unknown.
func NextPreset(current string) (string, bool) {
	idx := PresetIndex(current)
	if idx < 0 || idx >= len(PresetOrder)-1 {
		return "", false
	}
	return PresetOrder[idx+1], true
}

// IsMaxPreset returns true if the preset is the largest available.
func IsMaxPreset(name string) bool {
	return name == PresetOrder[len(PresetOrder)-1]
}

// PresetMemoryMB returns the memory in MB for a preset, or 0 if unknown.
func PresetMemoryMB(name string) int {
	p, ok := AllPresets[name]
	if !ok {
		return 0
	}
	return p.MemoryMB
}

// PresetIndex returns the index of a preset in the ordering, or -1 if unknown.
func PresetIndex(name string) int {
	for i, p := range PresetOrder {
		if p == name {
			return i
		}
	}
	return -1
}
