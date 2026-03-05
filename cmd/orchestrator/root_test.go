package main

import "testing"

func TestNormalizeLegacyArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "subcommand passthrough", in: []string{"version"}, want: []string{"version"}},
		{name: "legacy mode", in: []string{"--mode", "all"}, want: []string{"serve", "--mode", "all"}},
		{name: "legacy first flag", in: []string{"--verbose"}, want: []string{"serve", "--verbose"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeLegacyArgs(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len(got)=%d len(want)=%d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("arg[%d]=%q want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
