package migrationlint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests for the migration safety linter.

// baselineCutoff is the migration number up to which historical violations
// are tolerated. New migrations (> this number) must pass clean. Raise the
// cutoff over time as legacy migrations get annotated with `-- safety-ok`.
const baselineCutoff = 234

func TestLint_NewMigrationsPassClean(t *testing.T) {
	dir := filepath.Join("..", "..", "migrations")
	violations, err := LintDir(dir)
	require.NoError(t, err)

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
	assert.Empty(t, newViolations)
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
	assertHasRule(t, v, RuleCreateIndexNonConcurrent)
}

func TestLint_CreateIndexConcurrentlyAccepted(t *testing.T) {
	sql := `CREATE INDEX CONCURRENTLY idx_good ON job_runs (id);`
	v := LintFile("test.up.sql", []byte(sql))
	assertNoRule(t, v, RuleCreateIndexNonConcurrent)
}

func TestLint_CreateIndexInTransactionalMigrationIsStillFlagged(t *testing.T) {
	// Even inside BEGIN/COMMIT the linter should still flag.
	sql := `BEGIN;
CREATE INDEX idx_bad ON job_runs (id);
COMMIT;`
	v := LintFile("test.up.sql", []byte(sql))
	assertHasRule(t, v, RuleCreateIndexNonConcurrent)
}

func TestLint_CreateUniqueIndexWithoutConcurrently(t *testing.T) {
	sql := `CREATE UNIQUE INDEX idx_bad ON users (email);`
	v := LintFile("test.up.sql", []byte(sql))
	assertHasRule(t, v, RuleCreateUniqueNonConcurrent)
}

func TestLint_SetNotNull(t *testing.T) {
	sql := `ALTER TABLE users ALTER COLUMN email SET NOT NULL;`
	v := LintFile("test.up.sql", []byte(sql))
	assertHasRule(t, v, RuleSetNotNull)
}

func TestLint_AddColumnNotNullDefault(t *testing.T) {
	sql := `ALTER TABLE users ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();`
	v := LintFile("test.up.sql", []byte(sql))
	assertHasRule(t, v, RuleAddColumnNotNullDefault)
}

func TestLint_AddColumnNullableAccepted(t *testing.T) {
	sql := `ALTER TABLE users ADD COLUMN nickname TEXT;`
	v := LintFile("test.up.sql", []byte(sql))
	assert.Empty(t, v)
}

func TestLint_DropColumn(t *testing.T) {
	sql := `ALTER TABLE users DROP COLUMN old;`
	v := LintFile("test.up.sql", []byte(sql))
	assertHasRule(t, v, RuleDropColumn)
}

func TestLint_VacuumFull(t *testing.T) {
	sql := `VACUUM FULL job_runs;`
	v := LintFile("test.up.sql", []byte(sql))
	assertHasRule(t, v, RuleVacuumFull)
}

func TestLint_ReindexNonConcurrent(t *testing.T) {
	sql := `REINDEX TABLE job_runs;`
	v := LintFile("test.up.sql", []byte(sql))
	assertHasRule(t, v, RuleReindexNonConcurrent)
}

func TestLint_ReindexConcurrentlyAccepted(t *testing.T) {
	sql := `REINDEX INDEX CONCURRENTLY idx_ok;`
	v := LintFile("test.up.sql", []byte(sql))
	assertNoRule(t, v, RuleReindexNonConcurrent)
}

func TestLint_LockTable(t *testing.T) {
	sql := `LOCK TABLE job_runs IN ACCESS EXCLUSIVE MODE;`
	v := LintFile("test.up.sql", []byte(sql))
	assertHasRule(t, v, RuleLockTable)
}

func TestLint_Cluster(t *testing.T) {
	sql := `CLUSTER job_runs USING idx_runs_queue;`
	v := LintFile("test.up.sql", []byte(sql))
	assertHasRule(t, v, RuleCluster)
}

func TestLint_RenameColumn(t *testing.T) {
	sql := `ALTER TABLE users RENAME COLUMN old_email TO email;`
	v := LintFile("test.up.sql", []byte(sql))
	assertHasRule(t, v, RuleRenameColumn)
}

func TestLint_RenameTable(t *testing.T) {
	sql := `ALTER TABLE users RENAME TO people;`
	v := LintFile("test.up.sql", []byte(sql))
	assertHasRule(t, v, RuleRenameTable)
}

func TestLint_DropIndexBareFlagged(t *testing.T) {
	sql := `DROP INDEX idx_bad;`
	v := LintFile("test.up.sql", []byte(sql))
	assertHasRule(t, v, RuleDropIndexNonConcurrent)
}

func TestLint_DropIndexIfExistsAccepted(t *testing.T) {
	sql := `DROP INDEX IF EXISTS idx_ok;`
	v := LintFile("test.up.sql", []byte(sql))
	assertNoRule(t, v, RuleDropIndexNonConcurrent)
}

func TestLint_DropIndexConcurrentlyAccepted(t *testing.T) {
	sql := `DROP INDEX CONCURRENTLY idx_ok;`
	v := LintFile("test.up.sql", []byte(sql))
	assertNoRule(t, v, RuleDropIndexNonConcurrent)
}

func TestLint_SafetyOKBypass(t *testing.T) {
	sql := `-- safety-ok: brand new table, no readers yet
CREATE INDEX idx_new ON brand_new (id);`
	v := LintFile("test.up.sql", []byte(sql))
	assert.Empty(t, v)
}

func TestLint_SafetyOKRequiresReasonOnSameLine(t *testing.T) {
	// `-- safety-ok:` with a reason on a subsequent line should still
	// flag: the reason must be on the same comment line as the marker
	// so reviewers can see it in context.
	sql := `-- safety-ok:
-- reason on next line
CREATE INDEX idx_new ON unrelated (id);`
	v := LintFile("test.up.sql", []byte(sql))
	assert.NotEmpty(t, v)
}

func TestLint_MultiStatementFileReportsAllViolations(t *testing.T) {
	sql := `CREATE INDEX a ON t (c);
VACUUM FULL t;`
	v := LintFile("test.up.sql", []byte(sql))
	assert.GreaterOrEqual(t, len(v), 2)
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
	assertHasRule(t, v, RuleCreateIndexNonConcurrent)
}

func TestLint_CommentsStrippedForMatching(t *testing.T) {
	// A comment mentioning CREATE INDEX shouldn't flag.
	sql := `-- this migration does NOT create an index
ALTER TABLE t ADD COLUMN x INT;`
	v := LintFile("test.up.sql", []byte(sql))
	// It's fine for the linter to either skip comments or still match
	// them; we just want no false positive on safe ADD COLUMN.
	assertNoRule(t, v, RuleCreateIndexNonConcurrent)
	assert.Empty(t, v)
}

func TestLint_LineNumbersAccurate(t *testing.T) {
	sql := "ALTER TABLE t ADD COLUMN x INT;\nCREATE INDEX idx ON t (x);\n"
	v := LintFile("test.up.sql", []byte(sql))
	require.NotEmpty(t, v)
	assert.GreaterOrEqual(t, v[0].Line, 2)
}

func TestLintDir_MissingDir(t *testing.T) {
	_, err := LintDir("/nope/not/here")
	assert.Error(t, err)
}

func TestLintDir_SortedOutput(t *testing.T) {
	// Create a tempdir with two bad migrations.
	tmp := t.TempDir()
	must := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), []byte(body), 0o600))
	}
	must("000002_b.up.sql", `CREATE INDEX b ON t (c);`)
	must("000001_a.up.sql", `VACUUM FULL t;`)
	v, err := LintDir(tmp)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(v), 2)
	assert.Contains(t, v[0].File, "000001_a")
}

func TestLintPairs_AllRepoMigrationsArePaired(t *testing.T) {
	dir := filepath.Join("..", "..", "migrations")
	v, err := LintPairs(dir)
	require.NoError(t, err)
	assert.Empty(t, v)
}

func TestLintPairs_OrphanUpFlagged(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "000001_a.up.sql"), []byte(`SELECT 1;`), 0o600))

	v, err := LintPairs(tmp)
	require.NoError(t, err)
	assertHasRule(t, v, RuleUnpairedMigration)
	require.NotEmpty(t, v)
	assert.Contains(t, v[0].Snippet, "000001_a.up.sql")
}

func TestLintPairs_OrphanDownFlagged(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "000001_a.down.sql"), []byte(`SELECT 1;`), 0o600))

	v, err := LintPairs(tmp)
	require.NoError(t, err)
	assertHasRule(t, v, RuleUnpairedMigration)
	require.NotEmpty(t, v)
	assert.Contains(t, v[0].Snippet, "000001_a.down.sql")
}

func TestLintPairs_PairedAccepted(t *testing.T) {
	tmp := t.TempDir()
	must := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), []byte(body), 0o600))
	}
	must("000001_a.up.sql", `SELECT 1;`)
	must("000001_a.down.sql", `SELECT 1;`)
	must("000002_b.up.sql", `SELECT 1;`)
	must("000002_b.down.sql", `SELECT 1;`)
	v, err := LintPairs(tmp)
	require.NoError(t, err)
	assert.Empty(t, v)
}

func TestLintPairs_PairedDeletionAccepted(t *testing.T) {
	// Simulates the state after a migration pair has been deleted: the
	// directory simply contains the surviving paired migrations. The check
	// must not report anything because both halves were removed together.
	tmp := t.TempDir()
	must := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), []byte(body), 0o600))
	}
	must("000001_a.up.sql", `SELECT 1;`)
	must("000001_a.down.sql", `SELECT 1;`)
	v, err := LintPairs(tmp)
	require.NoError(t, err)
	assert.Empty(t, v)
}

func TestLintPairs_MissingDir(t *testing.T) {
	_, err := LintPairs("/nope/not/here")
	assert.Error(t, err)
}

func TestLintPairs_SortedOutput(t *testing.T) {
	tmp := t.TempDir()
	must := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), []byte(body), 0o600))
	}
	must("000002_b.up.sql", `SELECT 1;`)
	must("000001_a.down.sql", `SELECT 1;`)
	v, err := LintPairs(tmp)
	require.NoError(t, err)
	assert.Len(t, v, 2)
	assert.Contains(t, v[0].File, "000001_a")
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
				require.Failf(t, "panic", "%v", r)
			}
		}()
		_ = LintFile("fuzz.up.sql", []byte(src))
	})
}

func assertHasRule(t *testing.T, vs []Violation, r Rule) {
	t.Helper()
	assert.Truef(t, hasRule(vs, r), "expected %s, got %v", r, vs)
}

func assertNoRule(t *testing.T, vs []Violation, r Rule) {
	t.Helper()
	assert.Falsef(t, hasRule(vs, r), "unexpected %s in %v", r, vs)
}

func hasRule(vs []Violation, r Rule) bool {
	for _, v := range vs {
		if v.Rule == r {
			return true
		}
	}
	return false
}
