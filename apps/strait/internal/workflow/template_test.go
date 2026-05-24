package workflow

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

func TestRenderTemplateVars(t *testing.T) {
	t.Parallel()
	t.Run("simple string substitution", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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

	t.Run("unresolved nested variable returns payload unchanged", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"to":"{{user.email}}","msg":"Hello {{user.name}}"}`)
		vars := json.RawMessage(`{"user":{"id":"u-123"}}`)

		result := renderTemplateVars(payload, vars)
		if string(result) != string(payload) {
			t.Fatalf("expected payload unchanged, got %s", string(result))
		}
	})

	t.Run("dot-path variable resolution", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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

	t.Run("deeply nested 5+ levels", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"l1":{"l2":{"l3":{"l4":{"l5":"{{value}}"}}}}}`)
		vars := json.RawMessage(`{"value":"deep-replaced"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		l1, ok := got["l1"].(map[string]any)
		if !ok {
			t.Fatalf("l1 should be object, got %T", got["l1"])
		}
		l2, ok := l1["l2"].(map[string]any)
		if !ok {
			t.Fatalf("l2 should be object, got %T", l1["l2"])
		}
		l3, ok := l2["l3"].(map[string]any)
		if !ok {
			t.Fatalf("l3 should be object, got %T", l2["l3"])
		}
		l4, ok := l3["l4"].(map[string]any)
		if !ok {
			t.Fatalf("l4 should be object, got %T", l3["l4"])
		}
		if l4["l5"] != "deep-replaced" {
			t.Fatalf("l4.l5 = %v, want deep-replaced", l4["l5"])
		}
	})

	t.Run("array values", func(t *testing.T) {
		t.Parallel()
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

	t.Run("template in non-string context", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"items":[1,"{{x}}",3]}`)
		vars := json.RawMessage(`{"x":"replaced"}`)

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
		if items[0] != float64(1) || items[1] != "replaced" || items[2] != float64(3) {
			t.Fatalf("items = %v, want [1 replaced 3]", items)
		}
	})

	t.Run("empty template marker", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"val":"{{}}"}`)
		vars := json.RawMessage(`{"x":"ignored"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if got["val"] != "{{}}" {
			t.Fatalf("val = %v, want {{}}", got["val"])
		}
	})

	t.Run("invalid marker does not block later valid template", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"val":"{{}} then {{x}}"}`)
		vars := json.RawMessage(`{"x":"done"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if got["val"] != "{{}} then done" {
			t.Fatalf("val = %v, want '{{}} then done'", got["val"])
		}
	})

	t.Run("nil payload returns as-is", func(t *testing.T) {
		t.Parallel()
		result := renderTemplateVars(nil, json.RawMessage(`{"a":"b"}`))
		if result != nil {
			t.Fatalf("expected nil, got %s", string(result))
		}
	})

	t.Run("nil variables returns payload as-is", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"to":"{{x}}"}`)
		result := renderTemplateVars(payload, nil)
		if string(result) != string(payload) {
			t.Fatalf("expected payload unchanged, got %s", string(result))
		}
	})

	t.Run("non-object variables returns payload as-is", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"to":"{{x}}"}`)
		result := renderTemplateVars(payload, json.RawMessage(`"not an object"`))
		if string(result) != string(payload) {
			t.Fatalf("expected payload unchanged, got %s", string(result))
		}
	})

	t.Run("no templates in payload", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
	t.Parallel()
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
		t.Parallel()
		val, ok := resolveVar(vars, "name")
		if !ok || val != "Alice" {
			t.Fatalf("resolveVar(name) = %v, %v, want Alice, true", val, ok)
		}
	})

	t.Run("nested path", func(t *testing.T) {
		t.Parallel()
		val, ok := resolveVar(vars, "user.email")
		if !ok || val != "alice@example.com" {
			t.Fatalf("resolveVar(user.email) = %v, %v, want alice@example.com, true", val, ok)
		}
	})

	t.Run("deeply nested path", func(t *testing.T) {
		t.Parallel()
		val, ok := resolveVar(vars, "user.address.city")
		if !ok || val != "SF" {
			t.Fatalf("resolveVar(user.address.city) = %v, %v, want SF, true", val, ok)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		t.Parallel()
		_, ok := resolveVar(vars, "missing")
		if ok {
			t.Fatal("expected missing key to return false")
		}
	})

	t.Run("missing nested key", func(t *testing.T) {
		t.Parallel()
		_, ok := resolveVar(vars, "user.phone")
		if ok {
			t.Fatal("expected missing nested key to return false")
		}
	})

	t.Run("path through non-object", func(t *testing.T) {
		t.Parallel()
		_, ok := resolveVar(vars, "name.first")
		if ok {
			t.Fatal("expected path through string to return false")
		}
	})
}

func TestStringify(t *testing.T) {
	t.Parallel()
	t.Run("string", func(t *testing.T) {
		t.Parallel()
		if s := stringify("hello"); s != "hello" {
			t.Fatalf("got %q, want hello", s)
		}
	})

	t.Run("integer float", func(t *testing.T) {
		t.Parallel()
		if s := stringify(float64(42)); s != "42" {
			t.Fatalf("got %q, want 42", s)
		}
	})

	t.Run("fractional float", func(t *testing.T) {
		t.Parallel()
		s := stringify(3.14)
		if !strings.Contains(s, "3.14") {
			t.Fatalf("got %q, want contains 3.14", s)
		}
	})

	t.Run("bool true", func(t *testing.T) {
		t.Parallel()
		if s := stringify(true); s != "true" {
			t.Fatalf("got %q, want true", s)
		}
	})

	t.Run("bool false", func(t *testing.T) {
		t.Parallel()
		if s := stringify(false); s != "false" {
			t.Fatalf("got %q, want false", s)
		}
	})

	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		if s := stringify(nil); s != "" {
			t.Fatalf("got %q, want empty", s)
		}
	})

	t.Run("object", func(t *testing.T) {
		t.Parallel()
		s := stringify(map[string]any{"k": "v"})
		if !strings.Contains(s, `"k"`) || !strings.Contains(s, `"v"`) {
			t.Fatalf("got %q, want JSON object", s)
		}
	})
}

// TestRenderTemplateVarsSplice_MatchesTreeWalk locks the in-place splice fast
// path to the generic tree-walk fallback: for randomized nested payloads the two
// must produce structurally identical JSON. This guards the optimization against
// silent divergence in resolution, type preservation, or escaping.
func TestRenderTemplateVarsSplice_MatchesTreeWalk(t *testing.T) {
	t.Parallel()

	varNames := []string{"name", "count", "flag", "url", "id", "data", "nested"}

	for range 3000 {
		// Random variables, marshaled then unmarshaled so numbers normalize to
		// float64 exactly as the production paths see them.
		rawVars := make(map[string]any)
		for range rand.IntN(5) + 1 {
			key := varNames[rand.IntN(len(varNames))]
			switch rand.IntN(5) {
			case 0:
				rawVars[key] = randomString(rand.IntN(12) + 1)
			case 1:
				rawVars[key] = rand.IntN(100000)
			case 2:
				rawVars[key] = rand.IntN(2) == 0
			case 3:
				rawVars[key] = nil
			case 4:
				rawVars[key] = map[string]any{"a": randomString(4), "b": rand.IntN(10)}
			}
		}
		varsJSON, _ := json.Marshal(rawVars)
		var vars map[string]any
		if err := json.Unmarshal(varsJSON, &vars); err != nil {
			t.Fatalf("unmarshal vars: %v", err)
		}

		payloadJSON := randomTemplatedPayload(varNames, 0)

		spliced, ok := renderTemplateVarsSplice([]byte(payloadJSON), vars)
		if !ok {
			continue
		}

		var data any
		if err := json.Unmarshal([]byte(payloadJSON), &data); err != nil {
			t.Fatalf("payload not valid JSON: %s", payloadJSON)
		}
		rendered, changed := walkAndRender(data, vars)
		ref := []byte(payloadJSON)
		if changed {
			ref, _ = json.Marshal(rendered)
		}

		if !jsonStructurallyEqual(t, spliced, ref) {
			t.Fatalf("splice != tree walk:\n payload: %s\n vars: %s\n splice: %s\n ref:    %s",
				payloadJSON, varsJSON, spliced, ref)
		}
	}
}

// randomTemplatedPayload builds a random JSON object string mixing template
// references, embedded templates, static scalars, nested objects, and arrays.
func randomTemplatedPayload(varNames []string, depth int) string {
	obj := make(map[string]any)
	for j := range rand.IntN(4) + 1 {
		field := fmt.Sprintf("f_%d", j)
		obj[field] = randomTemplatedValue(varNames, depth)
	}
	b, _ := json.Marshal(obj)
	return string(b)
}

func randomTemplatedValue(varNames []string, depth int) any {
	choice := rand.IntN(7)
	if depth >= 3 && choice >= 5 {
		choice = rand.IntN(5)
	}
	switch choice {
	case 0:
		return "{{" + varNames[rand.IntN(len(varNames))] + "}}"
	case 1:
		return "prefix-{{" + varNames[rand.IntN(len(varNames))] + "}}-{{" + varNames[rand.IntN(len(varNames))] + "}}-suffix"
	case 2:
		return randomString(8)
	case 3:
		return rand.IntN(1000)
	case 4:
		return rand.IntN(2) == 0
	case 5:
		var raw json.RawMessage
		_ = json.Unmarshal([]byte(randomTemplatedPayload(varNames, depth+1)), &raw)
		var nested any
		_ = json.Unmarshal(raw, &nested)
		return nested
	default:
		arr := make([]any, rand.IntN(3)+1)
		for i := range arr {
			arr[i] = randomTemplatedValue(varNames, depth+1)
		}
		return arr
	}
}

func jsonStructurallyEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		t.Fatalf("unmarshal a (%s): %v", a, err)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		t.Fatalf("unmarshal b (%s): %v", b, err)
	}
	ar, _ := json.Marshal(av)
	br, _ := json.Marshal(bv)
	return string(ar) == string(br)
}

func BenchmarkRenderTemplateVars(b *testing.B) {
	payloadWithTemplates := json.RawMessage(`{
		"to":"{{user_email}}",
		"subject":"Hello {{user_name}}",
		"count":"{{total}}",
		"nested":{"config":"{{settings}}","msg":"Welcome {{user_name}}, you have {{total}} items"}
	}`)
	payloadWithoutTemplates := json.RawMessage(`{
		"to":"ops@example.com",
		"subject":"Workflow complete",
		"count":42,
		"nested":{"config":{"theme":"dark","lang":"en"},"msg":"No substitutions needed"}
	}`)
	vars := json.RawMessage(`{
		"user_email":"john@example.com",
		"user_name":"John",
		"total":42,
		"settings":{"theme":"dark","lang":"en"}
	}`)

	b.Run("with_templates", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = renderTemplateVars(payloadWithTemplates, vars)
		}
	})
	b.Run("without_templates", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = renderTemplateVars(payloadWithoutTemplates, vars)
		}
	})
	b.Run("unresolved_templates", func(b *testing.B) {
		payload := json.RawMessage(`{"message":"Hello {{missing_name}}","nested":{"value":"{{missing_value}}"}}`)
		b.ReportAllocs()
		for b.Loop() {
			_ = renderTemplateVars(payload, vars)
		}
	})
}

func BenchmarkRenderStringValue(b *testing.B) {
	vars := map[string]any{
		"name":  "test",
		"count": 42,
		"email": "test@example.com",
	}

	b.Run("no_vars", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			renderStringValue("plain string no vars", vars)
		}
	})
	b.Run("single_var", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			renderStringValue("{{name}}", vars)
		}
	})
	b.Run("mixed_content", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			renderStringValue("Hello {{name}}, your count is {{count}} and email is {{email}}", vars)
		}
	})
}

func BenchmarkRenderStringTemplate(b *testing.B) {
	variables := json.RawMessage(`{"prefix":"check","id":"42","suffix":"done","user":{"email":"a@b.com"}}`)

	b.Run("no_vars", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = renderStringTemplate("static-key", variables)
		}
	})
	b.Run("single_var", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = renderStringTemplate("aml:{{id}}", variables)
		}
	})
	b.Run("mixed_content", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = renderStringTemplate("{{prefix}}:{{id}}:{{suffix}}:{{user.email}}", variables)
		}
	})
}

func TestRenderStringTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		template  string
		variables json.RawMessage
		want      string
	}{
		{
			name:      "simple substitution",
			template:  "aml:{{app_id}}",
			variables: json.RawMessage(`{"app_id":"app-123"}`),
			want:      "aml:app-123",
		},
		{
			name:      "multiple substitutions",
			template:  "{{prefix}}:{{id}}:{{suffix}}",
			variables: json.RawMessage(`{"prefix":"check","id":"42","suffix":"done"}`),
			want:      "check:42:done",
		},
		{
			name:      "nested path",
			template:  "user:{{user.email}}",
			variables: json.RawMessage(`{"user":{"email":"a@b.com"}}`),
			want:      "user:a@b.com",
		},
		{
			name:      "no template vars",
			template:  "static-key",
			variables: json.RawMessage(`{"foo":"bar"}`),
			want:      "static-key",
		},
		{
			name:      "unresolved variable left as-is",
			template:  "key:{{missing}}",
			variables: json.RawMessage(`{"foo":"bar"}`),
			want:      "key:{{missing}}",
		},
		{
			name:      "empty variables",
			template:  "key:{{foo}}",
			variables: json.RawMessage(`{}`),
			want:      "key:{{foo}}",
		},
		{
			name:      "nil variables",
			template:  "key:{{foo}}",
			variables: nil,
			want:      "key:{{foo}}",
		},
		{
			name:      "numeric value stringified",
			template:  "order:{{count}}",
			variables: json.RawMessage(`{"count":42}`),
			want:      "order:42",
		},
		{
			name:      "boolean value stringified",
			template:  "flag:{{enabled}}",
			variables: json.RawMessage(`{"enabled":true}`),
			want:      "flag:true",
		},
		{
			name:      "empty string template",
			template:  "",
			variables: json.RawMessage(`{"foo":"bar"}`),
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := renderStringTemplate(tt.template, tt.variables)
			if got != tt.want {
				t.Errorf("renderStringTemplate(%q) = %q, want %q", tt.template, got, tt.want)
			}
		})
	}
}
