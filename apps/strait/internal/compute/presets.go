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

// PresetFromName returns the preset definition for a given name.
func PresetFromName(name string) (Preset, error) {
	p, ok := AllPresets[name]
	if !ok {
		return Preset{}, fmt.Errorf("unknown machine preset: %q", name)
	}
	return p, nil
}
