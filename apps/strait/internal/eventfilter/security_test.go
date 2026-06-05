package eventfilter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEval_UnknownOperatorFailsClosed(t *testing.T) {
	t.Parallel()

	match, err := Eval(
		json.RawMessage(`{"contains":[["role","admin"]]}`),
		json.RawMessage(`{"role":"admin"}`),
	)
	assert.False(t, match)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestEval_TooManyConditionsFailsClosed(t *testing.T) {
	t.Parallel()

	conditions := make([][2]string, maxFilterConditions+1)
	for i := range conditions {
		conditions[i] = [2]string{"status", "ok"}
	}
	filter, err := json.Marshal(FilterExpr{Eq: conditions})
	require.NoError(t, err)

	match, err := Eval(json.RawMessage(filter), json.RawMessage(`{"status":"ok"}`))
	assert.False(t, match)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many conditions")
}

func TestEval_TooDeepPathFailsClosed(t *testing.T) {
	t.Parallel()

	parts := make([]string, maxFilterPathDepth+1)
	for i := range parts {
		parts[i] = "x"
	}
	filter, err := json.Marshal(FilterExpr{Has: []string{strings.Join(parts, ".")}})
	require.NoError(t, err)

	match, err := Eval(json.RawMessage(filter), json.RawMessage(`{"x":{}}`))
	assert.False(t, match)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "too many") || strings.Contains(err.Error(), "exceeds"))
}
