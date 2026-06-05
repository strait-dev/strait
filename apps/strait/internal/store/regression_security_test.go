package store

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression guards: column allowlists in dynamic UPDATE builders.
// Each test verifies that the function rejects an unknown column name with a
// *domain.FieldError before any SQL is executed (nil db is safe).

// assertFieldError is a shared helper for all allowlist regression tests.
func assertFieldError(t *testing.T, err error, wantField string) {
	t.Helper()
	require.Error(
		t, err)

	var fe *domain.FieldError
	require.ErrorAs(t,
		err, &fe)
	require.Equal(
		t, wantField,
		fe.
			Field)
}

// TestRegression_LogDrainAllowlist verifies UpdateLogDrain rejects columns not
// in the allowlist.
func TestRegression_LogDrainAllowlist(t *testing.T) {
	t.Parallel()

	q := &Queries{} // nil db -- rejection happens before SQL execution.
	badColumns := []string{
		"id",
		"project_id",
		"created_at",
		"password",
		"role; DROP TABLE users --",
	}
	for _, col := range badColumns {
		t.Run(col, func(t *testing.T) {
			t.Parallel()
			err := q.UpdateLogDrain(t.Context(), "drain-1", "proj-1", map[string]any{
				col: "injected",
			})
			assertFieldError(t, err, col)
		})
	}
}

// TestRegression_EventSourceAllowlist verifies UpdateEventSource rejects
// columns not in the allowlist.
func TestRegression_EventSourceAllowlist(t *testing.T) {
	t.Parallel()

	q := &Queries{}
	badColumns := []string{
		"id",
		"project_id",
		"created_at",
		"secret_key",
		"admin; DROP TABLE event_sources --",
	}
	for _, col := range badColumns {
		t.Run(col, func(t *testing.T) {
			t.Parallel()
			err := q.UpdateEventSource(t.Context(), "src-1", "proj-1", map[string]any{
				col: "injected",
			})
			assertFieldError(t, err, col)
		})
	}
}

// TestRegression_RunStatusAllowlist verifies UpdateRunStatus has an
// allowedColumns map and rejects unknown fields.
func TestRegression_RunStatusAllowlist(t *testing.T) {
	t.Parallel()

	q := &Queries{}
	badColumns := []string{
		"id",
		"job_id",
		"project_id",
		"admin_override",
	}
	for _, col := range badColumns {
		t.Run(col, func(t *testing.T) {
			t.Parallel()
			// Use a valid transition so the function reaches the allowlist check.
			err := q.UpdateRunStatus(
				t.Context(),
				"run-1",
				domain.StatusQueued, domain.StatusDequeued,
				map[string]any{col: "injected"},
			)
			assertFieldError(t, err, col)
		})
	}
}

// TestRegression_StepRunAllowlist verifies UpdateStepRunStatus has an
// allowedColumns map and rejects unknown fields.
func TestRegression_StepRunAllowlist(t *testing.T) {
	t.Parallel()

	q := &Queries{}
	badColumns := []string{
		"id",
		"workflow_run_id",
		"step_id",
		"created_at",
	}
	for _, col := range badColumns {
		t.Run(col, func(t *testing.T) {
			t.Parallel()
			err := q.UpdateStepRunStatus(
				t.Context(),
				"step-1",
				domain.StepRunning,
				map[string]any{col: "injected"},
			)
			assertFieldError(t, err, col)
		})
	}
}

// TestRegression_WorkflowRunAllowlist verifies UpdateWorkflowRunStatus has an
// allowedColumns map and rejects unknown fields.
func TestRegression_WorkflowRunAllowlist(t *testing.T) {
	t.Parallel()

	q := &Queries{}
	badColumns := []string{
		"id",
		"workflow_id",
		"project_id",
		"created_at",
	}
	for _, col := range badColumns {
		t.Run(col, func(t *testing.T) {
			t.Parallel()
			err := q.UpdateWorkflowRunStatus(
				t.Context(),
				"wfrun-1",
				domain.WfStatusPending, domain.WfStatusRunning,
				map[string]any{col: "injected"},
			)
			assertFieldError(t, err, col)
		})
	}
}

// Meta-test: scan store/*.go for potential raw SQL interpolation patterns.

// TestRegression_NoRawSQLInterpolation uses go/ast to scan all non-test .go
// files in the store package for fmt.Sprintf calls containing %s inside
// strings that look like SQL (contain SELECT, INSERT, UPDATE, DELETE, or WHERE).
// This is a heuristic guard against accidental raw string interpolation in SQL
// queries.
func TestRegression_NoRawSQLInterpolation(t *testing.T) {
	t.Parallel()

	storeDir := "."

	entries, err := os.ReadDir(storeDir)
	require.NoError(t, err)

	fset := token.NewFileSet()
	var violations []string

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		filePath := filepath.Join(storeDir, name)
		node, parseErr := parser.ParseFile(fset, filePath, nil, 0)
		require.NoError(t, parseErr)

		ast.Inspect(node, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			// Look for fmt.Sprintf calls.
			if ident.Name != "fmt" || sel.Sel.Name != "Sprintf" {
				return true
			}

			if len(call.Args) == 0 {
				return true
			}

			// Check the format string argument.
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}

			format := lit.Value
			formatUpper := strings.ToUpper(format)

			// Check if the format string looks like SQL.
			sqlKeywords := []string{"SELECT", "INSERT", "UPDATE", "DELETE", "WHERE", "SET"}
			looksLikeSQL := false
			for _, kw := range sqlKeywords {
				if strings.Contains(formatUpper, kw) {
					looksLikeSQL = true
					break
				}
			}

			if !looksLikeSQL {
				return true
			}

			// Reject %s in SQL context. Parameterized queries use $1, $2, etc.
			// Allow %s for known safe patterns:
			//   - SET %s = $%d (column from allowlist)
			//   - '%s' (SQL string literal from typed constant, e.g. domain.StatusQueued)
			//   - %s WHERE / SET %s (dynamic column building)
			if strings.Contains(format, "%s") {
				pos := fset.Position(call.Pos())

				// Strip all known safe patterns and check if any bare %s remains.
				cleaned := format
				// Allow column-allowlist pattern: %s = $
				cleaned = strings.ReplaceAll(cleaned, "%s = $", "")
				// Allow dynamic SET column building.
				cleaned = strings.ReplaceAll(cleaned, "SET %s", "")
				// Allow %s in WHERE clause building.
				cleaned = strings.ReplaceAll(cleaned, "%s WHERE", "")
				// Allow SQL string literals: '%s' (interpolating typed Go constants).
				cleaned = strings.ReplaceAll(cleaned, "'%s'", "")

				if !strings.Contains(cleaned, "%s") {
					return true
				}
				violations = append(violations, fmt.Sprintf(
					"%s:%d: fmt.Sprintf with %%s in SQL context: %s",
					pos.Filename, pos.Line, truncate(format, 120),
				))
			}
			return true
		})
	}
	assert.Empty(t,
		violations)
}

// truncate shortens a string for display.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
