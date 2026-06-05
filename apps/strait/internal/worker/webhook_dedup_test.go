package worker

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestSingleSendWebhookOnceImpl regresses fix #7: prior to dedupe, the
// package carried two near-identical webhook delivery implementations
// (sendWebhookOnce and sendWebhookOnceWith) that differed only by which
// http.Client they invoked. The duplicate drifted: changes to headers,
// span attributes, or signing landed in one and not the other. Now
// SendWebhookWithRetry routes through sendWebhookOnceWith so there is
// only one delivery body. This test fails if a future contributor
// re-introduces a parallel sendWebhookOnce* helper.
func TestSingleSendWebhookOnceImpl(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	files, err := filepath.Glob("webhook*.go")
	if err != nil {
		t.Fatalf("glob webhook files: %v", err)
	}

	var matches []string
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue
			}
			if strings.HasPrefix(fn.Name.Name, "sendWebhookOnce") {
				matches = append(matches, fn.Name.Name)
			}
		}
	}

	if len(matches) != 1 {
		t.Fatalf("expected exactly one sendWebhookOnce* helper, found %d: %v. "+
			"Refactor callers onto the existing impl rather than duplicating.", len(matches), matches)
	}
	if matches[0] != "sendWebhookOnceWith" {
		t.Fatalf("expected the sole impl to be sendWebhookOnceWith (client-injected), got %q", matches[0])
	}
}
