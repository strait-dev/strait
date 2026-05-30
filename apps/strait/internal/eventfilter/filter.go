package eventfilter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	maxFilterBytes      = 64 * 1024
	maxPayloadBytes     = 1024 * 1024
	maxFilterConditions = 256
	maxFilterPathDepth  = 32
	maxFilterPathLength = 512
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
	if len(filterExpr) > maxFilterBytes {
		return false, fmt.Errorf("filter expression exceeds %d bytes", maxFilterBytes)
	}
	if len(payload) > maxPayloadBytes {
		return false, fmt.Errorf("payload exceeds %d bytes", maxPayloadBytes)
	}

	var expr FilterExpr
	decoder := json.NewDecoder(bytes.NewReader(filterExpr))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&expr); err != nil {
		return false, fmt.Errorf("invalid filter expression: %w", err)
	}
	if err := validateFilterExpr(expr); err != nil {
		return false, err
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

func validateFilterExpr(expr FilterExpr) error {
	if len(expr.Eq)+len(expr.Ne)+len(expr.Has) > maxFilterConditions {
		return fmt.Errorf("filter expression has too many conditions; max %d", maxFilterConditions)
	}
	for _, cond := range expr.Eq {
		if err := validatePath(cond[0]); err != nil {
			return err
		}
	}
	for _, cond := range expr.Ne {
		if err := validatePath(cond[0]); err != nil {
			return err
		}
	}
	for _, path := range expr.Has {
		if err := validatePath(path); err != nil {
			return err
		}
	}
	return nil
}

func validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("filter path is required")
	}
	if len(path) > maxFilterPathLength {
		return fmt.Errorf("filter path exceeds %d bytes", maxFilterPathLength)
	}
	if strings.Count(path, ".")+1 > maxFilterPathDepth {
		return fmt.Errorf("filter path exceeds %d segments", maxFilterPathDepth)
	}
	return nil
}

// getField traverses a nested map by dot-separated path. It walks the path in
// place via strings.IndexByte so the common lookup does not allocate a slice of
// segments; segmentation matches strings.Split(path, ".") for every input,
// including leading, trailing, and consecutive dots.
func getField(data map[string]any, path string) any {
	var current any = data
	for {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		i := strings.IndexByte(path, '.')
		if i < 0 {
			return m[path]
		}
		next, ok := m[path[:i]]
		if !ok {
			return nil
		}
		current = next
		path = path[i+1:]
	}
}
