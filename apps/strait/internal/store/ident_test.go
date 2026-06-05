package store

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateIdent_Valid(t *testing.T) {
	valid := []string{
		"job_runs", "job_runs_p2026_04", "_private", "a", "A123",
		"idx_runs_queue", "schema_version",
	}
	for _, s := range valid {
		assert.NoError(
			t, ValidateIdent(s))
	}
}

func TestValidateIdent_Invalid(t *testing.T) {
	invalid := []string{
		"", `"; DROP TABLE`, "123abc", "a-b", "a.b",
		"a b", "a\tb", "a\nb",
	}
	for _, s := range invalid {
		assert.Error(t,
			ValidateIdent(s))
	}
}

func TestValidateIdent_TooLong(t *testing.T) {
	long := strings.Repeat("a", 129)
	assert.Error(t,
		ValidateIdent(long))
}

func TestSafeQuoteIdent_HappyPath(t *testing.T) {
	got, err := SafeQuoteIdent("job_runs_p2026_04")
	require.NoError(t, err)
	assert.Equal(t,
		`"job_runs_p2026_04"`,

		got)
}

func TestSafeQuoteIdent_RejectsInjection(t *testing.T) {
	if _, err := SafeQuoteIdent(`"; DROP TABLE`); err == nil {
		assert.Fail(t,

			"expected rejection")
	}
}

func TestSafeQuoteIdent_InjectionPayloads(t *testing.T) {
	payloads := []string{
		"'; DROP TABLE job_runs;--",
		`" OR 1=1--`,
		"job_runs; DELETE FROM job_runs",
		"a\x00b",
		"table_name\nDROP TABLE",
		"$(whoami)",
		"UNION SELECT * FROM pg_shadow",
	}
	for _, p := range payloads {
		if _, err := SafeQuoteIdent(p); err == nil {
			assert.Failf(t, "test failure",

				"SafeQuoteIdent(%q) should reject injection payload", p)
		}
	}
}

func FuzzValidateIdent(f *testing.F) {
	f.Add("job_runs")
	f.Add("")
	f.Add(`"; DROP TABLE`)
	f.Add("a\x00b")
	f.Fuzz(func(t *testing.T, s string) {
		defer func() {
			require.Nil(t, recover())
		}()
		_ = ValidateIdent(s)
	})
}
