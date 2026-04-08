package cli_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"strait/internal/cli"
)

// writeTree creates a directory tree from a map of relPath→content.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// tarEntries extracts all entry names from a gzipped tar in r.
func tarEntries(t *testing.T, r io.Reader) []string {
	t.Helper()
	gz, err := gzip.NewReader(r)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		names = append(names, hdr.Name)
	}
	return names
}

func TestPack_BasicDirectory(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.py":     "print('hello')",
		"lib/util.py": "pass",
	})

	var buf bytes.Buffer
	res, err := cli.Pack(&buf, dir, nil)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if res.SizeBytes == 0 {
		t.Error("expected non-zero size")
	}

	entries := tarEntries(t, &buf)
	entrySet := make(map[string]bool, len(entries))
	for _, e := range entries {
		entrySet[e] = true
	}
	if !entrySet["main.py"] {
		t.Error("expected main.py in tarball")
	}
	if !entrySet["lib/util.py"] {
		t.Error("expected lib/util.py in tarball")
	}
}

func TestPack_ExcludesNodeModules(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"index.ts":              "export {}",
		"node_modules/pkg/a.js": "module.exports={}",
	})

	var buf bytes.Buffer
	if _, err := cli.Pack(&buf, dir, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	entries := tarEntries(t, &buf)
	for _, e := range entries {
		if e == "node_modules/pkg/a.js" {
			t.Error("node_modules should be excluded")
		}
	}
}

func TestPack_ExcludesStraitignorePatterns(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.go":       "package main",
		"secret.key":    "hunter2",
		".straitignore": "*.key\n",
	})

	var buf bytes.Buffer
	if _, err := cli.Pack(&buf, dir, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	entries := tarEntries(t, &buf)
	for _, e := range entries {
		if e == "secret.key" {
			t.Error("secret.key should be excluded by .straitignore")
		}
	}
}

func TestPack_ExtraPatterns(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.go":  "package main",
		"skip.txt": "skip me",
	})

	var buf bytes.Buffer
	if _, err := cli.Pack(&buf, dir, []string{"skip.txt"}); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	entries := tarEntries(t, &buf)
	for _, e := range entries {
		if e == "skip.txt" {
			t.Error("skip.txt should be excluded by extra patterns")
		}
	}
}

func TestPack_SHA256IsStable(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a.txt": "aaa",
		"b.txt": "bbb",
	})

	var buf1, buf2 bytes.Buffer
	r1, err := cli.Pack(&buf1, dir, nil)
	if err != nil {
		t.Fatalf("first Pack: %v", err)
	}
	r2, err := cli.Pack(&buf2, dir, nil)
	if err != nil {
		t.Fatalf("second Pack: %v", err)
	}
	if r1.SHA256Hex != r2.SHA256Hex {
		t.Errorf("hash unstable: %q vs %q", r1.SHA256Hex, r2.SHA256Hex)
	}
}

func TestPack_SHA256MatchesContent(t *testing.T) {
	dir := writeTree(t, map[string]string{"hello.txt": "world"})

	var buf bytes.Buffer
	res, err := cli.Pack(&buf, dir, nil)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	h := sha256.Sum256(buf.Bytes())
	expected := hex.EncodeToString(h[:])
	if res.SHA256Hex != expected {
		t.Errorf("hash mismatch: got %q want %q", res.SHA256Hex, expected)
	}
}

func TestPack_ExcludesDotGit(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.go":        "package main",
		".git/HEAD":      "ref: refs/heads/main",
		".git/ORIG_HEAD": "abc123",
	})

	var buf bytes.Buffer
	if _, err := cli.Pack(&buf, dir, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	entries := tarEntries(t, &buf)
	for _, e := range entries {
		if e == ".git/HEAD" || e == ".git/ORIG_HEAD" {
			t.Errorf(".git entry %q should be excluded", e)
		}
	}
}

func TestPack_SizeBytesMatchesWritten(t *testing.T) {
	dir := writeTree(t, map[string]string{"f.txt": "hello"})

	var buf bytes.Buffer
	res, err := cli.Pack(&buf, dir, nil)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if int64(buf.Len()) != res.SizeBytes {
		t.Errorf("size mismatch: buf=%d res.SizeBytes=%d", buf.Len(), res.SizeBytes)
	}
}
