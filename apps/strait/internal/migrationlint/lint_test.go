package migrationlint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Unit tests for the migration safety linter.

// baselineCutoff is the migration number up to which historical violations
// are tolerated. New migrations (> this number) must pass clean. Raise the
// cutoff over time as legacy migrations get annotated with `-- safety-ok`.
const baselineCutoff = 196

func TestLint_NewMigrationsPassClean(t *testing.T) {
	dir := filepath.Join("..", "..", "migrations")
	violations, err := LintDir(dir)
	if err != nil {
		t.Fatalf("LintDir: %v", err)
	}
	var newViolations []Violation
	for _, v := range violations {
		base := filepath.Base(v.File)
		if len(base) < 6 {
			continue
		}
		// File name format is NNNNNN_name.up.sql.
		var num int
		if _, err := fmtSscan(base, &num); err != nil {
			continue
		}
		if num > baselineCutoff {
			newViolations = append(newViolations, v)
		}
	}
	if len(newViolations) > 0 {
		for _, v := range newViolations {
			t.Errorf("new migration violation: %s", v.String())
		}
	}
}

func fmtSscan(name string, out *int) (int, error) {
	// Extract leading digits from a migration filename like
	// 000191_something.up.sql without pulling in fmt.Sscanf's cost.
	var n, consumed int
	for i := range len(name) {
		c := name[i]
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
		consumed++
	}
	if consumed == 0 {
		return 0, errFmt
	}
	*out = n
	return consumed, nil
}

var errFmt = &fmtErr{"no leading digits"}

type fmtErr struct{ msg string }

func (e *fmtErr) Error() string { return e.msg }

func TestLint_CreateIndexWithoutConcurrently(t *testing.T) {
	sql := `CREATE INDEX idx_bad ON job_runs (id);`
	v := LintFile("test.up.sql", []byte(sql))
	if !hasRule(v, RuleCreateIndexNonConcurrent) {
		t.Errorf("expected %s, got %v", RuleCreateIndexNonConcurrent, v)
	}
}

func TestLint_CreateIndexConcurrentlyAccepted(t *testing.T) {
	sql := `CREATE INDEX CONCURRENTLY idx_good ON job_runs (id);`
	v := LintFile("test.up.sql", []byte(sql))
	if hasRule(v, RuleCreateIndexNonConcurrent) {
		t.Errorf("CONCURRENTLY should pass: %v", v)
	}
}

func TestLint_CreateIndexInTransactionalMigrationIsStillFlagged(t *testing.T) {
	// Even inside BEGIN/COMMIT the linter should still flag.
	sql := `BEGIN;
CREATE INDEX idx_bad ON job_runs (id);
COMMIT;`
	v := LintFile("test.up.sql", []byte(sql))
	if !hasRule(v, RuleCreateIndexNonConcurrent) {
		t.Errorf("expected rule, got %v", v)
	}
}

func TestLint_CreateUniqueIndexWithoutConcurrently(t *testing.T) {
	sql := `CREATE UNIQUE INDEX idx_bad ON users (email);`
	v := LintFile("test.up.sql", []byte(sql))
	if !hasRule(v, RuleCreateUniqueNonConcurrent) {
		t.Errorf("expected %s, got %v", RuleCreateUniqueNonConcurrent, v)
	}
}

func TestLint_SetNotNull(t *testing.T) {
	sql := `ALTER TABLE users ALTER COLUMN email SET NOT NULL;`
	v := LintFile("test.up.sql", []byte(sql))
	if !hasRule(v, RuleSetNotNull) {
		t.Errorf("expected %s, got %v", RuleSetNotNull, v)
	}
}

func TestLint_AddColumnNotNullDefault(t *testing.T) {
	sql := `ALTER TABLE users ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();`
	v := LintFile("test.up.sql", []byte(sql))
	if !hasRule(v, RuleAddColumnNotNullDefault) {
		t.Errorf("expected %s, got %v", RuleAddColumnNotNullDefault, v)
	}
}

func TestLint_AddColumnNullableAccepted(t *testing.T) {
	sql := `ALTER TABLE users ADD COLUMN nickname TEXT;`
	v := LintFile("test.up.sql", []byte(sql))
	if len(v) > 0 {
		t.Errorf("nullable column should pass: %v", v)
	}
}

func TestLint_DropColumn(t *testing.T) {
	sql := `ALTER TABLE users DROP COLUMN old;`
	v := LintFile("test.up.sql", []byte(sql))
	if !hasRule(v, RuleDropColumn) {
		t.Errorf("expected %s, got %v", RuleDropColumn, v)
	}
}

func TestLint_VacuumFull(t *testing.T) {
	sql := `VACUUM FULL job_runs;`
	v := LintFile("test.up.sql", []byte(sql))
	if !hasRule(v, RuleVacuumFull) {
		t.Errorf("expected %s, got %v", RuleVacuumFull, v)
	}
}

func TestLint_ReindexNonConcurrent(t *testing.T) {
	sql := `REINDEX TABLE job_runs;`
	v := LintFile("test.up.sql", []byte(sql))
	if !hasRule(v, RuleReindexNonConcurrent) {
		t.Errorf("expected %s, got %v", RuleReindexNonConcurrent, v)
	}
}

func TestLint_ReindexConcurrentlyAccepted(t *testing.T) {
	sql := `REINDEX INDEX CONCURRENTLY idx_ok;`
	v := LintFile("test.up.sql", []byte(sql))
	if hasRule(v, RuleReindexNonConcurrent) {
		t.Errorf("CONCURRENTLY should pass: %v", v)
	}
}

func TestLint_LockTable(t *testing.T) {
	sql := `LOCK TABLE job_runs IN ACCESS EXCLUSIVE MODE;`
	v := LintFile("test.up.sql", []byte(sql))
	if !hasRule(v, RuleLockTable) {
		t.Errorf("expected %s", RuleLockTable)
	}
}

func TestLint_Cluster(t *testing.T) {
	sql := `CLUSTER job_runs USING idx_runs_queue;`
	v := LintFile("test.up.sql", []byte(sql))
	if !hasRule(v, RuleCluster) {
		t.Errorf("expected %s", RuleCluster)
	}
}

func TestLint_RenameColumn(t *testing.T) {
	sql := `ALTER TABLE users RENAME COLUMN old_email TO email;`
	v := LintFile("test.up.sql", []byte(sql))
	if !hasRule(v, RuleRenameColumn) {
		t.Errorf("expected %s", RuleRenameColumn)
	}
}

func TestLint_RenameTable(t *testing.T) {
	sql := `ALTER TABLE users RENAME TO people;`
	v := LintFile("test.up.sql", []byte(sql))
	if !hasRule(v, RuleRenameTable) {
		t.Errorf("expected %s", RuleRenameTable)
	}
}

func TestLint_DropIndexBareFlagged(t *testing.T) {
	sql := `DROP INDEX idx_bad;`
	v := LintFile("test.up.sql", []byte(sql))
	if !hasRule(v, RuleDropIndexNonConcurrent) {
		t.Errorf("expected %s", RuleDropIndexNonConcurrent)
	}
}

func TestLint_DropIndexIfExistsAccepted(t *testing.T) {
	sql := `DROP INDEX IF EXISTS idx_ok;`
	v := LintFile("test.up.sql", []byte(sql))
	if hasRule(v, RuleDropIndexNonConcurrent) {
		t.Errorf("IF EXISTS should pass: %v", v)
	}
}

func TestLint_DropIndexConcurrentlyAccepted(t *testing.T) {
	sql := `DROP INDEX CONCURRENTLY idx_ok;`
	v := LintFile("test.up.sql", []byte(sql))
	if hasRule(v, RuleDropIndexNonConcurrent) {
		t.Errorf("CONCURRENTLY should pass: %v", v)
	}
}

func TestLint_SafetyOKBypass(t *testing.T) {
	sql := `-- safety-ok: brand new table, no readers yet
CREATE INDEX idx_new ON brand_new (id);`
	v := LintFile("test.up.sql", []byte(sql))
	if len(v) != 0 {
		t.Errorf("safety-ok should bypass, got %v", v)
	}
}

func TestLint_SafetyOKRequiresReasonOnSameLine(t *testing.T) {
	// `-- safety-ok:` with a reason on a subsequent line should still
	// flag: the reason must be on the same comment line as the marker
	// so reviewers can see it in context.
	sql := `-- safety-ok:
-- reason on next line
CREATE INDEX idx_new ON unrelated (id);`
	v := LintFile("test.up.sql", []byte(sql))
	if len(v) == 0 {
		t.Errorf("reason on next line should still flag; need inline reason")
	}
}

func TestLint_MultiStatementFileReportsAllViolations(t *testing.T) {
	sql := `CREATE INDEX a ON t (c);
VACUUM FULL t;`
	v := LintFile("test.up.sql", []byte(sql))
	if len(v) < 2 {
		t.Errorf("expected at least 2 violations, got %d", len(v))
	}
}

func TestLint_PlpgsqlBlockNotMisparsed(t *testing.T) {
	// A DO $$ BEGIN ... END $$ block containing CREATE INDEX should
	// still be flagged since the command inside would still run non-
	// concurrently.
	sql := `DO $$
BEGIN
  EXECUTE 'CREATE INDEX idx_bad ON t (c)';
END $$;`
	v := LintFile("test.up.sql", []byte(sql))
	if !hasRule(v, RuleCreateIndexNonConcurrent) {
		t.Errorf("DO block with CREATE INDEX should be flagged")
	}
}

func TestLint_CommentsStrippedForMatching(t *testing.T) {
	// A comment mentioning CREATE INDEX shouldn't flag.
	sql := `-- this migration does NOT create an index
ALTER TABLE t ADD COLUMN x INT;`
	v := LintFile("test.up.sql", []byte(sql))
	// It's fine for the linter to either skip comments or still match
	// them; we just want no false positive on safe ADD COLUMN.
	if hasRule(v, RuleCreateIndexNonConcurrent) {
		t.Errorf("comment text should not cause rule match: %v", v)
	}
	if len(v) > 0 {
		t.Errorf("no violations expected, got %v", v)
	}
}

func TestLint_LineNumbersAccurate(t *testing.T) {
	sql := "ALTER TABLE t ADD COLUMN x INT;\nCREATE INDEX idx ON t (x);\n"
	v := LintFile("test.up.sql", []byte(sql))
	if len(v) == 0 {
		t.Fatal("expected violation")
	}
	if v[0].Line < 2 {
		t.Errorf("line = %d, want >= 2", v[0].Line)
	}
}

func TestLintDir_MissingDir(t *testing.T) {
	if _, err := LintDir("/nope/not/here"); err == nil {
		t.Error("missing dir should error")
	}
}

func TestLintDir_SortedOutput(t *testing.T) {
	// Create a tempdir with two bad migrations.
	tmp := t.TempDir()
	must := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(tmp, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	must("000002_b.up.sql", `CREATE INDEX b ON t (c);`)
	must("000001_a.up.sql", `VACUUM FULL t;`)
	v, err := LintDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) < 2 {
		t.Fatalf("expected 2, got %v", v)
	}
	if !strings.Contains(v[0].File, "000001_a") {
		t.Errorf("expected sorted order, got %s first", v[0].File)
	}
}

// FuzzLinterNoPanicOnGarbage asserts the linter never panics on arbitrary
// input.
func FuzzLinterNoPanicOnGarbage(f *testing.F) {
	f.Add("CREATE INDEX x ON y (z);")
	f.Add("")
	f.Add("$$$")
	f.Add("-- comment\nDROP COLUMN x;")
	f.Fuzz(func(t *testing.T, src string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic: %v", r)
			}
		}()
		_ = LintFile("fuzz.up.sql", []byte(src))
	})
}

func hasRule(vs []Violation, r Rule) bool {
	for _, v := range vs {
		if v.Rule == r {
			return true
		}
	}
	return false
}
