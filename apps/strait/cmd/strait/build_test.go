package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var stdoutCaptureMu sync.Mutex

func TestBuildCommand_JSONEmitsSingleDocumentAndDoesNotWriteFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.json")
	if err := os.WriteFile(configPath, []byte(`{"project":{"id":"proj-1"},"runtime":"node","jobs":[{"slug":"job-1","name":"Job 1"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	state := &appState{opts: &rootOptions{}}
	cmd := newBuildCommand(state)
	cmd.SetArgs([]string{"--config", configPath, "--json"})

	output := captureCommandStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("build --json: %v", err)
		}
	})

	var manifest map[string]any
	if err := json.Unmarshal([]byte(output), &manifest); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, output)
	}
	if strings.Count(strings.TrimSpace(output), "\n{") > 0 {
		t.Fatalf("expected a single JSON document, got:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(dir, ".strait", "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("build --json should not write manifest.json, stat err=%v", err)
	}
}

func TestBuildCommand_DryRunDoesNotWriteFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "strait.config.yml")
	if err := os.WriteFile(configPath, []byte("project:\n  id: proj-1\nruntime: node\njobs:\n  - slug: job-1\n    name: Job 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	state := &appState{opts: &rootOptions{}}
	cmd := newBuildCommand(state)
	cmd.SetArgs([]string{"--config", configPath, "--dry-run"})

	output := captureCommandStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("build --dry-run: %v", err)
		}
	})

	if !strings.Contains(output, `"checksum"`) {
		t.Fatalf("expected manifest JSON output, got:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(dir, ".strait", "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("build --dry-run should not write manifest.json, stat err=%v", err)
	}
}

func captureCommandStdout(t *testing.T, fn func()) string {
	t.Helper()
	stdoutCaptureMu.Lock()
	defer stdoutCaptureMu.Unlock()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}

	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = originalStdout
	})

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}

	os.Stdout = originalStdout
	return string(data)
}
