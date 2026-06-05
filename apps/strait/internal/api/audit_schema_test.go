package api

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuditSchemaGeneratedIsFresh re-runs the cmd/gen-audit-schema
// generator in a subprocess and asserts the committed output file
// matches. Run `go run ./cmd/gen-audit-schema > internal/api/audit_schema_generated.json`
// after adding a new action.
//
// Skipped under -short because it shells out to go run.
func TestAuditSchemaGeneratedIsFresh(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping schema drift check in short mode")
	}
	t.Parallel()

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("repo root not found: %v", err)
	}

	cmd := exec.Command("go", "run", "./cmd/gen-audit-schema")
	cmd.Dir = repoRoot
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	require.NoError(t, cmd.Run())

	committedPath := filepath.Join(repoRoot, "internal/api/audit_schema_generated.json")
	committed, err := os.ReadFile(committedPath)
	require.NoError(t, err)

	// Normalize trailing newlines.
	generated := bytes.TrimRight(out.Bytes(), "\n")
	committed = bytes.TrimRight(committed, "\n")

	// Parse both into any and compare via re-encoding to avoid
	// key-ordering false positives (Go maps may reorder on marshal —
	// the generator already sorts action keys, so this is belt-and-
	// braces).
	var g, c any
	require.NoError(t, json.Unmarshal(
		generated,
		&g))
	require.NoError(t, json.Unmarshal(
		committed,
		&c))

	gBytes, _ := json.Marshal(g)
	cBytes, _ := json.Marshal(c)
	assert.True(t, bytes.Equal(gBytes,

		cBytes))
}

// findRepoRoot walks up from the test's cwd looking for go.mod. Tests
// run from the package directory, so this climbs out to apps/strait.
func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
