package domain

import (
	"encoding/json"
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
