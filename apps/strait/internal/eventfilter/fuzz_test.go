package eventfilter

import (
	"encoding/json"
	"testing"
)

func FuzzEval(f *testing.F) {
	f.Add(
		[]byte(`{"eq":[["type","order.created"]]}`),
		[]byte(`{"type":"order.created","amount":42}`),
	)
	f.Add([]byte(`null`), []byte(`{}`))
	f.Add([]byte(`{}`), []byte(`{}`))
	f.Add([]byte(``), []byte(`{}`))
	f.Add([]byte(`{"has":["a.b.c"]}`), []byte(`{"a":{"b":{"c":1}}}`))
	f.Add([]byte(`{"ne":[["status","failed"]]}`), []byte(`{"status":"ok"}`))
	f.Add([]byte(`not json`), []byte(`{"key":"val"}`))
	f.Add([]byte(`{"eq":[["x","1"]]}`), []byte(`not json`))

	f.Fuzz(func(t *testing.T, filterExpr, payload []byte) {
		// Eval should never panic regardless of input.
		_, _ = Eval(json.RawMessage(filterExpr), json.RawMessage(payload))
	})
}

func FuzzGetField(f *testing.F) {
	f.Add("type", `{"type":"order.created"}`)
	f.Add("a.b.c", `{"a":{"b":{"c":"deep"}}}`)
	f.Add("missing", `{"other":"value"}`)
	f.Add("", `{"key":"val"}`)
	f.Add("a.b.c.d.e", `{"a":1}`)
	f.Add("...", `{}`)
	f.Add("key", `{"key":null}`)

	f.Fuzz(func(t *testing.T, path, rawJSON string) {
		var data map[string]any
		if err := json.Unmarshal([]byte(rawJSON), &data); err != nil {
			return
		}
		// getField should never panic regardless of path or data.
		_ = getField(data, path)
	})
}
