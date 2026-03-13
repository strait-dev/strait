package eventfilter

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FilterExpr defines filter conditions for event payloads.
type FilterExpr struct {
	Eq  [][2]string `json:"eq,omitempty"`
	Ne  [][2]string `json:"ne,omitempty"`
	Has []string    `json:"has,omitempty"`
}

// Eval evaluates a filter expression against a JSON payload.
// Returns true when all conditions pass (AND semantics).
// Empty/nil filter expression matches everything.
func Eval(filterExpr json.RawMessage, payload json.RawMessage) (bool, error) {
	if len(filterExpr) == 0 || string(filterExpr) == "null" {
		return true, nil
	}

	var expr FilterExpr
	if err := json.Unmarshal(filterExpr, &expr); err != nil {
		return false, fmt.Errorf("invalid filter expression: %w", err)
	}

	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return false, fmt.Errorf("invalid payload: %w", err)
	}

	// Evaluate eq conditions.
	for _, cond := range expr.Eq {
		val := getField(data, cond[0])
		if fmt.Sprintf("%v", val) != cond[1] {
			return false, nil
		}
	}

	// Evaluate ne conditions.
	for _, cond := range expr.Ne {
		val := getField(data, cond[0])
		if fmt.Sprintf("%v", val) == cond[1] {
			return false, nil
		}
	}

	// Evaluate has conditions.
	for _, field := range expr.Has {
		if getField(data, field) == nil {
			return false, nil
		}
	}

	return true, nil
}

// getField traverses a nested map by dot-separated path.
func getField(data map[string]any, path string) any {
	parts := strings.Split(path, ".")
	var current any = data
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = m[part]
		if !ok {
			return nil
		}
	}
	return current
}
