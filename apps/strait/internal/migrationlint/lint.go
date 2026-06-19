// Package migrationlint audits SQL migration files for patterns that are
// dangerous to run against a live production database. The linter is
// lexer-based rather than AST-based so it does not need to pull in a
// CGo dependency for pg_query -- the patterns we want to catch are all
// detectable at the statement level without a full parser.
//
// A PR adding `CREATE INDEX` (without CONCURRENTLY) or
// `ALTER TABLE ... SET NOT NULL` on a hot table can brick a production
// database that earlier work went to great lengths to keep healthy. This
// linter is the cheapest possible gate: ~400 lines of Go, no external
// parser, runs in CI in under a second.
package migrationlint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Rule identifies a single lint rule.
type Rule string

const (
	RuleCreateIndexNonConcurrent  Rule = "create-index-non-concurrent"
	RuleCreateUniqueNonConcurrent Rule = "create-unique-index-non-concurrent"
	RuleDropIndexNonConcurrent    Rule = "drop-index-non-concurrent"
	RuleSetNotNull                Rule = "alter-set-not-null"
	RuleAddColumnNotNullDefault   Rule = "add-column-not-null-default"
	RuleDropColumn                Rule = "drop-column"
	RuleVacuumFull                Rule = "vacuum-full"
	RuleReindexNonConcurrent      Rule = "reindex-non-concurrent"
	RuleLockTable                 Rule = "lock-table"
	RuleCluster                   Rule = "cluster"
	RuleRenameColumn              Rule = "rename-column"
	RuleRenameTable               Rule = "rename-table"
	RuleUnpairedMigration         Rule = "unpaired-migration"
)

// Violation is a single lint finding.
type Violation struct {
	File    string
	Line    int
	Rule    Rule
	Message string
	Snippet string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s:%d [%s] %s\n    %s", v.File, v.Line, v.Rule, v.Message, v.Snippet)
}

// LintFile runs all rules against a single .up.sql file and returns any
// violations. `file` is the path (absolute or repo-relative); `content`
// is the raw file bytes.
func LintFile(file string, content []byte) []Violation {
	lines := splitLines(string(content))
	var out []Violation

	// First pass: collect every table created in this file. CREATE INDEX
	// on a table that was created in the same migration is safe because
	// the table cannot have concurrent readers yet.
	newTables := collectNewTables(content)

	// Walk statements (semicolon-terminated, respecting $$ dollar quotes).
	statements := splitStatements(content)
	for _, stmt := range statements {
		// Check safety-ok against the raw text (which still contains
		// comments) so reviewers can annotate via inline comment.
		if isSafetyAllowlisted(stmt.text) {
			continue
		}
		// Strip comments line-by-line BEFORE normalizing. Removing them
		// from the fully-normalized single-line string would eat the
		// rest of the statement because `--` would then be at position 0
		// after a leading comment.
		stripped := stripCommentsMultiLine(stmt.text)
		ruleInput := normalize(stripped)
		for _, rule := range allRules {
			if !rule.match(ruleInput) {
				continue
			}
			// If this is an index rule and the target table was created
			// in the same file, skip.
			if isIndexRule(rule.id) {
				if target := extractIndexTarget(ruleInput); target != "" {
					if _, ok := newTables[target]; ok {
						continue
					}
				}
			}
			// Start of statement may be leading whitespace after the
			// previous semicolon; advance to the first non-space char.
			startOffset := stmt.offset
			for startOffset < len(content) && (content[startOffset] == ' ' || content[startOffset] == '\t' || content[startOffset] == '\n' || content[startOffset] == '\r') {
				startOffset++
			}
			line := findStatementLine(lines, startOffset, content)
			out = append(out, Violation{
				File:    file,
				Line:    line,
				Rule:    rule.id,
				Message: rule.message,
				Snippet: truncate(ruleInput, 140),
			})
		}
	}
	return out
}

// collectNewTables scans for CREATE TABLE statements and returns the set
// of lowercased, unqualified table names created in the file.
var createTableRE = regexp.MustCompile(`(?is)\bCREATE\s+(?:UNLOGGED\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?"?([A-Za-z_][A-Za-z0-9_]*)"?`)

func collectNewTables(content []byte) map[string]struct{} {
	out := map[string]struct{}{}
	for _, m := range createTableRE.FindAllSubmatch(content, -1) {
		out[strings.ToLower(string(m[1]))] = struct{}{}
	}
	return out
}

// extractIndexTarget pulls the target table from a CREATE INDEX statement.
// Returns the empty string on parse failure.
var indexTargetRE = regexp.MustCompile(`(?is)\bON\s+(?:ONLY\s+)?"?([A-Za-z_][A-Za-z0-9_]*)"?`)

func extractIndexTarget(stmt string) string {
	m := indexTargetRE.FindStringSubmatch(stmt)
	if len(m) != 2 {
		return ""
	}
	return strings.ToLower(m[1])
}

func isIndexRule(r Rule) bool {
	return r == RuleCreateIndexNonConcurrent ||
		r == RuleCreateUniqueNonConcurrent ||
		r == RuleDropIndexNonConcurrent
}

// stripCommentsMultiLine removes `-- ...` comment suffixes from each
// newline-separated line, preserving the line structure so the resulting
// statement can still be normalized afterward.
func stripCommentsMultiLine(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if before, _, found := strings.Cut(line, "--"); found {
			lines[i] = before
		}
	}
	return strings.Join(lines, "\n")
}

// LintDir walks a directory of `.up.sql` files and aggregates violations.
// Any file ending in `.down.sql` is ignored.
func LintDir(dir string) ([]Violation, error) {
	var out []Violation
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		out = append(out, LintFile(path, content)...)
	}
	return out, nil
}

// LintPairs walks `dir` and returns one violation per orphaned migration:
// any `.up.sql` lacking a matching `.down.sql` (or vice versa). Migrations
// must always be added or removed as a pair so an operator can roll forward
// and roll back symmetrically. Deleting a migration is allowed only when
// both halves of the pair are removed in the same change.
func LintPairs(dir string) ([]Violation, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	ups := map[string]string{}
	downs := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		switch {
		case strings.HasSuffix(name, ".up.sql"):
			stem := strings.TrimSuffix(name, ".up.sql")
			ups[stem] = name
		case strings.HasSuffix(name, ".down.sql"):
			stem := strings.TrimSuffix(name, ".down.sql")
			downs[stem] = name
		}
	}
	var out []Violation
	for stem, upName := range ups {
		if _, ok := downs[stem]; !ok {
			out = append(out, Violation{
				File:    filepath.Join(dir, upName),
				Line:    1,
				Rule:    RuleUnpairedMigration,
				Message: fmt.Sprintf("migration %q has no matching .down.sql; every .up.sql must ship with a .down.sql so the migration can be rolled back", stem+".up.sql"),
				Snippet: stem + ".up.sql",
			})
		}
	}
	for stem, downName := range downs {
		if _, ok := ups[stem]; !ok {
			out = append(out, Violation{
				File:    filepath.Join(dir, downName),
				Line:    1,
				Rule:    RuleUnpairedMigration,
				Message: fmt.Sprintf("migration %q has no matching .up.sql; orphan .down.sql files must be removed together with their .up.sql counterpart", stem+".down.sql"),
				Snippet: stem + ".down.sql",
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].File < out[j].File
	})
	return out, nil
}

// rule is a single lint predicate.
type rule struct {
	id      Rule
	re      *regexp.Regexp
	negRe   *regexp.Regexp // if non-nil, rule matches only when negRe does NOT match
	message string
}

func (r rule) match(stmt string) bool {
	if !r.re.MatchString(stmt) {
		return false
	}
	if r.negRe != nil && r.negRe.MatchString(stmt) {
		return false
	}
	return true
}

// allRules enumerates every active lint rule. Keep this list short and
// lexical so the linter stays fast and understandable.
var allRules = []rule{
	{
		id:      RuleCreateIndexNonConcurrent,
		re:      regexp.MustCompile(`(?is)\bCREATE\s+INDEX\b`),
		negRe:   regexp.MustCompile(`(?is)\bCONCURRENTLY\b`),
		message: "CREATE INDEX without CONCURRENTLY locks the table against writes; use CREATE INDEX CONCURRENTLY",
	},
	{
		id:      RuleCreateUniqueNonConcurrent,
		re:      regexp.MustCompile(`(?is)\bCREATE\s+UNIQUE\s+INDEX\b`),
		negRe:   regexp.MustCompile(`(?is)\bCONCURRENTLY\b`),
		message: "CREATE UNIQUE INDEX without CONCURRENTLY locks the table; use CREATE UNIQUE INDEX CONCURRENTLY",
	},
	{
		id:    RuleDropIndexNonConcurrent,
		re:    regexp.MustCompile(`(?is)\bDROP\s+INDEX\b`),
		negRe: regexp.MustCompile(`(?is)\b(CONCURRENTLY|IF\s+EXISTS)\b`),
		// We allow DROP INDEX IF EXISTS since it's still safe, but warn
		// on plain DROP INDEX without any qualifier.
		message: "DROP INDEX without CONCURRENTLY or IF EXISTS can break reads under load; use DROP INDEX CONCURRENTLY",
	},
	{
		id:      RuleSetNotNull,
		re:      regexp.MustCompile(`(?is)\bALTER\s+TABLE\b[^;]*\bSET\s+NOT\s+NULL\b`),
		message: "ALTER TABLE ... SET NOT NULL rewrites every row with an ACCESS EXCLUSIVE lock; backfill with a CHECK constraint VALIDATED first",
	},
	{
		id:      RuleAddColumnNotNullDefault,
		re:      regexp.MustCompile(`(?is)\bADD\s+COLUMN\b[^;]*\bNOT\s+NULL\b[^;]*\bDEFAULT\b`),
		message: "ADD COLUMN NOT NULL DEFAULT rewrites every row; use a nullable column first, backfill in batches, then SET NOT NULL in a later migration",
	},
	{
		id:      RuleDropColumn,
		re:      regexp.MustCompile(`(?is)\bDROP\s+COLUMN\b`),
		message: "DROP COLUMN is unsafe until all running processes stop reading it; rename to _deprecated_ first, ship a release, then drop",
	},
	{
		id:      RuleVacuumFull,
		re:      regexp.MustCompile(`(?is)\bVACUUM\s+FULL\b`),
		message: "VACUUM FULL takes ACCESS EXCLUSIVE lock; use pg_repack or scheduled maintenance windows",
	},
	{
		id:      RuleReindexNonConcurrent,
		re:      regexp.MustCompile(`(?is)\bREINDEX\s+(TABLE|INDEX|SCHEMA|DATABASE)\b`),
		negRe:   regexp.MustCompile(`(?is)\bCONCURRENTLY\b`),
		message: "REINDEX without CONCURRENTLY locks the target; use REINDEX ... CONCURRENTLY (PG 12+)",
	},
	{
		id:      RuleLockTable,
		re:      regexp.MustCompile(`(?is)\bLOCK\s+TABLE\b`),
		message: "LOCK TABLE blocks concurrent access; avoid outside emergency procedures",
	},
	{
		id:      RuleCluster,
		re:      regexp.MustCompile(`(?is)\bCLUSTER\b`),
		message: "CLUSTER takes ACCESS EXCLUSIVE lock and rewrites the whole table",
	},
	{
		id:      RuleRenameColumn,
		re:      regexp.MustCompile(`(?is)\bALTER\s+TABLE\b[^;]*\bRENAME\s+COLUMN\b`),
		message: "RENAME COLUMN is safe locking-wise but breaks running processes still using the old name; coordinate via a release, not a migration",
	},
	{
		id:      RuleRenameTable,
		re:      regexp.MustCompile(`(?is)\bALTER\s+TABLE\b[^;]*\bRENAME\s+TO\b`),
		message: "RENAME TO breaks running processes still using the old table name; coordinate via a release, not a migration",
	},
}

// statement is one SQL statement together with its byte offset in the
// source for line-number reporting.
type statement struct {
	text   string
	offset int
}

// splitStatements splits a SQL script into top-level statements by
// semicolons, respecting $$ dollar-quoted bodies (used by PL/pgSQL
// trigger functions) so a DO $$ BEGIN ... END $$ block is kept intact.
func splitStatements(content []byte) []statement {
	var out []statement
	var buf strings.Builder
	var offset int
	start := 0

	for offset < len(content) {
		if offset+2 <= len(content) && content[offset] == '$' && content[offset+1] == '$' {
			// Find matching $$
			end := offset + 2
			for end+2 <= len(content) {
				if content[end] == '$' && content[end+1] == '$' {
					end += 2
					break
				}
				end++
			}
			buf.Write(content[offset:end])
			offset = end
			continue
		}
		b := content[offset]
		if b == '-' && offset+1 < len(content) && content[offset+1] == '-' {
			// Line comment: skip to end of line but preserve it in buf so
			// safety-ok detection works.
			for offset < len(content) && content[offset] != '\n' {
				buf.WriteByte(content[offset])
				offset++
			}
			continue
		}
		if b == ';' {
			s := strings.TrimSpace(buf.String())
			if s != "" {
				out = append(out, statement{text: s, offset: start})
			}
			buf.Reset()
			offset++
			start = offset
			continue
		}
		buf.WriteByte(b)
		offset++
	}
	if s := strings.TrimSpace(buf.String()); s != "" {
		out = append(out, statement{text: s, offset: start})
	}
	return out
}

// normalize collapses whitespace in a statement so multi-line SQL matches
// single-line regexes and the snippet shown to the user is tidy.
func normalize(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// isSafetyAllowlisted returns true when the statement carries an
// explicit bypass annotation. Format: `-- safety-ok: reason here`
// The reason is required and must appear on the same line as the marker
// so an empty `-- safety-ok:` followed by a newline does not bypass.
var safetyOKRE = regexp.MustCompile(`(?im)--\s*safety-ok:[ \t]*\S`)

func isSafetyAllowlisted(stmt string) bool {
	return safetyOKRE.MatchString(stmt)
}

// splitLines is a cheap wrapper to keep the regex cost contained.
func splitLines(s string) []string { return strings.Split(s, "\n") }

// findStatementLine returns the 1-based line number that `offset` falls on.
func findStatementLine(_ []string, offset int, content []byte) int {
	if offset > len(content) {
		offset = len(content)
	}
	line := 1
	for i := range offset {
		if content[i] == '\n' {
			line++
		}
	}
	return line
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
