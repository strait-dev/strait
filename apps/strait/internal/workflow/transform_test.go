package workflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyOutputTransform(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		rawOutput       json.RawMessage
		transformPath   string
		wantTransformed string
		wantErr         bool
		wantErrContains string
	}{
		{
			name:            "no transform returns original",
			rawOutput:       json.RawMessage(`{"data": {"value": 42, "nested": {"x": 1}}}`),
			transformPath:   "",
			wantTransformed: `{"data": {"value": 42, "nested": {"x": 1}}}`,
		},
		{
			name:            "extract simple field",
			rawOutput:       json.RawMessage(`{"data": {"value": 42}}`),
			transformPath:   "data.value",
			wantTransformed: `42`,
		},
		{
			name:            "extract nested object",
			rawOutput:       json.RawMessage(`{"data": {"nested": {"x": 1, "y": 2}}}`),
			transformPath:   "data.nested",
			wantTransformed: `{"x": 1, "y": 2}`,
		},
		{
			name:            "extract array element",
			rawOutput:       json.RawMessage(`{"items": [{"id": 1}, {"id": 2}]}`),
			transformPath:   "items.0.id",
			wantTransformed: `1`,
		},
		{
			name:            "extract string value",
			rawOutput:       json.RawMessage(`{"name": "hello"}`),
			transformPath:   "name",
			wantTransformed: `"hello"`,
		},
		{
			name:            "extract boolean value",
			rawOutput:       json.RawMessage(`{"active": true}`),
			transformPath:   "active",
			wantTransformed: `true`,
		},
		{
			name:            "path not found returns error",
			rawOutput:       json.RawMessage(`{"data": {"value": 42}}`),
			transformPath:   "nonexistent",
			wantErr:         true,
			wantErrContains: "not found",
		},
		{
			name:            "empty output returns error",
			rawOutput:       json.RawMessage(``),
			transformPath:   "data.value",
			wantErr:         true,
			wantErrContains: "empty or invalid",
		},
		{
			name:            "null output returns error",
			rawOutput:       json.RawMessage(`null`),
			transformPath:   "data.value",
			wantErr:         true,
			wantErrContains: "not found",
		},
		{
			name:            "extract entire array",
			rawOutput:       json.RawMessage(`{"items": [1, 2, 3]}`),
			transformPath:   "items",
			wantTransformed: `[1, 2, 3]`,
		},
		{
			name:            "deeply nested path",
			rawOutput:       json.RawMessage(`{"a": {"b": {"c": {"d": "deep"}}}}`),
			transformPath:   "a.b.c.d",
			wantTransformed: `"deep"`,
		},
		{
			name:            "wildcard gjson path",
			rawOutput:       json.RawMessage(`{"items": [{"id":1},{"id":2}]}`),
			transformPath:   "items.#.id",
			wantTransformed: `[1,2]`,
		},
		{
			name:            "array root",
			rawOutput:       json.RawMessage(`[1,2,3]`),
			transformPath:   "0",
			wantTransformed: `1`,
		},
		{
			name:            "numeric string path in nested",
			rawOutput:       json.RawMessage(`{"data":{"0":"zero"}}`),
			transformPath:   "data.0",
			wantTransformed: `"zero"`,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ApplyOutputTransform(tt.rawOutput, tt.transformPath)

			if tt.wantErr {
				require.Error(t, err)
				require.False(t, tt.wantErrContains !=
					"" && !strings.Contains(err.Error(), tt.wantErrContains),
				)

				return
			}
			require.NoError(t, err)

			gotStr := string(got)
			require.Equal(t, tt.wantTransformed,

				gotStr)
		})
	}
}

func BenchmarkApplyOutputTransform(b *testing.B) {
	rawOutput := json.RawMessage(`{
		"data":{"value":42,"nested":{"x":1,"y":2},"items":[{"id":1},{"id":2},{"id":3}]},
		"metadata":{"status":"completed","attempt":2}
	}`)

	b.Run("no_transform", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			got, err := ApplyOutputTransform(rawOutput, "")
			if err != nil {
				b.Fatal(err)
			}
			if len(got) == 0 {
				b.Fatal("empty output")
			}
		}
	})
	b.Run("nested_scalar", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			got, err := ApplyOutputTransform(rawOutput, "data.value")
			if err != nil {
				b.Fatal(err)
			}
			if string(got) != "42" {
				b.Fatalf("got %s", got)
			}
		}
	})
	b.Run("nested_object", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			got, err := ApplyOutputTransform(rawOutput, "data.nested")
			if err != nil {
				b.Fatal(err)
			}
			if len(got) == 0 {
				b.Fatal("empty object")
			}
		}
	})
	b.Run("wildcard", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			got, err := ApplyOutputTransform(rawOutput, "data.items.#.id")
			if err != nil {
				b.Fatal(err)
			}
			if string(got) != "[1,2,3]" {
				b.Fatalf("got %s", got)
			}
		}
	})
}
