//go:build cloud

package domain

import "testing"

func TestParseEdition_Cloud(t *testing.T) {
	// In the cloud build, ParseEdition always returns EditionCloud
	// regardless of input. The edition is baked in at compile time.
	inputs := []string{"community", "cloud", "", "unknown", "Cloud", "CLOUD"}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			got := ParseEdition(input)
			if got != EditionCloud {
				t.Errorf("ParseEdition(%q) = %q, want %q (cloud build always returns cloud)", input, got, EditionCloud)
			}
		})
	}
}
