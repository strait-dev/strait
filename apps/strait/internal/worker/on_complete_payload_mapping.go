package worker

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/tidwall/gjson"
)

// applyPayloadMapping extracts fields from result using the mapping definition.
// The mapping is a JSON object where keys are output field names and values
// are dot-notation paths into the result.
func applyPayloadMapping(result json.RawMessage, mapping json.RawMessage) (json.RawMessage, error) {
	if len(result) == 0 || len(mapping) == 0 {
		return result, nil
	}

	paths, hasEmptyPath, err := parsePayloadMapping(mapping)
	if err != nil {
		return nil, err
	}
	if firstNonSpaceJSONByte(result) != '{' || !gjson.ValidBytes(result) {
		return result, nil
	}
	if hasEmptyPath {
		return applyPayloadMappingViaMaps(result, payloadMappingPathsToMap(paths))
	}

	outputFields := make([]mappedPayloadField, 0, len(paths))
	for i := range paths {
		val := gjson.GetBytes(result, paths[i].path)
		if val.Exists() && val.Type != gjson.Null {
			outputFields = append(outputFields, mappedPayloadField{key: paths[i].key, value: val})
		}
	}
	sort.Slice(outputFields, func(i, j int) bool {
		return outputFields[i].key < outputFields[j].key
	})

	out := make([]byte, 0, len(mapping)+len(outputFields)*8)
	out = append(out, '{')
	for i, field := range outputFields {
		if i > 0 {
			out = append(out, ',')
		}
		out = strconv.AppendQuote(out, field.key)
		out = append(out, ':')
		if field.value.Raw != "" {
			out = append(out, field.value.Raw...)
			continue
		}
		raw, err := json.Marshal(field.value.Value())
		if err != nil {
			return nil, fmt.Errorf("marshal mapped payload field: %w", err)
		}
		out = append(out, raw...)
	}
	out = append(out, '}')
	return out, nil
}

type payloadMappingPath struct {
	key  string
	path string
}

func parsePayloadMapping(mapping json.RawMessage) ([]payloadMappingPath, bool, error) {
	if firstNonSpaceJSONByte(mapping) != '{' || !gjson.ValidBytes(mapping) {
		return nil, false, fmt.Errorf("unmarshal payload mapping: invalid JSON object")
	}

	var paths []payloadMappingPath
	hasEmptyPath := false
	var parseErr error
	gjson.ParseBytes(mapping).ForEach(func(key, value gjson.Result) bool {
		if value.Type != gjson.String {
			parseErr = fmt.Errorf("unmarshal payload mapping: expected string path for %q", key.String())
			return false
		}
		path := value.String()
		if path == "" {
			hasEmptyPath = true
		}
		field := payloadMappingPath{key: key.String(), path: path}
		for i := range paths {
			if paths[i].key == field.key {
				paths[i] = field
				return true
			}
		}
		paths = append(paths, field)
		return true
	})
	if parseErr != nil {
		return nil, false, parseErr
	}
	return paths, hasEmptyPath, nil
}

func payloadMappingPathsToMap(paths []payloadMappingPath) map[string]string {
	pathMap := make(map[string]string, len(paths))
	for i := range paths {
		pathMap[paths[i].key] = paths[i].path
	}
	return pathMap
}

func applyPayloadMappingViaMaps(result json.RawMessage, pathMap map[string]string) (json.RawMessage, error) {
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

type mappedPayloadField struct {
	key   string
	value gjson.Result
}

func firstNonSpaceJSONByte(in json.RawMessage) byte {
	for _, c := range in {
		switch c {
		case ' ', '\n', '\r', '\t':
			continue
		default:
			return c
		}
	}
	return 0
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
