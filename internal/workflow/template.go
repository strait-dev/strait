package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// templateVarRegex matches {{var_name}} placeholders in strings.
// Variable names may contain word characters (letters, digits, underscores)
// and dot-separated paths (e.g. "user.email").
var templateVarRegex = regexp.MustCompile(`\{\{([a-zA-Z_]\w*(?:\.\w+)*)\}\}`)

// renderTemplateVars replaces {{var_name}} placeholders in JSON string values
// of payload with corresponding values from the variables JSON object.
//
// If a string value is exactly "{{var_name}}", the replacement preserves the
// variable's original JSON type (number, boolean, object, etc.).
// If {{var_name}} is embedded in a larger string, the replacement is stringified.
// Unresolved variables are left as-is.
func renderTemplateVars(payload, variables json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(payload)) == 0 || len(bytes.TrimSpace(variables)) == 0 {
		return payload
	}

	var vars map[string]any
	if err := json.Unmarshal(variables, &vars); err != nil {
		return payload
	}
	if len(vars) == 0 {
		return payload
	}

	var data any
	if err := json.Unmarshal(payload, &data); err != nil {
		return payload
	}

	rendered := walkAndRender(data, vars)

	out, err := json.Marshal(rendered)
	if err != nil {
		return payload
	}
	return out
}

// walkAndRender recursively walks a parsed JSON value and renders template
// variables in any string values encountered.
func walkAndRender(v any, vars map[string]any) any {
	switch val := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, child := range val {
			result[k] = walkAndRender(child, vars)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, child := range val {
			result[i] = walkAndRender(child, vars)
		}
		return result
	case string:
		return renderStringValue(val, vars)
	default:
		return v
	}
}

// renderStringValue handles template variable substitution in a single string.
// If the entire string is a single "{{var_name}}", the variable's native type
// is preserved (e.g. number stays a number). Otherwise, variables are converted
// to their string representation and interpolated.
func renderStringValue(s string, vars map[string]any) any {
	if !strings.Contains(s, "{{") {
		return s
	}

	matches := templateVarRegex.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return s
	}

	// Entire string is a single "{{var_name}}" — preserve the variable's type.
	if len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(s) {
		varName := s[matches[0][2]:matches[0][3]]
		if val, ok := resolveVar(vars, varName); ok {
			return val
		}
		return s
	}

	// Mixed content: build result from parts.
	var buf strings.Builder
	buf.Grow(len(s))
	prev := 0
	for _, m := range matches {
		buf.WriteString(s[prev:m[0]])
		varName := s[m[2]:m[3]]
		if val, ok := resolveVar(vars, varName); ok {
			buf.WriteString(stringify(val))
		} else {
			buf.WriteString(s[m[0]:m[1]])
		}
		prev = m[1]
	}
	buf.WriteString(s[prev:])
	return buf.String()
}

// resolveVar looks up a variable name in the vars map. Supports dot-separated
// paths (e.g. "user.email" resolves vars["user"]["email"]).
func resolveVar(vars map[string]any, name string) (any, bool) {
	parts := strings.Split(name, ".")
	var current any = vars

	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}

	return current, true
}

// renderStringTemplate renders {{var}} placeholders in a plain string (not JSON)
// using a JSON variables object. Returns the rendered string.
func renderStringTemplate(template string, variables json.RawMessage) string {
	if !strings.Contains(template, "{{") {
		return template
	}
	if len(bytes.TrimSpace(variables)) == 0 {
		return template
	}

	var vars map[string]any
	if err := json.Unmarshal(variables, &vars); err != nil {
		return template
	}
	if len(vars) == 0 {
		return template
	}

	return templateVarRegex.ReplaceAllStringFunc(template, func(match string) string {
		varName := templateVarRegex.FindStringSubmatch(match)[1]
		if val, ok := resolveVar(vars, varName); ok {
			return stringify(val)
		}
		return match
	})
}

// stringify converts a value to its string representation for interpolation.
func stringify(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}
