package main

import (
	"testing"
)

func TestGenerateSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"My Job Name", "my-job-name"},
		{"  spaces  ", "spaces"},
		{"UPPER-CASE", "upper-case"},
		{"special!@#chars", "specialchars"},
		{"multiple---hyphens", "multiple-hyphens"},
		{"trailing-", "trailing"},
		{"123-numbers", "123-numbers"},
		{"", ""},
		{"already-slug", "already-slug"},
	}

	for _, tc := range tests {
		t.Run("input="+tc.input, func(t *testing.T) {
			t.Parallel()
			got := generateSlug(tc.input)
			if got != tc.want {
				t.Fatalf("generateSlug(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
