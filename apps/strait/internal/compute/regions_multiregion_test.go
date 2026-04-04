package compute

import (
	"strings"
	"testing"
)

func TestRegion_AllCodesAreValid(t *testing.T) {
	t.Parallel()
	for _, code := range AllRegionCodes() {
		if !IsValidRegion(code) {
			t.Errorf("region code %q from AllRegionCodes is not valid", code)
		}
	}
}

func TestRegion_FallbackChains_NoDuplicates(t *testing.T) {
	t.Parallel()
	for _, code := range AllRegionCodes() {
		chain := RegionFallbackChain(code)
		seen := make(map[string]bool)
		for _, r := range chain {
			if seen[r] {
				t.Errorf("region %q fallback chain has duplicate: %q", code, r)
			}
			seen[r] = true
		}
		// Primary should not appear in its own fallback chain.
		if seen[code] {
			t.Errorf("region %q appears in its own fallback chain", code)
		}
	}
}

func TestRegion_FallbackChains_AllExist(t *testing.T) {
	t.Parallel()
	for _, code := range AllRegionCodes() {
		chain := RegionFallbackChain(code)
		for _, fallback := range chain {
			if !IsValidRegion(fallback) {
				t.Errorf("region %q fallback %q is not a valid region", code, fallback)
			}
		}
	}
}

func TestRegion_NearestRegion_AllContinentHints(t *testing.T) {
	t.Parallel()
	continents := []string{"us-east", "us-west", "eu", "asia", "sa", "africa", "oceania"}
	for _, hint := range continents {
		result := NearestRegion(hint)
		if result == "" {
			t.Errorf("NearestRegion(%q) returned empty", hint)
		}
		if !IsValidRegion(result) {
			t.Errorf("NearestRegion(%q) returned invalid region %q", hint, result)
		}
	}
}

func TestRegion_NearestRegion_DirectCodePassthrough(t *testing.T) {
	t.Parallel()
	for _, code := range AllRegionCodes() {
		result := NearestRegion(code)
		if result != code {
			t.Errorf("NearestRegion(%q) = %q, want passthrough", code, result)
		}
	}
}

func TestRegion_NearestRegion_EmptyAndInvalid(t *testing.T) {
	t.Parallel()
	if result := NearestRegion(""); result != "" {
		t.Errorf("NearestRegion(\"\") = %q, want empty", result)
	}
	if result := NearestRegion("mars"); result != "" {
		t.Errorf("NearestRegion(\"mars\") = %q, want empty", result)
	}
}

func TestRegion_RegionLabel_AllCodesHaveLabels(t *testing.T) {
	t.Parallel()
	for _, code := range AllRegionCodes() {
		label := RegionLabel(code)
		if label == "" {
			t.Errorf("region %q has empty label", code)
		}
		if !strings.Contains(label, ",") && !strings.Contains(label, " ") {
			// Labels should be human-readable like "Ashburn, Virginia"
			t.Logf("region %q label %q might not be descriptive enough", code, label)
		}
	}
}

func TestRegion_AllRegions_MetadataComplete(t *testing.T) {
	t.Parallel()
	allRegions := AllRegions()
	if len(allRegions) == 0 {
		t.Fatal("AllRegions() returned empty")
	}
	codes := AllRegionCodes()
	if len(allRegions) != len(codes) {
		t.Errorf("AllRegions() returned %d regions, AllRegionCodes() returned %d", len(allRegions), len(codes))
	}
}
