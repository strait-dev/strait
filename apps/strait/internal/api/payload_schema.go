package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

const (
	maxSchemaDepth        = 32
	payloadSchemaCacheCap = 256
)

var parsedPayloadSchemaCache = payloadSchemaCache{
	entries: make(map[[sha256.Size]byte]map[string]any, payloadSchemaCacheCap),
}

type payloadSchemaCache struct {
	mu      sync.RWMutex
	entries map[[sha256.Size]byte]map[string]any
}

type payloadSchemaPathSegment struct {
	key     string
	index   int
	isIndex bool
}

type payloadSchemaPath []payloadSchemaPathSegment

func newPayloadSchemaPath() payloadSchemaPath {
	return make(payloadSchemaPath, 0, maxSchemaDepth+1)
}

func (p payloadSchemaPath) withKey(key string) payloadSchemaPath {
	return append(p, payloadSchemaPathSegment{key: key})
}

func (p payloadSchemaPath) withIndex(index int) payloadSchemaPath {
	return append(p, payloadSchemaPathSegment{index: index, isIndex: true})
}

func (p payloadSchemaPath) String() string {
	if len(p) == 0 {
		return "$"
	}

	var b strings.Builder
	b.Grow(p.encodedLen())
	b.WriteByte('$')
	for _, segment := range p {
		if segment.isIndex {
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(segment.index))
			b.WriteByte(']')
			continue
		}
		b.WriteByte('.')
		b.WriteString(segment.key)
	}
	return b.String()
}

func (p payloadSchemaPath) encodedLen() int {
	n := 1
	for _, segment := range p {
		if segment.isIndex {
			n += 2 + intDigitCount(segment.index)
			continue
		}
		n += 1 + len(segment.key)
	}
	return n
}

func intDigitCount(n int) int {
	if n == 0 {
		return 1
	}
	count := 0
	for n > 0 {
		count++
		n /= 10
	}
	return count
}

func validatePayloadAgainstSchema(payload, schema json.RawMessage) error {
	if len(schema) == 0 {
		return nil
	}

	schemaNode, err := parsedPayloadSchemaCache.getOrParse(schema)
	if err != nil {
		return fmt.Errorf("invalid payload_schema: %w", err)
	}

	var payloadNode any
	if len(payload) == 0 {
		payloadNode = nil
	} else if err := json.Unmarshal(payload, &payloadNode); err != nil {
		return fmt.Errorf("payload is not valid JSON: %w", err)
	}

	return validateSchemaNode(payloadNode, schemaNode, newPayloadSchemaPath(), 0)
}

func (c *payloadSchemaCache) getOrParse(schema json.RawMessage) (map[string]any, error) {
	key := sha256.Sum256(schema)
	c.mu.RLock()
	schemaNode, ok := c.entries[key]
	c.mu.RUnlock()
	if ok {
		return schemaNode, nil
	}

	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		return nil, err
	}

	c.mu.Lock()
	if existing, ok := c.entries[key]; ok {
		c.mu.Unlock()
		return existing, nil
	}
	if len(c.entries) >= payloadSchemaCacheCap {
		c.entries = make(map[[sha256.Size]byte]map[string]any, payloadSchemaCacheCap)
	}
	c.entries[key] = parsed
	c.mu.Unlock()
	return parsed, nil
}

func validateSchemaNode(value any, schema map[string]any, path payloadSchemaPath, depth int) error {
	if depth > maxSchemaDepth {
		return fmt.Errorf("%s exceeds maximum schema nesting depth (%d)", path.String(), maxSchemaDepth)
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
			return fmt.Errorf("%s must be null", path.String())
		}
	}
	return nil
}

func validateObjectSchemaNode(value any, schema map[string]any, path payloadSchemaPath, depth int) error {
	obj, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be object", path.String())
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
		if err := validateSchemaNode(child, subSchema, path.withKey(key), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validateRequiredObjectFields(obj map[string]any, schema map[string]any, path payloadSchemaPath) error {
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
			return fmt.Errorf("%s is required", path.withKey(key).String())
		}
	}
	return nil
}

func validateArraySchemaNode(value any, schema map[string]any, path payloadSchemaPath, depth int) error {
	arr, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s must be array", path.String())
	}
	itemsRaw, ok := schema["items"].(map[string]any)
	if !ok {
		return nil
	}
	for idx, item := range arr {
		if err := validateSchemaNode(item, itemsRaw, path.withIndex(idx), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validateIntegerSchemaNode(value any, path payloadSchemaPath) error {
	num, ok := value.(float64)
	if !ok || num != float64(int64(num)) {
		return fmt.Errorf("%s must be integer", path.String())
	}
	return nil
}

func requireSchemaValueType[T any](value any, path payloadSchemaPath, name string) error {
	if _, ok := value.(T); !ok {
		return fmt.Errorf("%s must be %s", path.String(), name)
	}
	return nil
}
