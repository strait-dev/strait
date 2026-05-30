package domain

import "testing"

// TestContinueVersionStrategyNormalize verifies that the empty value resolves to
// the deterministic default (repin) and that explicit values pass through.
func TestContinueVersionStrategyNormalize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   ContinueVersionStrategy
		want ContinueVersionStrategy
	}{
		{in: "", want: ContinueVersionRepin},
		{in: ContinueVersionRepin, want: ContinueVersionRepin},
		{in: ContinueVersionLatest, want: ContinueVersionLatest},
	}
	for _, tc := range cases {
		if got := tc.in.Normalize(); got != tc.want {
			t.Fatalf("ContinueVersionStrategy(%q).Normalize() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestContinueVersionStrategyIsValid verifies that only the empty default and the
// two named strategies are accepted; anything else is rejected.
func TestContinueVersionStrategyIsValid(t *testing.T) {
	t.Parallel()
	valid := []ContinueVersionStrategy{"", ContinueVersionRepin, ContinueVersionLatest}
	for _, s := range valid {
		if !s.IsValid() {
			t.Fatalf("ContinueVersionStrategy(%q).IsValid() = false, want true", s)
		}
	}
	invalid := []ContinueVersionStrategy{"bogus", "REPIN", "Latest", "newest", " repin", "repin "}
	for _, s := range invalid {
		if s.IsValid() {
			t.Fatalf("ContinueVersionStrategy(%q).IsValid() = true, want false", s)
		}
	}
}
