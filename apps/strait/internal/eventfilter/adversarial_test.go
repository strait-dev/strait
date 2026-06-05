package eventfilter

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEval_DeeplyNestedPath tests a nested dot-separated path against a deeply nested payload.
func TestEval_DeeplyNestedPath(t *testing.T) {
	t.Parallel()

	parts := make([]string, maxFilterPathDepth)
	for i := range parts {
		parts[i] = fmt.Sprintf("k%d", i)
	}
	deepPath := strings.Join(parts, ".")

	// Build the nested JSON payload.
	payload := `"leaf"`
	for _, part := range slices.Backward(parts) {
		payload = fmt.Sprintf(`{%q:%s}`, part, payload)
	}

	filter := fmt.Sprintf(`{"eq":[[%q,"leaf"]]}`, deepPath)
	match, err := Eval(json.RawMessage(filter), json.RawMessage(payload))
	require.NoError(t, err)
	assert.True(t, match)

	// A missing deep path within the allowed depth should not match.
	missingParts := append([]string(nil), parts...)
	missingParts[len(missingParts)-1] = "missing"
	filter = fmt.Sprintf(`{"has":[%q]}`, strings.Join(missingParts, "."))
	match, err = Eval(json.RawMessage(filter), json.RawMessage(payload))
	require.NoError(t, err)
	assert.False(t, match)
}

// TestEval_HugeEqArray tests that excessive eq conditions fail closed.
func TestEval_HugeEqArray(t *testing.T) {
	t.Parallel()

	conditions := make([][2]string, 100_000)
	for i := range conditions {
		conditions[i] = [2]string{"status", "ok"}
	}
	expr := FilterExpr{Eq: conditions}
	filterBytes, err := json.Marshal(expr)
	require.NoError(t, err)

	payload := json.RawMessage(`{"status":"ok"}`)
	match, err := Eval(json.RawMessage(filterBytes), payload)
	assert.False(t, match)
	require.Error(t, err)
}

// TestEval_HugeHasArray tests that excessive has conditions fail closed.
func TestEval_HugeHasArray(t *testing.T) {
	t.Parallel()

	fields := make([]string, 100_000)
	for i := range fields {
		fields[i] = "status"
	}
	expr := FilterExpr{Has: fields}
	filterBytes, err := json.Marshal(expr)
	require.NoError(t, err)

	payload := json.RawMessage(`{"status":"ok"}`)
	match, err := Eval(json.RawMessage(filterBytes), payload)
	assert.False(t, match)
	require.Error(t, err)
}

// TestEval_TypeConfusion tests numeric vs string comparison behavior.
func TestEval_TypeConfusion(t *testing.T) {
	t.Parallel()

	// JSON number 42 compared as string "42" via fmt.Sprintf.
	filter := json.RawMessage(`{"eq":[["count","42"]]}`)
	payload := json.RawMessage(`{"count":42}`)
	match, err := Eval(filter, payload)
	require.NoError(t, err)
	assert.True(t, match)

	// Boolean true vs string "true".
	filter = json.RawMessage(`{"eq":[["active","true"]]}`)
	payload = json.RawMessage(`{"active":true}`)
	match, err = Eval(filter, payload)
	require.NoError(t, err)
	assert.True(t, match)

	// Null value vs string "<nil>".
	filter = json.RawMessage(`{"eq":[["val","<nil>"]]}`)
	payload = json.RawMessage(`{"val":null}`)
	match, err = Eval(filter, payload)
	require.NoError(t, err)
	assert.True(t, match)
}

// TestEval_NullValueInPath tests that a nil intermediate in a path is handled.
func TestEval_NullValueInPath(t *testing.T) {
	t.Parallel()

	// Intermediate null in path traversal.
	filter := json.RawMessage(`{"has":["a.b.c"]}`)
	payload := json.RawMessage(`{"a":{"b":null}}`)
	match, err := Eval(filter, payload)
	require.NoError(t, err)
	assert.False(t, match)

	// eq on null intermediate should not match.
	filter = json.RawMessage(`{"eq":[["a.b.c","value"]]}`)
	match, err = Eval(filter, payload)
	require.NoError(t, err)
	assert.False(t, match)
}

// TestEval_EmptyFieldPath tests empty string as a field path.
func TestEval_EmptyFieldPath(t *testing.T) {
	t.Parallel()

	// Empty paths are rejected before evaluation.
	filter := json.RawMessage(`{"has":[""]}`)
	payload := json.RawMessage(`{"":"secret"}`)
	_, err := Eval(filter, payload)
	require.Error(t, err)

	payload = json.RawMessage(`{"a":"b"}`)
	_, err = Eval(filter, payload)
	require.Error(t, err)
}

// TestEval_PathWithOnlyDots tests a path consisting entirely of dots.
func TestEval_PathWithOnlyDots(t *testing.T) {
	t.Parallel()

	paths := []string{".", "..", "...."}
	payload := json.RawMessage(`{"":"nested"}`)

	for _, p := range paths {
		filter := fmt.Sprintf(`{"has":[%q]}`, p)
		// Must not panic.
		_, err := Eval(json.RawMessage(filter), payload)
		require.NoError(t, err)
	}

	filter := fmt.Sprintf(`{"has":[%q]}`, strings.Repeat(".", 100))
	_, err := Eval(json.RawMessage(filter), payload)
	require.Error(t, err)
}

// FuzzEvalAdversarial fuzzes filter expressions and payloads for panics.
func FuzzEvalAdversarial(f *testing.F) {
	f.Add(`{"eq":[["type","deploy"]]}`, `{"type":"deploy"}`)
	f.Add(`{"has":["a.b.c"]}`, `{"a":{"b":{"c":1}}}`)
	f.Add(`{"ne":[["x","y"]]}`, `{"x":"z"}`)
	f.Add(`null`, `{}`)
	f.Add(`{}`, `{"key":"val"}`)
	f.Add(`{"eq":[["",""]]}`, `{"":""}`)
	f.Add(`{"has":["...."]}`, `{}`)
	f.Add(`{"eq":[["a","1"]]}`, `{"a":1}`)

	f.Fuzz(func(t *testing.T, filter, payload string) {
		// Must never panic regardless of input.
		_, _ = Eval(json.RawMessage(filter), json.RawMessage(payload))
	})
}

// FuzzGetFieldDeepPath fuzzes getField with paths containing dots.
func FuzzGetFieldDeepPath(f *testing.F) {
	f.Add("a.b.c", `{"a":{"b":{"c":"deep"}}}`)
	f.Add("", `{"key":"val"}`)
	f.Add("....", `{}`)
	f.Add("a", `{"a":null}`)
	f.Add("x.y.z", `{"x":{"y":1}}`)
	f.Add(strings.Repeat("a.", 50)+"a", `{"a":{"a":{}}}`)

	f.Fuzz(func(t *testing.T, path, rawJSON string) {
		var data map[string]any
		if err := json.Unmarshal([]byte(rawJSON), &data); err != nil {
			return
		}
		// Must never panic.
		_ = getField(data, path)
	})
}

// TestEval_SpecialCharsInFieldNames tests field names containing braces and brackets.
func TestEval_SpecialCharsInFieldNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		key     string
		payload string
	}{
		{"curly braces", "a{b}", `{"a{b}":"val"}`},
		{"square brackets", "a[0]", `{"a[0]":"val"}`},
		{"mixed brackets", "a{b}[c]", `{"a{b}[c]":"val"}`},
		{"angle brackets", "a<b>", `{"a<b>":"val"}`},
		{"parentheses", "a(b)", `{"a(b)":"val"}`},
		{"backslash", `a\b`, `{"a\\b":"val"}`},
		{"quotes", `a"b`, `{"a\"b":"val"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			filter := FilterExpr{Eq: [][2]string{{tc.key, "val"}}}
			filterBytes, err := json.Marshal(filter)
			require.NoError(t, err)
			match, err := Eval(json.RawMessage(filterBytes), json.RawMessage(tc.payload))
			require.NoError(t, err)
			assert.True(t, match)
		})
	}
}
