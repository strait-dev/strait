package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSingletonKind_IsValid(t *testing.T) {
	tests := []struct {
		kind SingletonKind
		want bool
	}{
		{SingletonKindJob, true},
		{SingletonKindWorkflow, true},
		{SingletonKind(""), false},
		{SingletonKind("task"), false},
		{SingletonKind("JOB"), false},
	}
	for _, tt := range tests {
		if got := tt.kind.IsValid(); got != tt.want {
			t.Errorf("SingletonKind(%q).IsValid() = %v, want %v", tt.kind, got, tt.want)
		}
	}
}

func TestSingletonOnConflict_Valid(t *testing.T) {
	tests := []struct {
		policy SingletonOnConflict
		want   bool
	}{
		{SingletonOnConflictQueue, true},
		{SingletonOnConflictDrop, true},
		{SingletonOnConflictReplace, true},
		{SingletonOnConflict(""), false},
		{SingletonOnConflict("skip"), false},
		{SingletonOnConflict("QUEUE"), false},
	}
	for _, tt := range tests {
		if got := tt.policy.Valid(); got != tt.want {
			t.Errorf("SingletonOnConflict(%q).Valid() = %v, want %v", tt.policy, got, tt.want)
		}
	}
}

func TestSingletonOutcome_IsValid(t *testing.T) {
	tests := []struct {
		outcome SingletonOutcome
		want    bool
	}{
		{SingletonOutcomeDispatched, true},
		{SingletonOutcomeQueuedBehind, true},
		{SingletonOutcomeDropped, true},
		{SingletonOutcomeReplaced, true},
		{SingletonOutcome(""), false},
		{SingletonOutcome("pending"), false},
	}
	for _, tt := range tests {
		if got := tt.outcome.IsValid(); got != tt.want {
			t.Errorf("SingletonOutcome(%q).IsValid() = %v, want %v", tt.outcome, got, tt.want)
		}
	}
}

func TestParseSingletonKeyExpr(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantZero bool
		wantTpl  string
		wantErr  bool
	}{
		{name: "empty bytes", raw: "", wantZero: true},
		{name: "null literal", raw: "null", wantZero: true},
		{name: "valid template", raw: `{"template":"${user.id}"}`, wantTpl: "${user.id}"},
		{name: "constant template", raw: `{"template":"global"}`, wantTpl: "global"},
		{name: "empty template rejected", raw: `{"template":""}`, wantErr: true},
		{name: "unknown field rejected", raw: `{"template":"x","extra":1}`, wantErr: true},
		{name: "malformed json rejected", raw: `{"template":`, wantErr: true},
		{name: "not an object rejected", raw: `"just a string"`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := ParseSingletonKeyExpr(json.RawMessage(tt.raw))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseSingletonKeyExpr(%q) expected error, got nil", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSingletonKeyExpr(%q) unexpected error: %v", tt.raw, err)
			}
			if expr.IsZero() != tt.wantZero {
				t.Errorf("IsZero() = %v, want %v", expr.IsZero(), tt.wantZero)
			}
			if !tt.wantZero && expr.Template != tt.wantTpl {
				t.Errorf("Template = %q, want %q", expr.Template, tt.wantTpl)
			}
		})
	}
}

func TestSingletonKeyExpr_ValidateLength(t *testing.T) {
	long := `{"template":"` + strings.Repeat("a", maxSingletonTemplateLen+1) + `"}`
	if _, err := ParseSingletonKeyExpr(json.RawMessage(long)); err == nil {
		t.Fatal("expected error for over-length template, got nil")
	}

	atLimit := `{"template":"` + strings.Repeat("a", maxSingletonTemplateLen) + `"}`
	if _, err := ParseSingletonKeyExpr(json.RawMessage(atLimit)); err != nil {
		t.Fatalf("template at limit should be valid, got error: %v", err)
	}
}

func TestSingletonKeyExpr_JSONRoundTrip(t *testing.T) {
	original := SingletonKeyExpr{Template: "${account.id}-${region}"}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, err := ParseSingletonKeyExpr(raw)
	if err != nil {
		t.Fatalf("parse round-trip: %v", err)
	}
	if parsed.Template != original.Template {
		t.Errorf("round-trip Template = %q, want %q", parsed.Template, original.Template)
	}
}

func TestResolveSingletonKey(t *testing.T) {
	tests := []struct {
		name    string
		tpl     string
		zero    bool
		payload string
		want    string
		wantErr bool
	}{
		{name: "zero expr", zero: true, payload: `{"id":"1"}`, want: ""},
		{name: "constant key", tpl: "global", payload: `{}`, want: "global"},
		{name: "single interpolation", tpl: "${id}", payload: `{"id":"acct-7"}`, want: "acct-7"},
		{name: "multi interpolation", tpl: "${account.id}-${region}", payload: `{"account":{"id":"a1"},"region":"us"}`, want: "a1-us"},
		{name: "nested path", tpl: "${a.b.c}", payload: `{"a":{"b":{"c":"deep"}}}`, want: "deep"},
		{name: "literal plus interpolation", tpl: "job:${id}:run", payload: `{"id":"42"}`, want: "job:42:run"},
		{name: "number scalar", tpl: "${n}", payload: `{"n":42}`, want: "42"},
		{name: "float scalar", tpl: "${n}", payload: `{"n":1.5}`, want: "1.5"},
		// Large integer ids must round-trip exactly: decoded as float64 these
		// would lose precision and two distinct ids could collide on one key.
		{name: "large integer id exact", tpl: "${id}", payload: `{"id":12345678901234567890}`, want: "12345678901234567890"},
		{name: "very large integer id exact", tpl: "${id}", payload: `{"id":9007199254740993}`, want: "9007199254740993"},
		{name: "bool scalar", tpl: "${flag}", payload: `{"flag":true}`, want: "true"},
		{name: "whitespace trimmed in path", tpl: "${ id }", payload: `{"id":"x"}`, want: "x"},
		{name: "missing path errors", tpl: "${missing}", payload: `{"id":"1"}`, wantErr: true},
		{name: "non-scalar object errors", tpl: "${account}", payload: `{"account":{"id":"1"}}`, wantErr: true},
		{name: "non-scalar array errors", tpl: "${list}", payload: `{"list":[1,2]}`, wantErr: true},
		{name: "null value errors", tpl: "${id}", payload: `{"id":null}`, wantErr: true},
		{name: "empty interpolation path errors", tpl: "${}", payload: `{}`, wantErr: true},
		{name: "interpolation against non-object payload errors", tpl: "${id}", payload: `"a string"`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var expr SingletonKeyExpr
			if !tt.zero {
				expr = SingletonKeyExpr{Template: tt.tpl}
			}
			got, err := ResolveSingletonKey(expr, json.RawMessage(tt.payload))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveSingletonKey(%q, %q) expected error, got nil (key=%q)", tt.tpl, tt.payload, got)
				}
				if !errors.Is(err, ErrSingletonKeyUnresolvable) {
					t.Errorf("error = %v, want ErrSingletonKeyUnresolvable", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveSingletonKey(%q, %q) unexpected error: %v", tt.tpl, tt.payload, err)
			}
			if got != tt.want {
				t.Errorf("ResolveSingletonKey(%q, %q) = %q, want %q", tt.tpl, tt.payload, got, tt.want)
			}
		})
	}
}

func TestResolveSingletonKey_OverLength(t *testing.T) {
	expr := SingletonKeyExpr{Template: "${big}"}
	payload := `{"big":"` + strings.Repeat("a", maxResolvedSingletonKeyLen+1) + `"}`
	if _, err := ResolveSingletonKey(expr, json.RawMessage(payload)); err == nil {
		t.Fatal("expected error for over-length resolved key, got nil")
	}
}

// TestResolveSingletonKey_LargeIntegerKeysDistinct is the precision regression:
// two integer ids that differ only past float64's 53-bit mantissa must resolve to
// distinct keys. Decoded as float64 they would both round to the same value and
// silently share one singleton lock.
func TestResolveSingletonKey_LargeIntegerKeysDistinct(t *testing.T) {
	expr := SingletonKeyExpr{Template: "acct:${id}"}

	k1, err := ResolveSingletonKey(expr, json.RawMessage(`{"id":9007199254740993}`))
	if err != nil {
		t.Fatalf("resolve id1 error = %v", err)
	}
	k2, err := ResolveSingletonKey(expr, json.RawMessage(`{"id":9007199254740994}`))
	if err != nil {
		t.Fatalf("resolve id2 error = %v", err)
	}
	if k1 == k2 {
		t.Fatalf("adjacent large integer ids collided on key %q (precision lost)", k1)
	}
	if k1 != "acct:9007199254740993" {
		t.Errorf("k1 = %q, want acct:9007199254740993", k1)
	}
	if k2 != "acct:9007199254740994" {
		t.Errorf("k2 = %q, want acct:9007199254740994", k2)
	}
}

func FuzzResolveSingletonKey(f *testing.F) {
	f.Add([]byte(`{"template":"${user.id}"}`), []byte(`{"user":{"id":"42"}}`))
	f.Add([]byte(`{"template":"global"}`), []byte(`{}`))
	f.Add([]byte(`{"template":"${missing}"}`), []byte(`{"id":"1"}`))
	f.Add([]byte(`{"template":"${}"}`), []byte(`{}`))
	f.Add([]byte(`null`), []byte(`{}`))
	f.Add([]byte(``), []byte(`null`))
	f.Add([]byte(`{"template":"${a.b.c}"}`), []byte(`not json`))
	f.Add([]byte(`{"template":"${n}"}`), []byte(`{"n":1.5}`))

	f.Fuzz(func(t *testing.T, rawExpr, payload []byte) {
		// ParseSingletonKeyExpr + ResolveSingletonKey must never panic.
		expr, err := ParseSingletonKeyExpr(json.RawMessage(rawExpr))
		if err != nil {
			return
		}
		key, rerr := ResolveSingletonKey(expr, json.RawMessage(payload))
		if rerr == nil && len(key) > maxResolvedSingletonKeyLen {
			t.Fatalf("resolved key exceeds max length: %d", len(key))
		}
	})
}

func FuzzSingletonKeyExprUnmarshal(f *testing.F) {
	f.Add([]byte(`{"template":"${user.id}"}`))
	f.Add([]byte(`{"template":""}`))
	f.Add([]byte(`null`))
	f.Add([]byte(``))
	f.Add([]byte(`{"template":"x","extra":true}`))
	f.Add([]byte(`not json`))
	f.Add([]byte(`{"template":`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`12345`))

	f.Fuzz(func(t *testing.T, raw []byte) {
		// ParseSingletonKeyExpr must never panic regardless of input.
		expr, err := ParseSingletonKeyExpr(json.RawMessage(raw))
		if err == nil && !expr.IsZero() {
			// A successfully-parsed non-zero expression must satisfy its own invariants.
			if vErr := expr.Validate(); vErr != nil {
				t.Fatalf("parsed expr failed its own Validate(): %v", vErr)
			}
		}
	})
}
