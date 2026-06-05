package api

import (
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/require"
)

// TestSanitizeCSVCellHandlesFormatChars pins the contract for
// invisible Unicode format characters that hide formula injection from
// human review: a cell that starts with a zero-width space, LRM/RLM,
// ZWJ, soft hyphen, or combining mark, followed by =, +, -, or @, must
// still be prefixed with a single quote so spreadsheet apps treat it as
// text.
func TestSanitizeCSVCellHandlesFormatChars(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
	}{
		{"zwsp_equals", "\u200b=SUM(1)"},
		{"lrm_equals", "\u200e=cmd|/c calc"},
		{"rlm_plus", "\u200f+1+1"},
		{"zwj_equals", "\u200d=A1"},
		{"bom_equals", "\uFEFF=A1"},
		{"soft_hyphen_minus", "\u00ad-1"},
		{"combining_acute_at", "\u0301@cmd"},
		{"two_format_then_equals", "\u200b\u200e=evil"},
		{"space_then_equals", " =leak"},
		{"tab_first", "\tcmd"},
		{"newline_first", "\ncmd"},
		{"cr_first", "\rcmd"},
		{"nul_first", "\x00=cmd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := sanitizeCSVCell(tc.in)
			require.True(
				t, strings.HasPrefix(out,

					"'"))

		})
	}
}

// TestSanitizeCSVCellPreservesBenignText regresses the happy
// path: ordinary text must not be prefixed.
func TestSanitizeCSVCellPreservesBenignText(t *testing.T) {
	t.Parallel()

	cases := []string{
		"hello",
		"Hello, world",
		"42",
		"3.14",
		"abc-def",
		"a@example.com",
		"  trimmed  ",
		"日本語",
		"\u200btext",
		"\uFEFFbenign",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			out := sanitizeCSVCell(in)
			require.False(t, strings.HasPrefix(
				out,

				"'"))
			require.Equal(t, in, out)

		})
	}
}

// FuzzSanitizeCSVCellNeverLeavesFormulaPrefix is the
// adversarial guard: after sanitization, the first rune that is NOT a
// leading single-quote, format, mark, BOM, or whitespace/control char
// must never be one of '=+-@'.
func FuzzSanitizeCSVCellNeverLeavesFormulaPrefix(f *testing.F) {
	seeds := []string{
		"\u200b=A1",
		"\u200e+leak",
		"\u200f-1",
		"\u00ad@cmd",
		"=plain",
		"+plain",
		"-plain",
		"@plain",
		"benign",
		"",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, in string) {
		out := sanitizeCSVCell(in)
		if strings.HasPrefix(out, "'") {
			return
		}
		for _, r := range out {
			if r == '\uFEFF' || unicode.IsSpace(r) || unicode.IsControl(r) ||
				unicode.Is(unicode.Cf, r) || unicode.IsMark(r) {
				continue
			}
			switch r {
			case '=', '+', '-', '@':
				require.Failf(t, "test failure", "sanitizeCSVCell(%q) = %q left formula trigger %q visible", in, out, r)
			}
			break
		}
	})
}
