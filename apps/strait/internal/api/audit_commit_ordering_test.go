package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runInTxHandlers are the handlers known to wrap store mutations inside
// a runInTx closure. The audit emit must appear AFTER the closure returns,
// otherwise a rollback would leave a phantom event in the log.
var runInTxHandlers = map[string]struct{}{
	"handleCreateWorkflow":  {},
	"handleUpdateWorkflow":  {},
	"handleCloneWorkflow":   {},
	"handleSendEvent":       {},
	"handleSDKWaitForEvent": {},
}

// TestAuditEmitOrdering_OutsideRunInTx verifies that for every handler
// that wraps store writes inside a runInTx closure, the emit call
// appears textually AFTER the runInTx call. This prevents a scenario
// where an audit event is recorded but the transaction rolls back.
func TestAuditEmitOrdering_OutsideRunInTx(t *testing.T) {
	t.Parallel()

	dir, err := filepath.Abs(".")
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	fset := token.NewFileSet()

	type result struct {
		handler     string
		file        string
		runInTxPos  token.Pos
		emitPos     token.Pos
		runInTxLine int
		emitLine    int
	}
	var violations []result

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		require.NoError(t, parseErr)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			if !isServerReceiver(fn.Recv) {
				continue
			}
			if _, tracked := runInTxHandlers[fn.Name.Name]; !tracked {
				continue
			}

			var firstRunInTx, firstEmit token.Pos
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if sel.Sel.Name == "runInTx" {
					if firstRunInTx == token.NoPos {
						firstRunInTx = call.Pos()
					}
				}
				if sel.Sel.Name == "emitAuditEvent" || sel.Sel.Name == "emitAuditEventAsync" {
					if firstEmit == token.NoPos {
						firstEmit = call.Pos()
					}
				}
				return true
			})

			if firstRunInTx == token.NoPos || firstEmit == token.NoPos {
				continue
			}

			if firstEmit < firstRunInTx {
				violations = append(violations, result{
					handler:     fn.Name.Name,
					file:        name,
					runInTxPos:  firstRunInTx,
					emitPos:     firstEmit,
					runInTxLine: fset.Position(firstRunInTx).Line,
					emitLine:    fset.Position(firstEmit).Line,
				})
			}
		}
	}

	if len(violations) > 0 {
		var b strings.Builder
		b.WriteString("audit emit appears BEFORE runInTx in the following handlers:\n\n")
		for _, v := range violations {
			b.WriteString("  - ")
			b.WriteString(v.file)
			b.WriteString(": ")
			b.WriteString(v.handler)
			b.WriteString(" (emit at line ")
			b.WriteString(itoa(v.emitLine))
			b.WriteString(", runInTx at line ")
			b.WriteString(itoa(v.runInTxLine))
			b.WriteString(")\n")
		}
		b.WriteString("\naudit events must only fire AFTER the runInTx closure returns.\n")
		require.Fail(t,

			b.String())
	}

	// Verify all tracked handlers were actually found in source.
	found := make(map[string]bool, len(runInTxHandlers))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			if _, tracked := runInTxHandlers[fn.Name.Name]; tracked {
				found[fn.Name.Name] = true
			}
		}
	}
	for name := range runInTxHandlers {
		assert.True(
			t, found[name])
	}
}
