package eventfilter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEval_UnknownOperatorFailsClosed(t *testing.T) {
	t.Parallel()

	match, err := Eval(
		json.RawMessage(`{"contains":[["role","admin"]]}`),
		json.RawMessage(`{"role":"admin"}`),
	)
	if match {
		t.Fatal("unknown operator matched; want fail closed")
	}
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown field", err)
	}
}

func TestEval_TooManyConditionsFailsClosed(t *testing.T) {
	t.Parallel()

	conditions := make([][2]string, maxFilterConditions+1)
	for i := range conditions {
		conditions[i] = [2]string{"status", "ok"}
	}
	filter, err := json.Marshal(FilterExpr{Eq: conditions})
	if err != nil {
		t.Fatalf("marshal filter: %v", err)
	}

	match, err := Eval(json.RawMessage(filter), json.RawMessage(`{"status":"ok"}`))
	if match {
		t.Fatal("oversized condition set matched; want fail closed")
	}
	if err == nil || !strings.Contains(err.Error(), "too many conditions") {
		t.Fatalf("error = %v, want too many conditions", err)
	}
}

func TestEval_TooDeepPathFailsClosed(t *testing.T) {
	t.Parallel()

	parts := make([]string, maxFilterPathDepth+1)
	for i := range parts {
		parts[i] = "x"
	}
	filter, err := json.Marshal(FilterExpr{Has: []string{strings.Join(parts, ".")}})
	if err != nil {
		t.Fatalf("marshal filter: %v", err)
	}

	match, err := Eval(json.RawMessage(filter), json.RawMessage(`{"x":{}}`))
	if match {
		t.Fatal("overly deep path matched; want fail closed")
	}
	if err == nil || !strings.Contains(err.Error(), "too many") && !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want path depth error", err)
	}
}
