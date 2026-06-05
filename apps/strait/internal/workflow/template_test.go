package workflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderTemplateVars(t *testing.T) {
	t.Parallel()
	t.Run("simple string substitution", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"to":"{{user_email}}","subject":"Hello"}`)
		vars := json.RawMessage(`{"user_email":"john@example.com"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))
		require.Equal(t, "john@example.com",

			got["to"],
		)
		require.Equal(t, "Hello",
			got["subject"])

	})

	t.Run("preserves number type", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"count":"{{total}}"}`)
		vars := json.RawMessage(`{"total":42}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))

		count, ok := got["count"].(float64)
		require.True(t, ok)
		require.EqualValues(t, 42, count)

	})

	t.Run("preserves boolean type", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"active":"{{is_active}}"}`)
		vars := json.RawMessage(`{"is_active":true}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))

		active, ok := got["active"].(bool)
		require.True(t, ok)
		require.True(t, active)

	})

	t.Run("preserves object type", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"config":"{{settings}}"}`)
		vars := json.RawMessage(`{"settings":{"key":"value","n":1}}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))

		cfg, ok := got["config"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "value",
			cfg["key"])

	})

	t.Run("embedded variable in string", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"message":"Hello {{name}}, welcome!"}`)
		vars := json.RawMessage(`{"name":"Alice"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))
		require.Equal(t, "Hello Alice, welcome!",

			got["message"])

	})

	t.Run("embedded number in string", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"message":"You have {{count}} items"}`)
		vars := json.RawMessage(`{"count":5}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))
		require.Equal(t, "You have 5 items",

			got["message"])

	})

	t.Run("multiple variables in one string", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"greeting":"Hi {{first}} {{last}}!"}`)
		vars := json.RawMessage(`{"first":"Jane","last":"Doe"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))
		require.Equal(t, "Hi Jane Doe!",
			got["greeting"],
		)

	})

	t.Run("unresolved variables left as-is", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"to":"{{unknown_var}}"}`)
		vars := json.RawMessage(`{"other":"value"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))
		require.Equal(t, "{{unknown_var}}",

			got["to"])

	})

	t.Run("unresolved nested variable returns payload unchanged", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"to":"{{user.email}}","msg":"Hello {{user.name}}"}`)
		vars := json.RawMessage(`{"user":{"id":"u-123"}}`)

		result := renderTemplateVars(payload, vars)
		require.Equal(t, string(
			payload), string(result),
		)

	})

	t.Run("dot-path variable resolution", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"email":"{{user.email}}"}`)
		vars := json.RawMessage(`{"user":{"email":"nested@example.com"}}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))
		require.Equal(t, "nested@example.com",

			got["email"])

	})

	t.Run("nested payload objects", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"outer":{"inner":"{{val}}"}}`)
		vars := json.RawMessage(`{"val":"deep"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))

		outer, ok := got["outer"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "deep",
			outer["inner"])

	})

	t.Run("deeply nested 5+ levels", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"l1":{"l2":{"l3":{"l4":{"l5":"{{value}}"}}}}}`)
		vars := json.RawMessage(`{"value":"deep-replaced"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))

		l1, ok := got["l1"].(map[string]any)
		require.True(t, ok)

		l2, ok := l1["l2"].(map[string]any)
		require.True(t, ok)

		l3, ok := l2["l3"].(map[string]any)
		require.True(t, ok)

		l4, ok := l3["l4"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "deep-replaced",
			l4["l5"])

	})

	t.Run("array values", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"items":["{{a}}","static","{{b}}"]}`)
		vars := json.RawMessage(`{"a":"first","b":"third"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))

		items, ok := got["items"].([]any)
		require.True(t, ok)
		require.Len(t, items, 3)
		require.False(t, items[0] != "first" ||
			items[1] !=
				"static" || items[2] != "third")

	})

	t.Run("template in non-string context", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"items":[1,"{{x}}",3]}`)
		vars := json.RawMessage(`{"x":"replaced"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))

		items, ok := got["items"].([]any)
		require.True(t, ok)
		require.Len(t, items, 3)
		require.False(t, items[0] != float64(1) || items[1] != "replaced" || items[2] != float64(3))

	})

	t.Run("empty template marker", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"val":"{{}}"}`)
		vars := json.RawMessage(`{"x":"ignored"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))
		require.Equal(t, "{{}}",
			got["val"],
		)

	})

	t.Run("invalid marker does not block later valid template", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"val":"{{}} then {{x}}"}`)
		vars := json.RawMessage(`{"x":"done"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))
		require.Equal(t, "{{}} then done",

			got["val"])

	})

	t.Run("nil payload returns as-is", func(t *testing.T) {
		t.Parallel()
		result := renderTemplateVars(nil, json.RawMessage(`{"a":"b"}`))
		require.Nil(t, result)

	})

	t.Run("nil variables returns payload as-is", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"to":"{{x}}"}`)
		result := renderTemplateVars(payload, nil)
		require.Equal(t, string(
			payload), string(result),
		)

	})

	t.Run("non-object variables returns payload as-is", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"to":"{{x}}"}`)
		result := renderTemplateVars(payload, json.RawMessage(`"not an object"`))
		require.Equal(t, string(
			payload), string(result),
		)

	})

	t.Run("no templates in payload", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"to":"plain@example.com","count":42}`)
		vars := json.RawMessage(`{"user_email":"john@example.com"}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))
		require.Equal(t, "plain@example.com",

			got["to"])

	})

	t.Run("null variable value replaces with empty string in embedded", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"msg":"value is {{x}} here"}`)
		vars := json.RawMessage(`{"x":null}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))
		require.Equal(t, "value is  here",

			got["msg"])

	})

	t.Run("null variable value preserved for full replacement", func(t *testing.T) {
		t.Parallel()
		payload := json.RawMessage(`{"val":"{{x}}"}`)
		vars := json.RawMessage(`{"x":null}`)

		result := renderTemplateVars(payload, vars)
		var got map[string]any
		require.NoError(t, json.
			Unmarshal(result,
				&got,
			))
		require.Nil(t, got["val"])

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
		require.False(t, !ok ||
			val != "Alice",
		)

	})

	t.Run("nested path", func(t *testing.T) {
		t.Parallel()
		val, ok := resolveVar(vars, "user.email")
		require.False(t, !ok ||
			val != "alice@example.com",
		)

	})

	t.Run("deeply nested path", func(t *testing.T) {
		t.Parallel()
		val, ok := resolveVar(vars, "user.address.city")
		require.False(t, !ok ||
			val != "SF",
		)

	})

	t.Run("missing key", func(t *testing.T) {
		t.Parallel()
		_, ok := resolveVar(vars, "missing")
		require.False(t, ok)

	})

	t.Run("missing nested key", func(t *testing.T) {
		t.Parallel()
		_, ok := resolveVar(vars, "user.phone")
		require.False(t, ok)

	})

	t.Run("path through non-object", func(t *testing.T) {
		t.Parallel()
		_, ok := resolveVar(vars, "name.first")
		require.False(t, ok)

	})
}

func TestStringify(t *testing.T) {
	t.Parallel()
	t.Run("string", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "hello",
			stringify("hello"))

	})

	t.Run("integer float", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "42", stringify(float64(42)))

	})

	t.Run("fractional float", func(t *testing.T) {
		t.Parallel()
		s := stringify(3.14)
		require.True(t, strings.Contains(s,
			"3.14"))

	})

	t.Run("bool true", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "true",
			stringify(
				true))

	})

	t.Run("bool false", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "false",
			stringify(false))

	})

	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "", stringify(nil))

	})

	t.Run("object", func(t *testing.T) {
		t.Parallel()
		s := stringify(map[string]any{"k": "v"})
		require.False(t, !strings.Contains(
			s, `"k"`) ||

			!strings.Contains(s, `"v"`))

	})
}

func TestRenderTemplateVars_RepeatedVariablesPreserveBehavior(t *testing.T) {
	t.Parallel()
	payload := json.RawMessage(`{
		"first":"{{user.email}}",
		"second":"{{user.email}}",
		"message":"{{user.email}}/{{count}}/{{missing}}/{{user.email}}",
		"native_count":"{{count}}",
		"native_config":"{{config}}"
	}`)
	vars := json.RawMessage(`{
		"user":{"email":"ops@example.com"},
		"count":42,
		"config":{"retry":3,"enabled":true}
	}`)

	result := renderTemplateVars(payload, vars)

	var got map[string]any
	require.NoError(t, json.
		Unmarshal(result,
			&got,
		))
	require.False(t, got["first"] != "ops@example.com" ||
		got["second"] != "ops@example.com")
	require.Equal(t, "ops@example.com/42/{{missing}}/ops@example.com",

		got["message"])
	require.Equal(t, float64(42), got["native_count"])

	if _, ok := got["native_config"].(map[string]any); !ok {
		require.Failf(t, "test failure",

			"native_config = %T, want object", got["native_config"])
	}
}

func TestRenderStringTemplate_RepeatedVariablesPreserveBehavior(t *testing.T) {
	t.Parallel()
	variables := json.RawMessage(`{"user":{"email":"ops@example.com"},"count":42}`)

	got := renderStringTemplate("{{user.email}}:{{count}}:{{missing}}:{{user.email}}", variables)
	require.Equal(t, "ops@example.com:42:{{missing}}:ops@example.com",

		got)

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
	b.Run("repeated_nested_vars", func(b *testing.B) {
		var builder strings.Builder
		builder.WriteByte('{')
		for i := range 64 {
			if i > 0 {
				builder.WriteByte(',')
			}
			builder.WriteString(`"field`)
			builder.WriteByte(byte('a' + i%26))
			builder.WriteString(`":"{{user.email}}/{{total}}/{{missing}}/{{user.email}}"`)
		}
		builder.WriteByte('}')
		payload := json.RawMessage(builder.String())
		vars := json.RawMessage(`{"user":{"email":"john@example.com"},"total":42}`)

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
	b.Run("repeated_nested_vars", func(b *testing.B) {
		template := strings.Repeat("{{user.email}}:{{id}}:{{missing}}:", 16)
		b.ReportAllocs()
		for b.Loop() {
			_ = renderStringTemplate(template, variables)
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
			assert.Equal(t, tt.want,
				got)

		})
	}
}
