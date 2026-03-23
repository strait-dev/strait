package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// HookContext carries contextual information passed to hook executables via stdin.
type HookContext struct {
	Hook      string         `json:"hook"`
	JobSlug   string         `json:"job_slug,omitempty"`
	ProjectID string         `json:"project_id,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// ExecuteHooks runs the named hook across all plugins in pluginDir.
// For pre-* hooks, a non-zero exit code returns an error (blocking the action).
// For post-* hooks, a non-zero exit code logs a warning but does not block.
// If STRAIT_SKIP_HOOKS=1 is set, all hooks are skipped.
func ExecuteHooks(ctx context.Context, hook string, hctx HookContext, pluginDir string, timeout time.Duration) error {
	if os.Getenv("STRAIT_SKIP_HOOKS") == "1" {
		return nil
	}

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading plugin directory: %w", err)
	}

	isPre := strings.HasPrefix(hook, "pre-")

	ctxJSON, err := json.Marshal(hctx)
	if err != nil {
		return fmt.Errorf("marshaling hook context: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(pluginDir, entry.Name(), "strait-plugin.json")
		data, err := os.ReadFile(manifestPath) //nolint:gosec // plugin dir from config path
		if err != nil {
			continue // no manifest, skip
		}

		manifest, err := ParseManifest(data)
		if err != nil {
			continue // invalid manifest, skip
		}

		if !slices.Contains(manifest.Hooks, hook) {
			continue
		}

		hookBin := filepath.Join(pluginDir, entry.Name(), "hooks", hook)
		if _, err := os.Stat(hookBin); err != nil {
			continue // hook executable not found
		}

		if err := runHook(ctx, hookBin, ctxJSON, timeout); err != nil {
			if isPre {
				return fmt.Errorf("pre-hook %q from plugin %q failed: %w", hook, manifest.Name, err)
			}
			slog.Warn("post-hook failed",
				"hook", hook,
				"plugin", manifest.Name,
				"error", err,
			)
		}
	}

	return nil
}

func runHook(ctx context.Context, binPath string, stdinData []byte, timeout time.Duration) error {
	// Verify the hook binary is not world-writable (prevents trivial tampering).
	info, statErr := os.Stat(binPath)
	if statErr != nil {
		return fmt.Errorf("stat hook binary: %w", statErr)
	}
	if info.Mode()&0o002 != 0 {
		return fmt.Errorf("refusing to execute hook %q: file is world-writable (mode %s)", binPath, info.Mode())
	}
	// Reject symlinks to prevent redirect-to-attacker-binary attacks.
	linkInfo, lstatErr := os.Lstat(binPath)
	if lstatErr != nil {
		return fmt.Errorf("lstat hook binary: %w", lstatErr)
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to execute hook %q: file is a symlink", binPath)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath)
	cmd.Stdin = strings.NewReader(string(stdinData))
	cmd.WaitDelay = 3 * time.Second

	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		_, _ = os.Stdout.Write(output)
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("hook timed out after %s", timeout)
		}
		return fmt.Errorf("hook exited with error: %w", err)
	}
	return nil
}
