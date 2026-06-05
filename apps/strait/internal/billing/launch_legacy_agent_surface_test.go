package billing

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLaunchSourceDoesNotExposeLegacyAgentGuardrails(t *testing.T) {
	t.Parallel()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)

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
			assert.NotContains(t, string(
				content), stale)
		}
		return nil
	})
	require.NoError(t, err)
}
