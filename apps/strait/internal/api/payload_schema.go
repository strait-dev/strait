package api

import (
	"encoding/json"
	"fmt"
)

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

	return validateSchemaNode(payloadNode, schemaNode, "$")
}

func validateSchemaNode(value any, schema map[string]any, path string) error {
	if rawType, ok := schema["type"].(string); ok {
		switch rawType {
		case "object":
			obj, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("%s must be object", path)
			}
			if required, ok := schema["required"].([]any); ok {
				for _, item := range required {
					key, ok := item.(string)
					if !ok {
						continue
					}
					if _, exists := obj[key]; !exists {
						return fmt.Errorf("%s.%s is required", path, key)
					}
				}
			}
			if props, ok := schema["properties"].(map[string]any); ok {
				for key, sub := range props {
					subSchema, ok := sub.(map[string]any)
					if !ok {
						continue
					}
					if child, exists := obj[key]; exists {
						if err := validateSchemaNode(child, subSchema, path+"."+key); err != nil {
							return err
						}
					}
				}
			}
		case "array":
			arr, ok := value.([]any)
			if !ok {
				return fmt.Errorf("%s must be array", path)
			}
			if itemsRaw, ok := schema["items"].(map[string]any); ok {
				for idx, item := range arr {
					if err := validateSchemaNode(item, itemsRaw, fmt.Sprintf("%s[%d]", path, idx)); err != nil {
						return err
					}
				}
			}
		case "string":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%s must be string", path)
			}
		case "number":
			if _, ok := value.(float64); !ok {
				return fmt.Errorf("%s must be number", path)
			}
		case "integer":
			num, ok := value.(float64)
			if !ok || num != float64(int64(num)) {
				return fmt.Errorf("%s must be integer", path)
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("%s must be boolean", path)
			}
		case "null":
			if value != nil {
				return fmt.Errorf("%s must be null", path)
			}
		}
	}

	return nil
}
