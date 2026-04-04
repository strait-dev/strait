package compute

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Per-second compute costs in micro-USD ($0.000001) for each machine preset.
// These are what we CHARGE users (user-facing price).
const (
	CostMicro    int64 = 17   // 1 vCPU / 256 MB  — $0.061/hr
	CostSmall1x  int64 = 34   // 1 vCPU / 512 MB  — $0.122/hr
	CostSmall2x  int64 = 68   // 2 vCPU / 1 GB    — $0.245/hr
	CostMedium1x int64 = 130  // 2 vCPU / 4 GB    — $0.468/hr
	CostMedium2x int64 = 260  // 4 vCPU / 8 GB    — $0.936/hr
	CostLarge1x  int64 = 525  // 8 vCPU / 16 GB   — $1.890/hr
	CostLarge2x  int64 = 1050 // 16 vCPU / 32 GB  — $3.780/hr
)

// NodePoolLabel is the K8s node label used for pool-based scheduling.
const NodePoolLabel = "strait.dev/pool"

// NodePoolAffinityWeight controls how strongly jobs prefer their designated pool.
// 80 = strong preference but not absolute. Jobs overflow to other pools if needed.
const NodePoolAffinityWeight = 80

// Node pool names for K8s scheduling affinity.
const (
	NodePoolGeneral     = "general"     // micro, small-1x, small-2x (Hetzner CAX21: 4 vCPU, 8GB)
	NodePoolPerformance = "performance" // medium-1x, medium-2x (Hetzner CAX31: 8 vCPU, 16GB)
	NodePoolHeavy       = "heavy"       // large-1x, large-2x (Hetzner CAX41: 16 vCPU, 32GB)
)

// Preset defines the resource allocation for a machine preset.
type Preset struct {
	Name             string // e.g. "micro", "small-1x"
	CPUs             int    // Number of vCPUs
	MemoryMB         int    // Memory in megabytes
	CostPerSecond    int64  // User-facing cost in micro-USD per second (what we charge).
	K8sCostPerSecond int64  // Infrastructure cost in micro-USD per second (internal margin tracking).
	NodePool         string // K8s node pool for affinity scheduling (general, performance, heavy).
}

// InternalCostPerSecond returns the infrastructure cost based on the runtime.
// When K8sCostPerSecond is configured (> 0) and runtime is "k8s", returns the
// K8s-specific cost. Otherwise returns the user-facing cost (CostPerSecond).
// K8s costs are loaded from Doppler at runtime, not hardcoded in source.
func (p Preset) InternalCostPerSecond(runtime string) int64 {
	if runtime == "k8s" && p.K8sCostPerSecond > 0 {
		return p.K8sCostPerSecond
	}
	return p.CostPerSecond
}

// SetK8sCosts overrides K8s infrastructure costs for internal margin tracking.
// Called at startup with values from Doppler/config. Not hardcoded in source.
func SetK8sCosts(costs map[string]int64) {
	for name, cost := range costs {
		if p, ok := AllPresets[name]; ok {
			p.K8sCostPerSecond = cost
			AllPresets[name] = p
		}
	}
}

// K8sCostPerSecond defaults to 0 (same as user-facing cost). Override via
// config at runtime for internal margin tracking (values stored in Doppler,
// not in source code).
var AllPresets = map[string]Preset{
	"micro":     {Name: "micro", CPUs: 1, MemoryMB: 256, CostPerSecond: CostMicro, NodePool: NodePoolGeneral},
	"small-1x":  {Name: "small-1x", CPUs: 1, MemoryMB: 512, CostPerSecond: CostSmall1x, NodePool: NodePoolGeneral},
	"small-2x":  {Name: "small-2x", CPUs: 2, MemoryMB: 1024, CostPerSecond: CostSmall2x, NodePool: NodePoolGeneral},
	"medium-1x": {Name: "medium-1x", CPUs: 2, MemoryMB: 4096, CostPerSecond: CostMedium1x, NodePool: NodePoolPerformance},
	"medium-2x": {Name: "medium-2x", CPUs: 4, MemoryMB: 8192, CostPerSecond: CostMedium2x, NodePool: NodePoolPerformance},
	"large-1x":  {Name: "large-1x", CPUs: 8, MemoryMB: 16384, CostPerSecond: CostLarge1x, NodePool: NodePoolHeavy},
	"large-2x":  {Name: "large-2x", CPUs: 16, MemoryMB: 32768, CostPerSecond: CostLarge2x, NodePool: NodePoolHeavy},
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

// K8sResources returns Kubernetes resource requests and limits for this preset.
// Micro/small presets use burstable QoS (CPU request < limit) to match Fly's shared-cpu behavior.
// Medium/large presets use guaranteed QoS (request = limit) to match Fly's performance tier.
func (p Preset) K8sResources() (requests, limits corev1.ResourceList) {
	memQuantity := resource.MustParse(fmt.Sprintf("%dMi", p.MemoryMB))

	if p.CPUs < 2 || p.MemoryMB < 4096 {
		// Burstable: low CPU request, high limit (like Fly shared-cpu).
		cpuRequest := resource.MustParse(fmt.Sprintf("%dm", max(p.CPUs*100, 100)))
		cpuLimit := resource.MustParse(fmt.Sprintf("%dm", p.CPUs*1000))
		requests = corev1.ResourceList{
			corev1.ResourceCPU:    cpuRequest,
			corev1.ResourceMemory: memQuantity,
		}
		limits = corev1.ResourceList{
			corev1.ResourceCPU:    cpuLimit,
			corev1.ResourceMemory: memQuantity,
		}
	} else {
		// Guaranteed: request = limit (like Fly performance tier).
		cpuQuantity := resource.MustParse(fmt.Sprintf("%dm", p.CPUs*1000))
		requests = corev1.ResourceList{
			corev1.ResourceCPU:    cpuQuantity,
			corev1.ResourceMemory: memQuantity,
		}
		limits = corev1.ResourceList{
			corev1.ResourceCPU:    cpuQuantity,
			corev1.ResourceMemory: memQuantity,
		}
	}
	return requests, limits
}

// K8sNodeAffinity returns a soft node affinity targeting this preset's node pool.
// Uses PreferredDuringScheduling so jobs prefer their pool but can overflow to any node.
// Returns nil if the preset has no node pool configured.
func (p Preset) K8sNodeAffinity() *corev1.Affinity {
	if p.NodePool == "" {
		return nil
	}
	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
				{
					Weight: NodePoolAffinityWeight,
					Preference: corev1.NodeSelectorTerm{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      NodePoolLabel,
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{p.NodePool},
							},
						},
					},
				},
			},
		},
	}
}
