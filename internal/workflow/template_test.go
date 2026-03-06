package workflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderTemplateVars(t *testing.T) {
	t.Run("simple string substitution", func(t *testing.T) {
		payload := json.RawMessage(`{"to":"{{user_email}}","subject":"Hello"}`)
		vars := json.RawMessage(`{"user_email":"john@example.com"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if got["to"] != "john@example.com" {
			t.Fatalf("to = %v, want john@example.com", got["to"])
		}
		if got["subject"] != "Hello" {
			t.Fatalf("subject = %v, want Hello", got["subject"])
		}
	})

	t.Run("preserves number type", func(t *testing.T) {
		payload := json.RawMessage(`{"count":"{{total}}"}`)
		vars := json.RawMessage(`{"total":42}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		count, ok := got["count"].(float64)
		if !ok {
			t.Fatalf("count should be a number, got %T: %v", got["count"], got["count"])
		}
		if count != 42 {
			t.Fatalf("count = %v, want 42", count)
		}
	})

	t.Run("preserves boolean type", func(t *testing.T) {
		payload := json.RawMessage(`{"active":"{{is_active}}"}`)
		vars := json.RawMessage(`{"is_active":true}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		active, ok := got["active"].(bool)
		if !ok {
			t.Fatalf("active should be a bool, got %T: %v", got["active"], got["active"])
		}
		if !active {
			t.Fatal("active = false, want true")
		}
	})

	t.Run("preserves object type", func(t *testing.T) {
		payload := json.RawMessage(`{"config":"{{settings}}"}`)
		vars := json.RawMessage(`{"settings":{"key":"value","n":1}}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		cfg, ok := got["config"].(map[string]any)
		if !ok {
			t.Fatalf("config should be an object, got %T", got["config"])
		}
		if cfg["key"] != "value" {
			t.Fatalf("config.key = %v, want value", cfg["key"])
		}
	})

	t.Run("embedded variable in string", func(t *testing.T) {
		payload := json.RawMessage(`{"message":"Hello {{name}}, welcome!"}`)
		vars := json.RawMessage(`{"name":"Alice"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if got["message"] != "Hello Alice, welcome!" {
			t.Fatalf("message = %v, want 'Hello Alice, welcome!'", got["message"])
		}
	})

	t.Run("embedded number in string", func(t *testing.T) {
		payload := json.RawMessage(`{"message":"You have {{count}} items"}`)
		vars := json.RawMessage(`{"count":5}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if got["message"] != "You have 5 items" {
			t.Fatalf("message = %v, want 'You have 5 items'", got["message"])
		}
	})

	t.Run("multiple variables in one string", func(t *testing.T) {
		payload := json.RawMessage(`{"greeting":"Hi {{first}} {{last}}!"}`)
		vars := json.RawMessage(`{"first":"Jane","last":"Doe"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if got["greeting"] != "Hi Jane Doe!" {
			t.Fatalf("greeting = %v, want 'Hi Jane Doe!'", got["greeting"])
		}
	})

	t.Run("unresolved variables left as-is", func(t *testing.T) {
		payload := json.RawMessage(`{"to":"{{unknown_var}}"}`)
		vars := json.RawMessage(`{"other":"value"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if got["to"] != "{{unknown_var}}" {
			t.Fatalf("to = %v, want {{unknown_var}}", got["to"])
		}
	})

	t.Run("dot-path variable resolution", func(t *testing.T) {
		payload := json.RawMessage(`{"email":"{{user.email}}"}`)
		vars := json.RawMessage(`{"user":{"email":"nested@example.com"}}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if got["email"] != "nested@example.com" {
			t.Fatalf("email = %v, want nested@example.com", got["email"])
		}
	})

	t.Run("nested payload objects", func(t *testing.T) {
		payload := json.RawMessage(`{"outer":{"inner":"{{val}}"}}`)
		vars := json.RawMessage(`{"val":"deep"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		outer, ok := got["outer"].(map[string]any)
		if !ok {
			t.Fatalf("outer should be object, got %T", got["outer"])
		}
		if outer["inner"] != "deep" {
			t.Fatalf("outer.inner = %v, want deep", outer["inner"])
		}
	})

	t.Run("array values", func(t *testing.T) {
		payload := json.RawMessage(`{"items":["{{a}}","static","{{b}}"]}`)
		vars := json.RawMessage(`{"a":"first","b":"third"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		items, ok := got["items"].([]any)
		if !ok {
			t.Fatalf("items should be array, got %T", got["items"])
		}
		if len(items) != 3 {
			t.Fatalf("len(items) = %d, want 3", len(items))
		}
		if items[0] != "first" || items[1] != "static" || items[2] != "third" {
			t.Fatalf("items = %v, want [first static third]", items)
		}
	})

	t.Run("nil payload returns as-is", func(t *testing.T) {
		result := renderTemplateVars(nil, json.RawMessage(`{"a":"b"}`))
		if result != nil {
			t.Fatalf("expected nil, got %s", string(result))
		}
	})

	t.Run("nil variables returns payload as-is", func(t *testing.T) {
		payload := json.RawMessage(`{"to":"{{x}}"}`)
		result := renderTemplateVars(payload, nil)
		if string(result) != string(payload) {
			t.Fatalf("expected payload unchanged, got %s", string(result))
		}
	})

	t.Run("non-object variables returns payload as-is", func(t *testing.T) {
		payload := json.RawMessage(`{"to":"{{x}}"}`)
		result := renderTemplateVars(payload, json.RawMessage(`"not an object"`))
		if string(result) != string(payload) {
			t.Fatalf("expected payload unchanged, got %s", string(result))
		}
	})

	t.Run("no templates in payload", func(t *testing.T) {
		payload := json.RawMessage(`{"to":"plain@example.com","count":42}`)
		vars := json.RawMessage(`{"user_email":"john@example.com"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if got["to"] != "plain@example.com" {
			t.Fatalf("to = %v, want plain@example.com", got["to"])
		}
	})

	t.Run("null variable value replaces with empty string in embedded", func(t *testing.T) {
		payload := json.RawMessage(`{"msg":"value is {{x}} here"}`)
		vars := json.RawMessage(`{"x":null}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if got["msg"] != "value is  here" {
			t.Fatalf("msg = %q, want 'value is  here'", got["msg"])
		}
	})

	t.Run("null variable value preserved for full replacement", func(t *testing.T) {
		payload := json.RawMessage(`{"val":"{{x}}"}`)
		vars := json.RawMessage(`{"x":null}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if got["val"] != nil {
			t.Fatalf("val = %v, want nil", got["val"])
		}
	})
}

func TestResolveVar(t *testing.T) {
	vars := map[string]any{
		"name": "Alice",
		"user": map[string]any{
			"email": "alice@example.com",
			"address": map[string]any{
				"city": "SF",
			},
		},
	}

	t.Run("simple key", func(t *testing.T) {
		val, ok := resolveVar(vars, "name")
		if !ok || val != "Alice" {
			t.Fatalf("resolveVar(name) = %v, %v, want Alice, true", val, ok)
		}
	})

	t.Run("nested path", func(t *testing.T) {
		val, ok := resolveVar(vars, "user.email")
		if !ok || val != "alice@example.com" {
			t.Fatalf("resolveVar(user.email) = %v, %v, want alice@example.com, true", val, ok)
		}
	})

	t.Run("deeply nested path", func(t *testing.T) {
		val, ok := resolveVar(vars, "user.address.city")
		if !ok || val != "SF" {
			t.Fatalf("resolveVar(user.address.city) = %v, %v, want SF, true", val, ok)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		_, ok := resolveVar(vars, "missing")
		if ok {
			t.Fatal("expected missing key to return false")
		}
	})

	t.Run("missing nested key", func(t *testing.T) {
		_, ok := resolveVar(vars, "user.phone")
		if ok {
			t.Fatal("expected missing nested key to return false")
		}
	})

	t.Run("path through non-object", func(t *testing.T) {
		_, ok := resolveVar(vars, "name.first")
		if ok {
			t.Fatal("expected path through string to return false")
		}
	})
}

func TestStringify(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		if s := stringify("hello"); s != "hello" {
			t.Fatalf("got %q, want hello", s)
		}
	})

	t.Run("integer float", func(t *testing.T) {
		if s := stringify(float64(42)); s != "42" {
			t.Fatalf("got %q, want 42", s)
		}
	})

	t.Run("fractional float", func(t *testing.T) {
		s := stringify(3.14)
		if !strings.Contains(s, "3.14") {
			t.Fatalf("got %q, want contains 3.14", s)
		}
	})

	t.Run("bool true", func(t *testing.T) {
		if s := stringify(true); s != "true" {
			t.Fatalf("got %q, want true", s)
		}
	})

	t.Run("bool false", func(t *testing.T) {
		if s := stringify(false); s != "false" {
			t.Fatalf("got %q, want false", s)
		}
	})

	t.Run("nil", func(t *testing.T) {
		if s := stringify(nil); s != "" {
			t.Fatalf("got %q, want empty", s)
		}
	})

	t.Run("object", func(t *testing.T) {
		s := stringify(map[string]any{"k": "v"})
		if !strings.Contains(s, `"k"`) || !strings.Contains(s, `"v"`) {
			t.Fatalf("got %q, want JSON object", s)
		}
	})
}
