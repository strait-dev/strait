package compute

import (
	"testing"
)

func TestNearestRegion_DirectCode(t *testing.T) {
	t.Parallel()
	tests := []string{"iad", "lhr", "nrt", "syd", "gru", "lax"}
	for _, code := range tests {
		if got := NearestRegion(code); got != code {
			t.Errorf("NearestRegion(%q) = %q, want %q", code, got, code)
		}
	}
}

func TestNearestRegion_ContinentHints(t *testing.T) {
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
		if got := NearestRegion(hint); got != want {
			t.Errorf("NearestRegion(%q) = %q, want %q", hint, got, want)
		}
	}
}

func TestNearestRegion_Unknown(t *testing.T) {
	t.Parallel()
	if got := NearestRegion("mars"); got != "" {
		t.Errorf("NearestRegion(mars) = %q, want empty", got)
	}
}

func TestNearestRegion_Empty(t *testing.T) {
	t.Parallel()
	if got := NearestRegion(""); got != "" {
		t.Errorf("NearestRegion('') = %q, want empty", got)
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
	for region := range regions {
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

func TestIsValidRegion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code string
		want bool
	}{
		{"iad", true},
		{"lhr", true},
		{"nrt", true},
		{"syd", true},
		{"fra", true},
		{"", false},
		{"invalid", false},
		{"us-east", false},
		{"IAD", false},
	}

	for _, tt := range tests {
		t.Run(tt.code+"_valid="+boolStr(tt.want), func(t *testing.T) {
			t.Parallel()
			if got := IsValidRegion(tt.code); got != tt.want {
				t.Errorf("IsValidRegion(%q) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestAllRegionCodes(t *testing.T) {
	t.Parallel()

	codes := AllRegionCodes()
	if len(codes) != len(regions) {
		t.Fatalf("AllRegionCodes() returned %d codes, want %d", len(codes), len(regions))
	}

	// Verify sorted order.
	for i := 1; i < len(codes); i++ {
		if codes[i] <= codes[i-1] {
			t.Fatalf("AllRegionCodes() not sorted: %q <= %q at index %d", codes[i], codes[i-1], i)
		}
	}

	// All codes must be valid.
	for _, code := range codes {
		if !IsValidRegion(code) {
			t.Errorf("AllRegionCodes() returned invalid code %q", code)
		}
	}
}

func TestRegionLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code string
		want string
	}{
		{"iad", "Ashburn, Virginia (US)"},
		{"lhr", "London, United Kingdom"},
		{"nrt", "Tokyo, Japan"},
		{"syd", "Sydney, Australia"},
		{"fra", "Frankfurt, Germany"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			t.Parallel()
			if got := RegionLabel(tt.code); got != tt.want {
				t.Errorf("RegionLabel(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestAllRegions(t *testing.T) {
	t.Parallel()

	regions := AllRegions()
	if len(regions) != len(regions) {
		t.Fatalf("AllRegions() returned %d regions, want %d", len(regions), len(regions))
	}

	// Verify sorted by code.
	for i := 1; i < len(regions); i++ {
		if regions[i].Code <= regions[i-1].Code {
			t.Fatalf("AllRegions() not sorted by code: %q <= %q", regions[i].Code, regions[i-1].Code)
		}
	}

	// All entries must have required fields.
	for _, r := range regions {
		if r.Code == "" || r.Label == "" || r.City == "" || r.Country == "" || r.Continent == "" {
			t.Errorf("region %q has empty required field", r.Code)
		}
	}
}

func TestAllRegions_MetadataConsistency(t *testing.T) {
	t.Parallel()

	for code := range regions {
		if _, ok := regionMetadata[code]; !ok {
			t.Errorf("region %q in regions but not in regionMetadata", code)
		}
	}
	for code := range regionMetadata {
		if _, ok := regions[code]; !ok {
			t.Errorf("region %q in regionMetadata but not in regions", code)
		}
	}
}
