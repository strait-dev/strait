package api

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
	if err := cmd.Run(); err != nil {
		t.Fatalf("go run gen-audit-schema: %v\noutput: %s", err, out.String())
	}

	committedPath := filepath.Join(repoRoot, "internal/api/audit_schema_generated.json")
	committed, err := os.ReadFile(committedPath)
	if err != nil {
		t.Fatalf("read committed schema: %v", err)
	}

	// Normalize trailing newlines.
	generated := bytes.TrimRight(out.Bytes(), "\n")
	committed = bytes.TrimRight(committed, "\n")

	// Parse both into any and compare via re-encoding to avoid
	// key-ordering false positives (Go maps may reorder on marshal —
	// the generator already sorts action keys, so this is belt-and-
	// braces).
	var g, c any
	if err := json.Unmarshal(generated, &g); err != nil {
		t.Fatalf("unmarshal generated: %v", err)
	}
	if err := json.Unmarshal(committed, &c); err != nil {
		t.Fatalf("unmarshal committed: %v", err)
	}
	gBytes, _ := json.Marshal(g)
	cBytes, _ := json.Marshal(c)
	if !bytes.Equal(gBytes, cBytes) {
		t.Errorf("committed audit_schema_generated.json is stale.\nRun:\n  go run ./cmd/gen-audit-schema > internal/api/audit_schema_generated.json\nand commit the result.")
	}
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
