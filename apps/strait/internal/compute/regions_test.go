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
