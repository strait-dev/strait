package domain

// Per-second compute costs in micro-USD ($0.000001) for each machine preset.
// These are what we charge users (user-facing price).
const (
	CostMicro    int64 = 17   // 1 vCPU / 256 MB  -- $0.061/hr
	CostSmall1x  int64 = 34   // 1 vCPU / 512 MB  -- $0.122/hr
	CostSmall2x  int64 = 68   // 2 vCPU / 1 GB    -- $0.245/hr
	CostMedium1x int64 = 130  // 2 vCPU / 4 GB    -- $0.468/hr
	CostMedium2x int64 = 260  // 4 vCPU / 8 GB    -- $0.936/hr
	CostLarge1x  int64 = 525  // 8 vCPU / 16 GB   -- $1.890/hr
	CostLarge2x  int64 = 1050 // 16 vCPU / 32 GB  -- $3.780/hr
)

// Preset defines the resource allocation for a machine preset.
type Preset struct {
	Name          string // e.g. "micro", "small-1x"
	CPUs          int    // Number of vCPUs
	MemoryMB      int    // Memory in megabytes
	CostPerSecond int64  // User-facing cost in micro-USD per second
}

// AllPresets maps preset names to their definitions.
var AllPresets = map[string]Preset{
	"micro":     {Name: "micro", CPUs: 1, MemoryMB: 256, CostPerSecond: CostMicro},
	"small-1x":  {Name: "small-1x", CPUs: 1, MemoryMB: 512, CostPerSecond: CostSmall1x},
	"small-2x":  {Name: "small-2x", CPUs: 2, MemoryMB: 1024, CostPerSecond: CostSmall2x},
	"medium-1x": {Name: "medium-1x", CPUs: 2, MemoryMB: 4096, CostPerSecond: CostMedium1x},
	"medium-2x": {Name: "medium-2x", CPUs: 4, MemoryMB: 8192, CostPerSecond: CostMedium2x},
	"large-1x":  {Name: "large-1x", CPUs: 8, MemoryMB: 16384, CostPerSecond: CostLarge1x},
	"large-2x":  {Name: "large-2x", CPUs: 16, MemoryMB: 32768, CostPerSecond: CostLarge2x},
}

// PresetOrder defines the canonical ordering of presets from smallest to largest.
var PresetOrder = []string{"micro", "small-1x", "small-2x", "medium-1x", "medium-2x", "large-1x", "large-2x"}

// PresetFromName returns the preset definition for a given name.
// Returns the zero value and false if the preset is unknown.
func PresetFromName(name string) (Preset, bool) {
	p, ok := AllPresets[name]
	return p, ok
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
