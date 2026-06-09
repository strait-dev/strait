package workflow

import (
	"encoding/json"
	"fmt"

	"github.com/tidwall/gjson"
)

// ApplyOutputTransform transforms a step output using a JSONPath expression (gjson syntax).
// If the transform is empty, returns the original output unchanged.
// If the transform extracts a value, returns it as a JSON-encoded value.
// If the path is not found, returns an error.
func ApplyOutputTransform(rawOutput json.RawMessage, transformPath string) (json.RawMessage, error) {
	if transformPath == "" {
		return rawOutput, nil
	}

	matched := gjson.GetBytes(rawOutput, transformPath)
	if !matched.Exists() {
		if !gjson.ValidBytes(rawOutput) {
			return nil, fmt.Errorf("output transform: source output is empty or invalid JSON")
		}
		return nil, fmt.Errorf("output transform: path %q not found in output", transformPath)
	}

	raw := matched.Raw
	if raw == "" {
		return nil, fmt.Errorf("output transform: path %q did not extract a value", transformPath)
	}

	if matched.Index > 0 && matched.Index+len(raw) <= len(rawOutput) {
		return rawOutput[matched.Index : matched.Index+len(raw)], nil
	}

	return json.RawMessage(raw), nil
}
