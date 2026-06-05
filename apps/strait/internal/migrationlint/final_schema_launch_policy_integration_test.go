//go:build integration

package migrationlint_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"strait/internal/testutil"
)

func TestFinalSchemaDoesNotRetainRetiredModelOrKeyNames(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	tdb, err := testutil.SetupSharedTestDB(ctx, migrationsRelPath, "migrationlint-final-schema")
	if err != nil {
		t.Fatalf("setup test db: %v", err)
	}
	defer tdb.Cleanup(ctx)

	retiredModelPrefix := strings.Join([]string{"(^|_)", "a", "i", "($|_)"}, "")
	retiredModelNamedPrefix := strings.Join([]string{"(^|_)", "a", "i", "_"}, "")
	retiredModelSuffix := strings.Join([]string{"_", "a", "i", "($|_)"}, "")
	retiredKeyAcronym := strings.Join([]string{"b", "y", "o", "k"}, "")
	retiredKeyPhrase := strings.Join([]string{"bring_?your_?own_?key"}, "")
	retiredEnterpriseCredit := "included_credit_microusd"
	retiredEnterpriseDiscount := "compute_discount_pct"

	rows, err := tdb.Pool.Query(ctx, `
		WITH names AS (
			SELECT 'table' AS kind, c.relname AS name
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public'
			  AND c.relkind IN ('r', 'p', 'v', 'm', 'i', 'S')

			UNION ALL

			SELECT 'column' AS kind, table_name || '.' || column_name AS name
			FROM information_schema.columns
			WHERE table_schema = 'public'
		)
		SELECT kind, name
		FROM names
		WHERE name ~* $1
		   OR name ~* $2
		   OR name ~* $3
		   OR name ~* $4
		   OR name ~* $5
		   OR name LIKE '%.' || $6
		   OR name LIKE '%.' || $7
		ORDER BY kind, name
	`, retiredModelPrefix, retiredModelNamedPrefix, retiredModelSuffix, retiredKeyAcronym, retiredKeyPhrase, retiredEnterpriseCredit, retiredEnterpriseDiscount)
	if err != nil {
		t.Fatalf("query final schema names: %v", err)
	}
	defer rows.Close()

	var stale []string
	for rows.Next() {
		var kind, name string
		if err := rows.Scan(&kind, &name); err != nil {
			t.Fatalf("scan stale schema name: %v", err)
		}
		stale = append(stale, kind+":"+name)
	}
	if rows.Err() != nil {
		t.Fatalf("iterate stale schema names: %v", rows.Err())
	}
	if len(stale) > 0 {
		t.Fatalf("final migrated schema retains retired model/key names: %s", strings.Join(stale, ", "))
	}
}
