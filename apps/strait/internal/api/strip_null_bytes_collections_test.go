package api

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStripNullBytesFromStruct_Collections locks in that NUL bytes decoded from
// JSON \x00 escapes are stripped from slice, array, map, and interface fields,
// not just top-level string fields. Before the fix these survived into Postgres
// text/jsonb columns and produced "invalid byte sequence for encoding UTF8: 0x00"
// 500s for tags/labels/metadata-style payloads.
func TestStripNullBytesFromStruct_Collections(t *testing.T) {
	t.Parallel()

	type nested struct {
		Label string
	}
	type payload struct {
		Tags     []string
		Fixed    [2]string
		Metadata map[string]string
		Items    []nested
		Keyed    map[string]nested
		Any      map[string]any
		Top      string
	}

	p := payload{
		Tags:  []string{"a\x00b", "clean"},
		Fixed: [2]string{"x\x00", "y"},
		Metadata: map[string]string{
			"k\x00ey": "v\x00al",
			"plain":   "ok",
		},
		Items: []nested{{Label: "nested\x00label"}},
		Keyed: map[string]nested{"row": {Label: "deep\x00"}},
		Any: map[string]any{
			"s":    "i\x00face",
			"list": []string{"q\x00r"},
		},
		Top: "t\x00op",
	}

	stripNullBytesFromStruct(reflect.ValueOf(&p).Elem())

	assertNoNul := func(label, s string) {
		t.Helper()
		assert.False(
			t, strings.ContainsRune(
				s,
				0))
	}

	for i, tag := range p.Tags {
		assertNoNul("Tags["+tag+"]", p.Tags[i])
	}
	for i := range p.Fixed {
		assertNoNul("Fixed", p.Fixed[i])
	}
	for k, v := range p.Metadata {
		assertNoNul("Metadata key", k)
		assertNoNul("Metadata value", v)
	}
	for _, it := range p.Items {
		assertNoNul("Items.Label", it.Label)
	}
	for _, n := range p.Keyed {
		assertNoNul("Keyed.Label", n.Label)
	}
	if s, ok := p.Any["s"].(string); ok {
		assertNoNul("Any.s", s)
	} else {
		assert.Fail(t,

			"Any[\"s\"] missing or not a string after sanitization")
	}
	if list, ok := p.Any["list"].([]string); ok {
		for _, s := range list {
			assertNoNul("Any.list", s)
		}
	} else {
		assert.Fail(t,

			"Any[\"list\"] missing or not a []string after sanitization")
	}
	assertNoNul("Top", p.Top)

	// Sanitizing the key must not leave the original NUL-bearing key behind.
	if _, ok := p.Metadata["k\x00ey"]; ok {
		assert.Fail(t,

			"original NUL-bearing map key was not removed")
	}
	if _, ok := p.Metadata["key"]; !ok {
		assert.Fail(t,

			"sanitized map key \"key\" not present")
	}
}
