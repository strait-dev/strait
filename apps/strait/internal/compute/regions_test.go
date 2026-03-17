package compute

import "testing"

func TestNearestFlyRegion_DirectCode(t *testing.T) {
	t.Parallel()
	tests := []string{"iad", "lhr", "nrt", "syd", "gru", "lax"}
	for _, code := range tests {
		if got := NearestFlyRegion(code); got != code {
			t.Errorf("NearestFlyRegion(%q) = %q, want %q", code, got, code)
		}
	}
}

func TestNearestFlyRegion_ContinentHints(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"us-east": "iad",
		"us-west": "lax",
		"eu":      "lhr",
		"europe":  "lhr",
		"asia":    "nrt",
		"oceania": "syd",
		"sa":      "gru",
		"africa":  "jnb",
	}
	for hint, want := range tests {
		if got := NearestFlyRegion(hint); got != want {
			t.Errorf("NearestFlyRegion(%q) = %q, want %q", hint, got, want)
		}
	}
}

func TestNearestFlyRegion_Unknown(t *testing.T) {
	t.Parallel()
	if got := NearestFlyRegion("mars"); got != "" {
		t.Errorf("NearestFlyRegion(mars) = %q, want empty", got)
	}
}

func TestNearestFlyRegion_Empty(t *testing.T) {
	t.Parallel()
	if got := NearestFlyRegion(""); got != "" {
		t.Errorf("NearestFlyRegion('') = %q, want empty", got)
	}
}

func TestRegionFallbackChain_KnownRegions(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"iad": {"ewr", "ord", "atl"},
		"lhr": {"cdg", "fra", "ams"},
		"nrt": {"hkg", "sin", "icn"},
		"syd": {"sin", "nrt", "hkg"},
	}
	for region, want := range tests {
		got := RegionFallbackChain(region)
		if len(got) != len(want) {
			t.Errorf("RegionFallbackChain(%q) = %v, want %v", region, got, want)
			continue
		}
		for i, r := range got {
			if r != want[i] {
				t.Errorf("RegionFallbackChain(%q)[%d] = %q, want %q", region, i, r, want[i])
			}
		}
	}
}

func TestRegionFallbackChain_Unknown(t *testing.T) {
	t.Parallel()
	if got := RegionFallbackChain("unknown"); got != nil {
		t.Errorf("RegionFallbackChain(unknown) = %v, want nil", got)
	}
}

func TestRegionFallbackChain_AllKnownRegionsHaveChains(t *testing.T) {
	t.Parallel()
	for region := range flyRegions {
		chain := RegionFallbackChain(region)
		if len(chain) == 0 {
			t.Errorf("region %q has no fallback chain", region)
		}
	}
}

func TestRegionFallbackChain_NoDuplicates(t *testing.T) {
	t.Parallel()
	for region := range regionFallbacks {
		chain := RegionFallbackChain(region)
		seen := map[string]bool{region: true} // Primary shouldn't appear in chain either.
		for _, r := range chain {
			if seen[r] {
				t.Errorf("region %q: duplicate %q in fallback chain", region, r)
			}
			seen[r] = true
		}
	}
}
