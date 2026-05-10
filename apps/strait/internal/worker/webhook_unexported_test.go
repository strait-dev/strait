package worker

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestSendWebhookWithClientUnexported is the regression guard for fix #6:
// the BYOC (bring-your-own-client) entrypoint must remain package-private
// so production code cannot accidentally bypass newSafeWebhookTransport's
// SSRF protections by passing in a vanilla http.Client. The test
// statically inspects webhook.go and fails if a top-level identifier
// matching the old name reappears as exported.
func TestSendWebhookWithClientUnexported(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "webhook.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse webhook.go: %v", err)
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		// Methods are fine; we only care about top-level functions whose
		// name signals a BYOC client entrypoint.
		if fn.Recv != nil {
			continue
		}
		if !strings.Contains(fn.Name.Name, "WithClient") {
			continue
		}
		if ast.IsExported(fn.Name.Name) {
			t.Fatalf(
				"top-level function %q is exported. BYOC webhook senders MUST stay package-private "+
					"to keep the SSRF-safe transport (newSafeWebhookTransport) on the only public delivery path. "+
					"Production callers must route through SendWebhook / SendWebhookWithRetry.",
				fn.Name.Name,
			)
		}
	}
}
