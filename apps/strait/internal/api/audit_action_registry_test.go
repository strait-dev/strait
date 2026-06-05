package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAuditActionRegistryCoverage walks every .go file under internal/api/
// and asserts that every call to emitAuditEvent / emitAuditEventAsync passes
// a domain.AuditAction* constant as the action argument, never a string
// literal. This ensures typos are impossible and the action taxonomy stays
// centralized in internal/domain/audit_actions.go.
//
// Exceptions:
//   - *_test.go files may pass string literals directly (tests hard-code
//     action names to assert behavior).
//   - rbac.go and audit_emit.go are exempt: they contain the definitions of
//     emitAuditEvent and emitAuditEventAsync themselves.
func TestAuditActionRegistryCoverage(t *testing.T) {
	t.Parallel()

	dir, err := filepath.Abs(".")
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	fset := token.NewFileSet()

	type violation struct {
		file string
		line int
		lit  string
	}
	var violations []violation

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(dir, name)
		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		require.NoError(t, parseErr)

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name != "emitAuditEvent" && sel.Sel.Name != "emitAuditEventAsync" {
				return true
			}
			// Signature: (ctx, action, resourceType, resourceID, details)
			if len(call.Args) < 2 {
				return true
			}
			actionArg := call.Args[1]
			lit, isLit := actionArg.(*ast.BasicLit)
			if isLit && lit.Kind == token.STRING {
				pos := fset.Position(lit.Pos())
				violations = append(violations, violation{
					file: filepath.Base(pos.Filename),
					line: pos.Line,
					lit:  lit.Value,
				})
			}
			return true
		})
	}

	if len(violations) == 0 {
		return
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].file != violations[j].file {
			return violations[i].file < violations[j].file
		}
		return violations[i].line < violations[j].line
	})

	var b strings.Builder
	b.WriteString("the following emitAuditEvent* calls pass a string literal instead of a domain.AuditAction* const:\n\n")
	for _, v := range violations {
		b.WriteString("  - ")
		b.WriteString(v.file)
		b.WriteString(":")
		b.WriteString(itoa(v.line))
		b.WriteString("  action=")
		b.WriteString(v.lit)
		b.WriteString("\n")
	}
	b.WriteString("\nadd a new const in internal/domain/audit_actions.go and use it at the call site.\n")
	b.WriteString("this keeps the action taxonomy centralized and prevents typos like \"job.delted\".\n")
	require.Fail(t,

		b.String())
}
