package eventfilter

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// buildLargePayload returns a JSON object with n top-level scalar keys plus a
// nested object, modeling an event payload where only a few fields are filtered
// but the whole body must be considered.
func buildLargePayload(n int) json.RawMessage {
	var b strings.Builder
	b.WriteByte('{')
	b.WriteString(`"type":"deploy","env":"prod",`)
	for i := range n {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `"field_%d":"value_%d"`, i, i)
	}
	b.WriteString(`,"user":{"name":"alice","role":"admin"},"count":42,"active":true}`)
	return json.RawMessage(b.String())
}

func BenchmarkEval(b *testing.B) {
	smallPayload := json.RawMessage(`{"type":"deploy","env":"prod","user":{"name":"alice","role":"admin"},"count":42,"active":true}`)
	largePayload := buildLargePayload(100)

	manyEq := make([][2]string, 0, 50)
	for i := range 50 {
		manyEq = append(manyEq, [2]string{fmt.Sprintf("field_%d", i), fmt.Sprintf("value_%d", i)})
	}
	manyEqFilter, _ := json.Marshal(FilterExpr{Eq: manyEq})

	cases := []struct {
		name    string
		filter  json.RawMessage
		payload json.RawMessage
	}{
		{
			name:    "small_payload_single_eq",
			filter:  json.RawMessage(`{"eq":[["type","deploy"]]}`),
			payload: smallPayload,
		},
		{
			name:    "small_payload_nested_eq",
			filter:  json.RawMessage(`{"eq":[["user.name","alice"]]}`),
			payload: smallPayload,
		},
		{
			name:    "small_payload_mixed",
			filter:  json.RawMessage(`{"eq":[["type","deploy"],["count","42"]],"ne":[["env","staging"]],"has":["user.role","active"]}`),
			payload: smallPayload,
		},
		{
			name:    "large_payload_single_eq",
			filter:  json.RawMessage(`{"eq":[["type","deploy"]]}`),
			payload: largePayload,
		},
		{
			name:    "large_payload_many_eq",
			filter:  manyEqFilter,
			payload: largePayload,
		},
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := Eval(c.filter, c.payload); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
