package worker

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)

	var matches []string
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		require.NoError(t, err)

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
	require.Len(t,
		matches, 1)
	require.Equal(t,
		"sendWebhookOnceWith",

		matches[0])

}
