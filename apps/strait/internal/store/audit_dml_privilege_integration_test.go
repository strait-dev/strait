//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestIntegration_AuditEventsDMLRestricted_DetectsUnsafeColumnUpdateGrant(t *testing.T) {
	ctx := context.Background()

	got := auditDMLRestrictedAsAppRole(t, ctx, "")
	require.True(t, got)

	got = auditDMLRestrictedAsAppRole(t, ctx, "GRANT UPDATE (details) ON audit_events TO strait_app")
	require.False(t, got)

}

func auditDMLRestrictedAsAppRole(t *testing.T, ctx context.Context, extraGrant string) bool {
	t.Helper()

	tx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Logf("rollback audit privilege test tx: %v", err)
		}
	}()

	stmts := []string{
		"REVOKE UPDATE, DELETE, TRUNCATE ON audit_events FROM strait_app",
		"REVOKE UPDATE (details) ON audit_events FROM strait_app",
		"GRANT SELECT, INSERT ON audit_events TO strait_app",
		"GRANT UPDATE (signature) ON audit_events TO strait_app",
	}
	if extraGrant != "" {
		stmts = append(stmts, extraGrant)
	}
	stmts = append(stmts, "SET LOCAL ROLE strait_app")
	for _, stmt := range stmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			require.Failf(t, "test failure",

				"exec %q: %v", stmt, err)
		}
	}

	got, err := store.New(tx).AuditEventsDMLRestricted(ctx)
	require.NoError(t, err)

	return got
}
