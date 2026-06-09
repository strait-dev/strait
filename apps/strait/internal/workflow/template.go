package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

var templateMarker = []byte("{{")

// renderTemplateVars replaces {{var_name}} placeholders in JSON string values
// of payload with corresponding values from the variables JSON object.
//
// If a string value is exactly "{{var_name}}", the replacement preserves the
// variable's original JSON type (number, boolean, object, etc.).
// If {{var_name}} is embedded in a larger string, the replacement is stringified.
// Unresolved variables are left as-is.
func renderTemplateVars(payload, variables json.RawMessage) json.RawMessage {
	if !bytes.Contains(payload, templateMarker) ||
		len(bytes.TrimSpace(payload)) == 0 ||
		len(bytes.TrimSpace(variables)) == 0 {
		return payload
	}
	if !gjson.ValidBytes(variables) || !payloadHasResolvableTemplateJSON(payload, variables) {
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

	rendered, changed := walkAndRender(data, vars)
	if !changed {
		return payload
	}

	out, err := json.Marshal(rendered)
	if err != nil {
		return payload
	}
	return out
}

func payloadHasResolvableTemplateJSON(payload, variables []byte) bool {
	start := 0
	for start < len(payload) {
		relOpen := bytes.Index(payload[start:], templateMarker)
		if relOpen < 0 {
			return false
		}
		open := start + relOpen
		nameStart := open + len(templateMarker)
		relClose := bytes.Index(payload[nameStart:], []byte("}}"))
		if relClose < 0 {
			return false
		}
		nameEnd := nameStart + relClose
		nameBytes := payload[nameStart:nameEnd]
		if isTemplateVarNameBytes(nameBytes) && gjson.GetBytes(variables, string(nameBytes)).Exists() {
			return true
		}
		start = open + len(templateMarker)
	}
	return false
}

// walkAndRender recursively walks a parsed JSON value and renders template
// variables in any string values encountered.
func walkAndRender(v any, vars map[string]any) (any, bool) {
	switch val := v.(type) {
	case map[string]any:
		changedAny := false
		for k, child := range val {
			rendered, changed := walkAndRender(child, vars)
			if changed {
				val[k] = rendered
				changedAny = true
			}
		}
		return val, changedAny
	case []any:
		changedAny := false
		for i, child := range val {
			rendered, changed := walkAndRender(child, vars)
			if changed {
				val[i] = rendered
				changedAny = true
			}
		}
		return val, changedAny
	case string:
		rendered := renderStringValue(val, vars)
		if renderedString, ok := rendered.(string); ok && renderedString == val {
			return val, false
		}
		return rendered, true
	default:
		return v, false
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

	open, end, varName, ok := nextTemplateVar(s, 0)
	if !ok {
		return s
	}

	// Entire string is a single "{{var_name}}" — preserve the variable's type.
	if open == 0 && end == len(s) {
		if val, ok := resolveVar(vars, varName); ok {
			return val
		}
		return s
	}

	// Mixed content: build result from parts.
	var buf strings.Builder
	buf.Grow(len(s))
	prev := 0
	for {
		buf.WriteString(s[prev:open])
		if val, ok := resolveVar(vars, varName); ok {
			buf.WriteString(stringify(val))
		} else {
			buf.WriteString(s[open:end])
		}
		prev = end

		open, end, varName, ok = nextTemplateVar(s, end)
		if !ok {
			break
		}
	}
	buf.WriteString(s[prev:])
	return buf.String()
}

// resolveVar looks up a variable name in the vars map. Supports dot-separated
// paths (e.g. "user.email" resolves vars["user"]["email"]).
func resolveVar(vars map[string]any, name string) (any, bool) {
	if strings.IndexByte(name, '.') < 0 {
		val, ok := vars[name]
		return val, ok
	}

	var current any = vars
	start := 0
	for start <= len(name) {
		dot := strings.IndexByte(name[start:], '.')
		end := len(name)
		if dot >= 0 {
			end = start + dot
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[name[start:end]]
		if !ok {
			return nil, false
		}
		if dot < 0 {
			break
		}
		start = end + 1
	}

	return current, true
}

func isTemplateVarNameBytes(name []byte) bool {
	if len(name) == 0 || !isTemplateVarStart(name[0]) {
		return false
	}
	previousDot := false
	for i := 1; i < len(name); i++ {
		c := name[i]
		if c == '.' {
			if previousDot || i == len(name)-1 {
				return false
			}
			previousDot = true
			continue
		}
		if previousDot {
			if !isTemplateVarChar(c) {
				return false
			}
			previousDot = false
			continue
		}
		if !isTemplateVarChar(c) {
			return false
		}
	}
	return true
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

	if !gjson.ValidBytes(variables) {
		return template
	}

	open, end, varName, ok := nextTemplateVar(template, 0)
	if !ok {
		return template
	}

	useCache := len(template) > 128
	var cache stringTemplateVarCache
	var buf strings.Builder
	buf.Grow(len(template))
	prev := 0
	for {
		buf.WriteString(template[prev:open])
		if useCache {
			if val, ok := cache.resolve(variables, varName); ok {
				buf.WriteString(val)
			} else {
				buf.WriteString(template[open:end])
			}
		} else if val := gjson.GetBytes(variables, varName); val.Exists() {
			buf.WriteString(stringifyJSONResult(val))
		} else {
			buf.WriteString(template[open:end])
		}
		prev = end

		open, end, varName, ok = nextTemplateVar(template, end)
		if !ok {
			break
		}
	}
	buf.WriteString(template[prev:])
	return buf.String()
}

type stringTemplateVarCache struct {
	names        [8]string
	values       [8]string
	missingNames [8]string
	valueCount   int
	missingCount int
}

func (c *stringTemplateVarCache) resolve(variables []byte, name string) (string, bool) {
	for i := range c.valueCount {
		if c.names[i] == name {
			return c.values[i], true
		}
	}
	for i := range c.missingCount {
		if c.missingNames[i] == name {
			return "", false
		}
	}

	val := gjson.GetBytes(variables, name)
	if !val.Exists() {
		if c.missingCount < len(c.missingNames) {
			c.missingNames[c.missingCount] = name
			c.missingCount++
		}
		return "", false
	}

	str := stringifyJSONResult(val)
	if c.valueCount < len(c.names) {
		c.names[c.valueCount] = name
		c.values[c.valueCount] = str
		c.valueCount++
	}
	return str, true
}

func stringifyJSONResult(v gjson.Result) string {
	switch v.Type {
	case gjson.String:
		return v.Str
	case gjson.Number:
		return v.Raw
	case gjson.True:
		return "true"
	case gjson.False:
		return "false"
	case gjson.JSON:
		return v.Raw
	default:
		return ""
	}
}

func nextTemplateVar(s string, start int) (open int, end int, name string, ok bool) {
	for start < len(s) {
		relOpen := strings.Index(s[start:], "{{")
		if relOpen < 0 {
			return 0, 0, "", false
		}
		open = start + relOpen
		nameStart := open + len("{{")
		relClose := strings.Index(s[nameStart:], "}}")
		if relClose < 0 {
			return 0, 0, "", false
		}
		nameEnd := nameStart + relClose
		end = nameEnd + len("}}")
		name = s[nameStart:nameEnd]
		if isTemplateVarName(name) {
			return open, end, name, true
		}
		start = open + len("{{")
	}
	return 0, 0, "", false
}

func isTemplateVarName(name string) bool {
	if name == "" || !isTemplateVarStart(name[0]) {
		return false
	}
	previousDot := false
	for i := 1; i < len(name); i++ {
		c := name[i]
		if c == '.' {
			if previousDot || i == len(name)-1 {
				return false
			}
			previousDot = true
			continue
		}
		if previousDot {
			if !isTemplateVarChar(c) {
				return false
			}
			previousDot = false
			continue
		}
		if !isTemplateVarChar(c) {
			return false
		}
	}
	return true
}

func isTemplateVarStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isTemplateVarChar(c byte) bool {
	return isTemplateVarStart(c) || (c >= '0' && c <= '9')
}

// stringify converts a value to its string representation for interpolation.
func stringify(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
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
