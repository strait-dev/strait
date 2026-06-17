package eventfilter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	maxFilterBytes      = 65_536
	maxPayloadBytes     = 1_048_576
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
		if filterValueString(val) != cond[1] {
			return false, nil
		}
	}

	// Evaluate ne conditions.
	for _, cond := range expr.Ne {
		val := getField(data, cond[0])
		if filterValueString(val) == cond[1] {
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

func filterValueString(val any) string {
	switch v := val.(type) {
	case nil:
		return "<nil>"
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return fmt.Sprint(v)
	}
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
	if pathSegmentCount(path) > maxFilterPathDepth {
		return fmt.Errorf("filter path exceeds %d segments", maxFilterPathDepth)
	}
	return nil
}

func pathSegmentCount(path string) int {
	segments := 1
	for i := range path {
		if path[i] == '.' {
			segments++
		}
	}
	return segments
}

// getField traverses a nested map by dot-separated path.
func getField(data map[string]any, path string) any {
	var current any = data
	for {
		part, rest, found := strings.Cut(path, ".")
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = m[part]
		if !ok {
			return nil
		}
		if !found {
			return current
		}
		path = rest
	}
}
