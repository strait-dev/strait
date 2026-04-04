package compute

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestPreset_NodePool_Mapping(t *testing.T) {
	tests := []struct {
		preset   string
		wantPool string
	}{
		{"micro", NodePoolGeneral},
		{"small-1x", NodePoolGeneral},
		{"small-2x", NodePoolGeneral},
		{"medium-1x", NodePoolPerformance},
		{"medium-2x", NodePoolPerformance},
		{"large-1x", NodePoolHeavy},
		{"large-2x", NodePoolHeavy},
	}

	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			p, err := PresetFromName(tt.preset)
			if err != nil {
				t.Fatalf("PresetFromName(%q): %v", tt.preset, err)
			}
			if p.NodePool != tt.wantPool {
				t.Errorf("preset %q: NodePool=%q, want %q", tt.preset, p.NodePool, tt.wantPool)
			}
		})
	}
}

func TestPreset_K8sNodeAffinity_General(t *testing.T) {
	p := AllPresets["micro"]
	affinity := p.K8sNodeAffinity()
	if affinity == nil {
		t.Fatal("K8sNodeAffinity() returned nil for micro")
	}

	terms := affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution
	if len(terms) != 1 {
		t.Fatalf("expected 1 preferred term, got %d", len(terms))
	}
	if terms[0].Preference.MatchExpressions[0].Values[0] != NodePoolGeneral {
		t.Errorf("pool=%q, want %q", terms[0].Preference.MatchExpressions[0].Values[0], NodePoolGeneral)
	}
}

func TestPreset_K8sNodeAffinity_Performance(t *testing.T) {
	p := AllPresets["medium-1x"]
	affinity := p.K8sNodeAffinity()
	if affinity == nil {
		t.Fatal("nil affinity for medium-1x")
	}

	pool := affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0].Preference.MatchExpressions[0].Values[0]
	if pool != NodePoolPerformance {
		t.Errorf("pool=%q, want %q", pool, NodePoolPerformance)
	}
}

func TestPreset_K8sNodeAffinity_Heavy(t *testing.T) {
	p := AllPresets["large-1x"]
	affinity := p.K8sNodeAffinity()
	if affinity == nil {
		t.Fatal("nil affinity for large-1x")
	}

	pool := affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0].Preference.MatchExpressions[0].Values[0]
	if pool != NodePoolHeavy {
		t.Errorf("pool=%q, want %q", pool, NodePoolHeavy)
	}
}

func TestPreset_K8sNodeAffinity_EmptyPool(t *testing.T) {
	p := Preset{Name: "custom", CPUs: 1, MemoryMB: 256, NodePool: ""}
	affinity := p.K8sNodeAffinity()
	if affinity != nil {
		t.Error("expected nil affinity for empty NodePool")
	}
}

func TestPreset_K8sNodeAffinity_Weight(t *testing.T) {
	p := AllPresets["micro"]
	affinity := p.K8sNodeAffinity()
	weight := affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0].Weight
	if weight != NodePoolAffinityWeight {
		t.Errorf("weight=%d, want %d", weight, NodePoolAffinityWeight)
	}
}

func TestPreset_K8sNodeAffinity_Label(t *testing.T) {
	p := AllPresets["micro"]
	affinity := p.K8sNodeAffinity()
	key := affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0].Preference.MatchExpressions[0].Key
	if key != NodePoolLabel {
		t.Errorf("label key=%q, want %q", key, NodePoolLabel)
	}
}

func TestPreset_K8sNodeAffinity_Operator(t *testing.T) {
	p := AllPresets["micro"]
	affinity := p.K8sNodeAffinity()
	op := affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0].Preference.MatchExpressions[0].Operator
	if op != corev1.NodeSelectorOpIn {
		t.Errorf("operator=%q, want In", op)
	}
}
