package store

import (
	"strings"
	"testing"
)

func TestValidateIdent_Valid(t *testing.T) {
	valid := []string{
		"job_runs", "job_runs_p2026_04", "_private", "a", "A123",
		"idx_runs_queue", "schema_version",
	}
	for _, s := range valid {
		if err := ValidateIdent(s); err != nil {
			t.Errorf("ValidateIdent(%q) = %v, want nil", s, err)
		}
	}
}

func TestValidateIdent_Invalid(t *testing.T) {
	invalid := []string{
		"", `"; DROP TABLE`, "123abc", "a-b", "a.b",
		"a b", "a\tb", "a\nb",
	}
	for _, s := range invalid {
		if err := ValidateIdent(s); err == nil {
			t.Errorf("ValidateIdent(%q) = nil, want error", s)
		}
	}
}

func TestValidateIdent_TooLong(t *testing.T) {
	long := strings.Repeat("a", 129)
	if err := ValidateIdent(long); err == nil {
		t.Error("expected too-long error")
	}
}

func TestSafeQuoteIdent_HappyPath(t *testing.T) {
	got, err := SafeQuoteIdent("job_runs_p2026_04")
	if err != nil {
		t.Fatal(err)
	}
	if got != `"job_runs_p2026_04"` {
		t.Errorf("got %q", got)
	}
}

func TestSafeQuoteIdent_RejectsInjection(t *testing.T) {
	if _, err := SafeQuoteIdent(`"; DROP TABLE`); err == nil {
		t.Error("expected rejection")
	}
}

func FuzzValidateIdent(f *testing.F) {
	f.Add("job_runs")
	f.Add("")
	f.Add(`"; DROP TABLE`)
	f.Add("a\x00b")
	f.Fuzz(func(t *testing.T, s string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic on %q: %v", s, r)
			}
		}()
		_ = ValidateIdent(s)
	})
}
