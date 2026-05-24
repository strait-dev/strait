package eventfilter

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
)

// valueEquals must produce the same result as the original
// fmt.Sprintf("%v", val) == target comparison for every value type
// json.Unmarshal yields. These tests pin that equivalence.

func TestValueEquals_MatchesSprintf(t *testing.T) {
	vals := []any{
		nil,
		"",
		"deploy",
		"<nil>",
		"true",
		"42",
		true,
		false,
		float64(0),
		float64(42),
		float64(-7),
		float64(1.5),
		float64(0.0001),
		float64(1e20),
		float64(-1e-20),
		math.MaxFloat64,
		math.SmallestNonzeroFloat64,
		map[string]any{"name": "alice", "role": "admin"},
		[]any{float64(1), "two", true},
	}
	for _, val := range vals {
		want := fmt.Sprintf("%v", val)
		// Equal case: target is the canonical rendering.
		if !valueEquals(val, want) {
			t.Errorf("valueEquals(%#v, %q) = false, want true", val, want)
		}
		// Unequal case: a target that cannot match the rendering.
		if valueEquals(val, want+"_x") {
			t.Errorf("valueEquals(%#v, %q) = true, want false", val, want+"_x")
		}
	}
}

// FuzzValueEquals asserts valueEquals stays byte-identical to the fmt.Sprintf
// comparison across arbitrary scalar values and targets parsed from JSON, the
// only source of values Eval ever compares.
func FuzzValueEquals(f *testing.F) {
	f.Add(`"hello"`, "hello")
	f.Add(`42`, "42")
	f.Add(`42.5`, "42.5")
	f.Add(`true`, "true")
	f.Add(`null`, "<nil>")
	f.Add(`1e30`, "1e+30")
	f.Add(`{"a":1}`, "map[a:1]")
	f.Add(`[1,2]`, "[1 2]")

	f.Fuzz(func(t *testing.T, raw, target string) {
		var val any
		if err := json.Unmarshal([]byte(raw), &val); err != nil {
			return // only values that survive json.Unmarshal reach Eval
		}
		want := fmt.Sprintf("%v", val) == target
		got := valueEquals(val, target)
		if got != want {
			t.Fatalf("valueEquals(%#v, %q) = %v, want %v (sprintf=%q)", val, target, got, want, fmt.Sprintf("%v", val))
		}
	})
}
