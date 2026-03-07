package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func FuzzDecodeJSON(f *testing.F) {
	f.Add([]byte(`{"key":"value"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"nested":{"a":1}}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"string"`))
	f.Add([]byte(`123`))
	f.Add([]byte(`[1,2,3]`))
	f.Add([]byte(``))
	f.Add([]byte(`{invalid`))
	f.Add([]byte(`{"a":"\u0000"}`))
	f.Add([]byte(`{"emoji":"😀"}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")

		var target map[string]any
		// decodeJSON should never panic regardless of input
		_ = decodeJSON(r, &target)
	})
}

func FuzzValidatePayloadAgainstSchema(f *testing.F) {
	f.Add(
		[]byte(`{"name":"test","count":1}`),
		[]byte(`{"type":"object","properties":{"name":{"type":"string"},"count":{"type":"number"}},"required":["name"]}`),
	)
	f.Add(
		[]byte(`{"items":[1,2,3]}`),
		[]byte(`{"type":"object","properties":{"items":{"type":"array","items":{"type":"number"}}}}`),
	)
	f.Add([]byte(`{}`), []byte(`{}`))
	f.Add([]byte(``), []byte(``))
	f.Add([]byte(`null`), []byte(`{"type":"null"}`))
	f.Add([]byte(`"string"`), []byte(`{"type":"string"}`))
	f.Add([]byte(`42`), []byte(`{"type":"number"}`))
	f.Add([]byte(`true`), []byte(`{"type":"boolean"}`))
	f.Add([]byte(`[1,2]`), []byte(`{"type":"array"}`))

	f.Fuzz(func(t *testing.T, payload, schema []byte) {
		// validatePayloadAgainstSchema should never panic regardless of input
		_ = validatePayloadAgainstSchema(json.RawMessage(payload), json.RawMessage(schema))
	})
}
