package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestStreamSSEHandlersAreThinWrappers regresses fix #9: the three
// run-scoped SSE handlers (run, log, chunks) used to each carry their own
// ~70-line copy of the connection-cap acquire, Flusher assertion,
// header set, pubsub subscribe + cleanup, max-duration timeout, and
// keepalive ticker. A bug fix in one (e.g. fix #1's flusher promotion,
// fix #7's connection-cap, fix #8's max-duration enforcement) had to
// be ported by hand to the others. Now the bodies are thin wrappers
// over streamSSE; this test fails if any future contributor inlines
// the pump back into a handler body.
func TestStreamSSEHandlersAreThinWrappers(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "stream.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse stream.go: %v", err)
	}

	const maxBodyStmts = 3
	wrappers := map[string]bool{
		"handleRunStream":      false,
		"handleRunLogStream":   false,
		"handleRunChunkStream": false,
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil {
			continue
		}
		if _, watch := wrappers[fn.Name.Name]; !watch {
			continue
		}
		wrappers[fn.Name.Name] = true
		if fn.Body == nil {
			t.Fatalf("%s has nil body", fn.Name.Name)
		}
		if got := len(fn.Body.List); got > maxBodyStmts {
			t.Fatalf("%s has %d statements; expected <= %d. "+
				"SSE pump logic must live in streamSSE so a single fix lands once.",
				fn.Name.Name, got, maxBodyStmts)
		}

		// Each wrapper must invoke streamSSE.
		var sawStreamSSE bool
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "streamSSE" {
				sawStreamSSE = true
				return false
			}
			return true
		})
		if !sawStreamSSE {
			t.Fatalf("%s does not call s.streamSSE; the wrapper must delegate to the shared pump", fn.Name.Name)
		}
	}

	for name, found := range wrappers {
		if !found {
			t.Fatalf("expected wrapper handler %s in stream.go", name)
		}
	}
}

// TestStreamSSESingleSubscribeCall pins that pubsub.Subscribe appears
// exactly once in stream.go: dedupe must collapse the three call sites.
// A regression that re-inlines the pump would re-introduce parallel
// Subscribe calls and is what this guard catches.
func TestStreamSSESingleSubscribeCall(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "stream.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse stream.go: %v", err)
	}

	var subscribes int
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == "Subscribe" {
			// Heuristic: only count s.pubsub.Subscribe. The selector's
			// X is itself a SelectorExpr (s.pubsub).
			if inner, ok := sel.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "pubsub" {
				subscribes++
			}
		}
		return true
	})
	if subscribes != 1 {
		t.Fatalf("found %d s.pubsub.Subscribe calls in stream.go, want exactly 1", subscribes)
	}
}
