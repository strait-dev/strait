package eventfilter

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// errString renders an error for byte-exact comparison (nil-safe).
func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

// TestEvalParsed_MatchesEval asserts EvalParsed via a fresh ParsedPayload is
// indistinguishable from the original single-shot Eval across representative
// inputs, including the error cases (oversized, invalid filter, invalid
// payload, empty filter).
func TestEvalParsed_MatchesEval(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		filter  string
		payload string
	}{
		{"empty_filter_matches_all", `null`, `{"a":1}`},
		{"empty_object_filter", `{}`, `{"a":1}`},
		{"single_eq_match", `{"eq":[["type","deploy"]]}`, `{"type":"deploy"}`},
		{"single_eq_nomatch", `{"eq":[["type","deploy"]]}`, `{"type":"build"}`},
		{"nested_eq", `{"eq":[["user.name","alice"]]}`, `{"user":{"name":"alice"}}`},
		{"ne_match", `{"ne":[["env","staging"]]}`, `{"env":"prod"}`},
		{"has_present", `{"has":["a.b"]}`, `{"a":{"b":1}}`},
		{"has_absent", `{"has":["a.b"]}`, `{"a":{}}`},
		{"number_eq", `{"eq":[["count","42"]]}`, `{"count":42}`},
		{"bool_eq", `{"eq":[["active","true"]]}`, `{"active":true}`},
		{"invalid_filter_json", `{"eq":`, `{"a":1}`},
		{"unknown_filter_field", `{"bogus":1}`, `{"a":1}`},
		{"invalid_payload_json", `{"eq":[["a","1"]]}`, `{"a":`},
		{"payload_overflow_number", `{"eq":[["a","1"]]}`, `{"a":1e1000}`},
		{"empty_filter_invalid_payload", `null`, `{"a":`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			wantMatch, wantErr := Eval(json.RawMessage(c.filter), json.RawMessage(c.payload))
			gotMatch, gotErr := EvalParsed(json.RawMessage(c.filter), NewParsedPayload(json.RawMessage(c.payload)))
			if gotMatch != wantMatch || errString(gotErr) != errString(wantErr) {
				t.Fatalf("EvalParsed=(%v,%q) Eval=(%v,%q)", gotMatch, errString(gotErr), wantMatch, errString(wantErr))
			}
		})
	}
}

// TestEvalParsed_ReusedPayloadMatchesIndependentEval evaluates many different
// filters against one shared ParsedPayload and confirms each result matches an
// independent Eval call. This is the fan-out path used by event dispatch.
func TestEvalParsed_ReusedPayloadMatchesIndependentEval(t *testing.T) {
	t.Parallel()
	payload := json.RawMessage(`{"type":"deploy","env":"prod","user":{"name":"alice","role":"admin"},"count":42,"active":true}`)
	filters := []string{
		`{"eq":[["type","deploy"]]}`,
		`{"eq":[["env","staging"]]}`,
		`{"has":["user.name"]}`,
		`{"ne":[["count","42"]]}`,
		`null`,
		`{"eq":[["user.role","admin"],["active","true"]]}`,
	}
	shared := NewParsedPayload(payload)
	for _, f := range filters {
		want, wantErr := Eval(json.RawMessage(f), payload)
		got, gotErr := EvalParsed(json.RawMessage(f), shared)
		if got != want || errString(gotErr) != errString(wantErr) {
			t.Fatalf("filter %s: shared=(%v,%q) independent=(%v,%q)", f, got, errString(gotErr), want, errString(wantErr))
		}
	}
}

// TestParsedPayload_LazyParseOnce verifies the payload is decoded at most once
// and never decoded for an empty (match-all) filter.
func TestParsedPayload_LazyParseOnce(t *testing.T) {
	t.Parallel()

	// Empty filter must short-circuit before touching the payload.
	pp := NewParsedPayload(json.RawMessage(`{"a":1}`))
	if _, err := EvalParsed(json.RawMessage(`null`), pp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.parsed {
		t.Fatal("empty filter parsed the payload; expected lazy no-op")
	}

	// First non-empty filter decodes; the cached map is reused afterward.
	pp = NewParsedPayload(json.RawMessage(`{"a":1}`))
	if _, err := EvalParsed(json.RawMessage(`{"eq":[["a","1"]]}`), pp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pp.parsed {
		t.Fatal("non-empty filter did not parse the payload")
	}
	firstData := reflect.ValueOf(pp.data).Pointer()
	if _, err := EvalParsed(json.RawMessage(`{"has":["a"]}`), pp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reflect.ValueOf(pp.data).Pointer() != firstData {
		t.Fatal("payload re-parsed on second evaluation; expected cached map")
	}
}

// FuzzEvalParsedParity is the differential oracle: for arbitrary inputs the
// original Eval and EvalParsed(NewParsedPayload(...)) must agree exactly on both
// the boolean result and the error text.
func FuzzEvalParsedParity(f *testing.F) {
	seeds := []struct{ filter, payload string }{
		{`{"eq":[["type","deploy"]]}`, `{"type":"deploy"}`},
		{`{"has":["a.b.c"]}`, `{"a":{"b":{"c":1}}}`},
		{`null`, `{}`},
		{`{}`, `{"key":"val"}`},
		{`{"eq":[["a","1"]]}`, `{"a":1e1000}`},
		{`{"eq":`, `{"a":1}`},
		{`{"ne":[["x","y"]]}`, `{"x":`},
		{strings.Repeat(`{"has":["a"]}`, 1), `[1,2,3]`},
	}
	for _, s := range seeds {
		f.Add(s.filter, s.payload)
	}
	f.Fuzz(func(t *testing.T, filter, payload string) {
		wantMatch, wantErr := Eval(json.RawMessage(filter), json.RawMessage(payload))
		gotMatch, gotErr := EvalParsed(json.RawMessage(filter), NewParsedPayload(json.RawMessage(payload)))
		if gotMatch != wantMatch {
			t.Fatalf("match mismatch: Eval=%v EvalParsed=%v (filter=%q payload=%q)", wantMatch, gotMatch, filter, payload)
		}
		if errString(gotErr) != errString(wantErr) {
			t.Fatalf("error mismatch: Eval=%q EvalParsed=%q (filter=%q payload=%q)", errString(wantErr), errString(gotErr), filter, payload)
		}
	})
}
