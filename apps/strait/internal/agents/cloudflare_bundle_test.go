package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloudflareRuntimeSourceFromPathsPrefersBuiltBundle(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	candidate := filepath.Join(dir, "worker.js")
	if err := os.WriteFile(candidate, []byte(`export default { fetch() { return new Response("ok"); } };`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got := cloudflareRuntimeSourceFromPaths([]string{candidate})
	if !strings.Contains(got, `return new Response("ok")`) {
		t.Fatalf("cloudflareRuntimeSourceFromPaths() = %q", got)
	}
}

func TestCloudflareRuntimeSourceFromPathsFallsBackToEmbeddedBundle(t *testing.T) {
	t.Parallel()

	got := cloudflareRuntimeSourceFromPaths([]string{filepath.Join(t.TempDir(), "missing.js")})
	if got == "" {
		t.Fatal("expected embedded runtime bundle fallback")
	}
	if !strings.Contains(got, "runtime_worker_error") && !strings.Contains(got, "buildRuntimeOutput") {
		t.Fatalf("embedded runtime bundle does not look like the generated worker bundle")
	}
}
