package billing

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLaunchSourceDoesNotExposeLegacyAgentGuardrails(t *testing.T) {
	t.Parallel()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	internalRoot := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))

	forbidden := []string{
		"Max" + "TokensPerRun",
		"Max" + "ToolCallsPerRun",
		"Max" + "IterationsPerRun",
		"Allowed" + "Tools",
		"Blocked" + "Tools",
		"max_" + "tokens_per_run",
		"max_" + "tool_calls_per_run",
		"max_" + "iterations_per_run",
		"allowed_" + "tools",
		"blocked_" + "tools",
	}

	err := filepath.WalkDir(internalRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, stale := range forbidden {
			if strings.Contains(string(content), stale) {
				t.Errorf("%s contains retired launch-inactive agent guardrail field %q", path, stale)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal source: %v", err)
	}
}
