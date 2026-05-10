package worker

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestSendWebhookWithClientUnexported pins the SSRF-safe-transport
// guarantee: no top-level *WithClient function may be exported in the
// production webhook.go file, and the test-only sendWebhookWithClient*
// helper must live in a _test.go file (where it cannot link into the
// production binary). Scanning both files catches a future contributor
// who promotes the helper back into webhook.go OR exports a new
// BYOC entrypoint.
func TestSendWebhookWithClientUnexported(t *testing.T) {
	t.Parallel()

	files := []struct {
		path             string
		allowTestHelper  bool
		mustHaveTestImpl bool
	}{
		{path: "webhook.go", allowTestHelper: false, mustHaveTestImpl: false},
		{path: "webhook_client_test.go", allowTestHelper: true, mustHaveTestImpl: true},
	}

	for _, f := range files {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, f.path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", f.path, err)
		}

		var sawTestHelper bool
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue
			}
			name := fn.Name.Name
			if !strings.Contains(name, "WithClient") {
				continue
			}
			if ast.IsExported(name) {
				t.Fatalf("%s: top-level function %q is exported. BYOC webhook senders MUST stay "+
					"package-private to keep newSafeWebhookTransport on the only public delivery path.",
					f.path, name)
			}
			if name == "sendWebhookWithClientForTest" {
				if !f.allowTestHelper {
					t.Fatalf("%s: sendWebhookWithClientForTest must live in webhook_client_test.go, "+
						"not in a production .go file (it bypasses the SSRF-safe transport).", f.path)
				}
				sawTestHelper = true
			}
		}
		if f.mustHaveTestImpl && !sawTestHelper {
			t.Fatalf("%s: expected sendWebhookWithClientForTest definition", f.path)
		}
	}
}
