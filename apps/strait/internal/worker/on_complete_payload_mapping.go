package worker

import (
	"encoding/json"
	"fmt"
)

// applyPayloadMapping extracts fields from result using the mapping definition.
// The mapping is a JSON object where keys are output field names and values
// are dot-notation paths into the result.
func applyPayloadMapping(result json.RawMessage, mapping json.RawMessage) (json.RawMessage, error) {
	if len(result) == 0 || len(mapping) == 0 {
		return result, nil
	}

	var pathMap map[string]string
	if err := json.Unmarshal(mapping, &pathMap); err != nil {
		return nil, fmt.Errorf("unmarshal payload mapping: %w", err)
	}

	var resultData map[string]any
	if unmarshalErr := json.Unmarshal(result, &resultData); unmarshalErr != nil {
		// If result isn't a JSON object, return as-is.
		return result, nil //nolint:nilerr // intentional: non-object results pass through unchanged
	}

	output := make(map[string]any, len(pathMap))
	for key, path := range pathMap {
		val := extractPath(resultData, path)
		if val != nil {
			output[key] = val
		}
	}

	mapped, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("marshal mapped payload: %w", err)
	}
	return mapped, nil
}

// extractPath extracts a value from a nested map using dot-notation.
func extractPath(data map[string]any, path string) any {
	current := any(data)
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '.' {
			key := path[start:i]
			start = i + 1

			m, ok := current.(map[string]any)
			if !ok {
				return nil
			}
			current = m[key]
		}
	}
	return current
}
