package extension

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func setupHookPlugin(t *testing.T, dir, pluginName, hook, script string) {
	t.Helper()

	pluginDir := filepath.Join(dir, pluginName)
	hooksDir := filepath.Join(pluginDir, "hooks")

	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	manifest := PluginManifest{
		Name:     pluginName,
		Version:  "1.0.0",
		Commands: []string{"cmd"},
		Hooks:    []string{hook},
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(pluginDir, "strait-plugin.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	hookPath := filepath.Join(hooksDir, hook)
	if err := os.WriteFile(hookPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteHooks_PreHook_BlocksOnFailure(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not portable on windows")
	}

	dir := t.TempDir()
	setupHookPlugin(t, dir, "failing-plugin", "pre-deploy", "#!/bin/sh\nexit 1\n")

	hctx := HookContext{Hook: "pre-deploy", ProjectID: "proj-1"}
	err := ExecuteHooks(context.Background(), "pre-deploy", hctx, dir, 5*time.Second)
	if err == nil {
		t.Fatal("expected error from failing pre-hook, got nil")
	}
}

func TestExecuteHooks_PostHook_WarnsOnFailure(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not portable on windows")
	}

	dir := t.TempDir()
	setupHookPlugin(t, dir, "failing-post", "post-deploy", "#!/bin/sh\nexit 1\n")

	hctx := HookContext{Hook: "post-deploy", ProjectID: "proj-1"}
	err := ExecuteHooks(context.Background(), "post-deploy", hctx, dir, 5*time.Second)
	if err != nil {
		t.Fatalf("post-hook failure should not return error, got: %v", err)
	}
}

func TestExecuteHooks_Timeout_KillsProcess(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not portable on windows")
	}

	dir := t.TempDir()
	setupHookPlugin(t, dir, "slow-plugin", "pre-build", "#!/bin/sh\nsleep 60\n")

	hctx := HookContext{Hook: "pre-build"}
	err := ExecuteHooks(context.Background(), "pre-build", hctx, dir, 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestExecuteHooks_PassesContextOnStdin(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not portable on windows")
	}

	dir := t.TempDir()
	outFile := filepath.Join(dir, "stdin-capture.json")

	// The hook script reads stdin and writes it to a file.
	script := "#!/bin/sh\ncat > " + outFile + "\n"
	setupHookPlugin(t, dir, "stdin-reader", "pre-trigger", script)

	hctx := HookContext{
		Hook:      "pre-trigger",
		JobSlug:   "my-job",
		ProjectID: "proj-42",
		Extra:     map[string]any{"key": "value"},
	}
	err := ExecuteHooks(context.Background(), "pre-trigger", hctx, dir, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	captured, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading captured stdin: %v", err)
	}

	var got HookContext
	if err := json.Unmarshal(captured, &got); err != nil {
		t.Fatalf("unmarshaling captured stdin: %v", err)
	}
	if got.JobSlug != "my-job" {
		t.Errorf("expected job_slug %q, got %q", "my-job", got.JobSlug)
	}
	if got.ProjectID != "proj-42" {
		t.Errorf("expected project_id %q, got %q", "proj-42", got.ProjectID)
	}
}

func TestExecuteHooks_NoPlugins_Noop(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	hctx := HookContext{Hook: "pre-deploy"}

	err := ExecuteHooks(context.Background(), "pre-deploy", hctx, dir, 5*time.Second)
	if err != nil {
		t.Fatalf("expected no error for empty plugin dir, got: %v", err)
	}
}

func TestExecuteHooks_SkipHooksEnvVar(t *testing.T) {
	// Cannot use t.Parallel with t.Setenv.
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not portable on windows")
	}

	dir := t.TempDir()
	setupHookPlugin(t, dir, "should-skip", "pre-deploy", "#!/bin/sh\nexit 1\n")

	t.Setenv("STRAIT_SKIP_HOOKS", "1")

	hctx := HookContext{Hook: "pre-deploy"}
	err := ExecuteHooks(context.Background(), "pre-deploy", hctx, dir, 5*time.Second)
	if err != nil {
		t.Fatalf("expected nil when STRAIT_SKIP_HOOKS=1, got: %v", err)
	}
}
