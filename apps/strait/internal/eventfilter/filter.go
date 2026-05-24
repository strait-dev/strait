package eventfilter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
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
	return EvalParsed(filterExpr, NewParsedPayload(payload))
}

// ParsedPayload holds an event payload so it can be evaluated against many
// filter expressions while parsing it at most once. It decodes lazily on first
// use and caches the result (and any decode error), so a payload is never
// parsed for an expression that matches everything (empty filter). Reusing one
// ParsedPayload across a source's subscriptions turns N payload parses per
// event into one. A ParsedPayload is not safe for concurrent use.
type ParsedPayload struct {
	raw    json.RawMessage
	data   map[string]any
	err    error
	parsed bool
}

// NewParsedPayload wraps a raw event payload for evaluation. The payload is not
// decoded until the first non-empty filter expression needs it.
func NewParsedPayload(payload json.RawMessage) *ParsedPayload {
	return &ParsedPayload{raw: payload}
}

// decode unmarshals the payload on first call and caches the outcome. The
// payload size limit is enforced by EvalParsed to preserve the original check
// ordering, so this performs only the unmarshal.
func (p *ParsedPayload) decode() (map[string]any, error) {
	if !p.parsed {
		p.parsed = true
		p.err = json.Unmarshal(p.raw, &p.data)
	}
	return p.data, p.err
}

// EvalParsed evaluates a filter expression against an already-wrapped payload.
// Behavior matches Eval; passing the same ParsedPayload to repeated calls
// avoids re-parsing the payload for each filter expression.
func EvalParsed(filterExpr json.RawMessage, payload *ParsedPayload) (bool, error) {
	if len(filterExpr) == 0 || string(filterExpr) == "null" {
		return true, nil
	}
	if len(filterExpr) > maxFilterBytes {
		return false, fmt.Errorf("filter expression exceeds %d bytes", maxFilterBytes)
	}
	if len(payload.raw) > maxPayloadBytes {
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

	data, err := payload.decode()
	if err != nil {
		return false, fmt.Errorf("invalid payload: %w", err)
	}

	// Evaluate eq conditions.
	for _, cond := range expr.Eq {
		val := getField(data, cond[0])
		if !valueEquals(val, cond[1]) {
			return false, nil
		}
	}

	// Evaluate ne conditions.
	for _, cond := range expr.Ne {
		val := getField(data, cond[0])
		if valueEquals(val, cond[1]) {
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

// valueEquals reports whether val's fmt "%v" representation equals target.
// It avoids the reflection and allocation of fmt.Sprintf for the scalar types
// json.Unmarshal produces (nil, string, bool, float64), matching that output
// byte-for-byte, and falls back to fmt.Sprintf only for composite values
// (objects and arrays) where a filter path resolves to a non-scalar.
func valueEquals(val any, target string) bool {
	switch v := val.(type) {
	case nil:
		return target == "<nil>"
	case string:
		return v == target
	case bool:
		if v {
			return target == "true"
		}
		return target == "false"
	case float64:
		var buf [32]byte
		b := strconv.AppendFloat(buf[:0], v, 'g', -1, 64)
		return string(b) == target
	default:
		return fmt.Sprintf("%v", v) == target
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
	if strings.Count(path, ".")+1 > maxFilterPathDepth {
		return fmt.Errorf("filter path exceeds %d segments", maxFilterPathDepth)
	}
	return nil
}

// getField traverses a nested map by dot-separated path. It walks the path by
// byte index rather than strings.Split so a lookup allocates nothing.
func getField(data map[string]any, path string) any {
	var current any = data
	for start := 0; ; {
		dot := strings.IndexByte(path[start:], '.')
		var part string
		if dot < 0 {
			part = path[start:]
		} else {
			part = path[start : start+dot]
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		next, ok := m[part]
		if !ok {
			return nil
		}
		if dot < 0 {
			return next
		}
		current = next
		start += dot + 1
	}
}
