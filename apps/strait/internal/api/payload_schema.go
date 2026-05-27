package api

import (
	"encoding/json"
	"fmt"
)

const maxSchemaDepth = 32

func validatePayloadAgainstSchema(payload, schema json.RawMessage) error {
	if len(schema) == 0 {
		return nil
	}

	var schemaNode map[string]any
	if err := json.Unmarshal(schema, &schemaNode); err != nil {
		return fmt.Errorf("invalid payload_schema: %w", err)
	}

	var payloadNode any
	if len(payload) == 0 {
		payloadNode = nil
	} else if err := json.Unmarshal(payload, &payloadNode); err != nil {
		return fmt.Errorf("payload is not valid JSON: %w", err)
	}

	return validateSchemaNode(payloadNode, schemaNode, "$", 0)
}

func validateSchemaNode(value any, schema map[string]any, path string, depth int) error {
	if depth > maxSchemaDepth {
		return fmt.Errorf("%s exceeds maximum schema nesting depth (%d)", path, maxSchemaDepth)
	}
	rawType, ok := schema["type"].(string)
	if !ok {
		return nil
	}

	switch rawType {
	case "object":
		return validateObjectSchemaNode(value, schema, path, depth)
	case "array":
		return validateArraySchemaNode(value, schema, path, depth)
	case "string":
		return requireSchemaValueType[string](value, path, "string")
	case "number":
		return requireSchemaValueType[float64](value, path, "number")
	case "integer":
		return validateIntegerSchemaNode(value, path)
	case "boolean":
		return requireSchemaValueType[bool](value, path, "boolean")
	case "null":
		if value != nil {
			return fmt.Errorf("%s must be null", path)
		}
	}
	return nil
}

func validateObjectSchemaNode(value any, schema map[string]any, path string, depth int) error {
	obj, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be object", path)
	}
	if err := validateRequiredObjectFields(obj, schema, path); err != nil {
		return err
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	for key, sub := range props {
		subSchema, ok := sub.(map[string]any)
		if !ok {
			continue
		}
		child, exists := obj[key]
		if !exists {
			continue
		}
		if err := validateSchemaNode(child, subSchema, path+"."+key, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validateRequiredObjectFields(obj map[string]any, schema map[string]any, path string) error {
	required, ok := schema["required"].([]any)
	if !ok {
		return nil
	}
	for _, item := range required {
		key, ok := item.(string)
		if !ok {
			continue
		}
		if _, exists := obj[key]; !exists {
			return fmt.Errorf("%s.%s is required", path, key)
		}
	}
	return nil
}

func validateArraySchemaNode(value any, schema map[string]any, path string, depth int) error {
	arr, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s must be array", path)
	}
	itemsRaw, ok := schema["items"].(map[string]any)
	if !ok {
		return nil
	}
	for idx, item := range arr {
		if err := validateSchemaNode(item, itemsRaw, fmt.Sprintf("%s[%d]", path, idx), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validateIntegerSchemaNode(value any, path string) error {
	num, ok := value.(float64)
	if !ok || num != float64(int64(num)) {
		return fmt.Errorf("%s must be integer", path)
	}
	return nil
}

func requireSchemaValueType[T any](value any, path, name string) error {
	if _, ok := value.(T); !ok {
		return fmt.Errorf("%s must be %s", path, name)
	}
	return nil
}
