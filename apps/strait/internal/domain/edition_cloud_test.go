//go:build cloud

package domain

import "testing"

func TestParseEdition_Cloud(t *testing.T) {
	tests := []struct {
		input string
		want  Edition
	}{
		{"community", EditionCommunity},
		{"cloud", EditionCloud},
		{"", EditionCommunity},
		{"unknown", EditionCommunity},
		{"Cloud", EditionCommunity},
		{"CLOUD", EditionCommunity},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseEdition(tt.input)
			if got != tt.want {
				t.Errorf("ParseEdition(%q) = %q, want %q (cloud build)", tt.input, got, tt.want)
			}
		})
	}
}
