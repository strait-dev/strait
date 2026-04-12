package store

import (
	"fmt"
	"regexp"
)

// R4 Phase 6: SQL identifier validation.
//
// Several fmt.Sprintf SQL builders across the codebase interpolate
// table/partition names into DDL statements. Today those names come
// from controlled paths (partition name calculators, pg_class lookups)
// but a future refactor could open an injection surface. These helpers
// enforce the constraint at the call site.

// identRE matches a valid unqualified Postgres identifier: starts with
// a letter or underscore, followed by letters, digits, or underscores.
var identRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// ValidateIdent returns nil if s is a safe unqualified Postgres
// identifier, or an error describing the violation.
func ValidateIdent(s string) error {
	if s == "" {
		return fmt.Errorf("empty identifier")
	}
	if len(s) > 128 {
		return fmt.Errorf("identifier too long (%d chars)", len(s))
	}
	if !identRE.MatchString(s) {
		return fmt.Errorf("invalid identifier %q: must match [a-zA-Z_][a-zA-Z0-9_]*", s)
	}
	return nil
}

// SafeQuoteIdent validates and double-quotes an identifier. Returns an
// error if the identifier is invalid. Prefer this over the unvalidated
// quoteIdent helper in partition code.
func SafeQuoteIdent(s string) (string, error) {
	if err := ValidateIdent(s); err != nil {
		return "", err
	}
	return `"` + s + `"`, nil
}
