package store

import "strings"

var postgresLikePatternEscaper = strings.NewReplacer(
	`\`, `\\`,
	`%`, `\%`,
	`_`, `\_`,
)

// EscapePostgresLikePattern escapes user-controlled text for a PostgreSQL LIKE
// pattern that uses backslash as its ESCAPE character.
func EscapePostgresLikePattern(s string) string {
	return postgresLikePatternEscaper.Replace(s)
}
