package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const straitignoreTemplate = `# Strait ignore file — same syntax as .gitignore
# Patterns listed here are excluded from 'strait deploy' tarballs.

# Version control
.git
.gitignore
.straitignore

# Secrets / environment
.env
.env.*
*.pem
*.key

# Editor / OS
.DS_Store
.idea
.vscode
*.swp

# Logs and temporaries
*.log
*.tmp
tmp/

`

var runtimeStraitignore = map[string]string{
	"typescript": `# Node
node_modules/
dist/
build/
.next/
.nuxt/
out/
*.js.map
*.d.ts
coverage/
`,
	"python": `# Python
__pycache__/
*.pyc
*.pyo
*.egg-info/
*.egg
.eggs/
venv/
.venv/
env/
.pytest_cache/
.mypy_cache/
.tox/
dist/
build/
`,
	"go": `# Go
*.test
*.out
vendor/
`,
	"ruby": `# Ruby
.bundle/
vendor/bundle/
tmp/
log/
`,
	"rust": `# Rust
target/
`,
}

func newInitCommand() *cobra.Command {
	var (
		runtime   string
		projectID string
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a strait.json and .straitignore in the current directory",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runInit(runtime, projectID)
		},
	}
	cmd.Flags().StringVar(&runtime, "runtime", "", "runtime: python|typescript|go|ruby|rust (auto-detected if omitted)")
	cmd.Flags().StringVar(&projectID, "project", "", "project ID from the Strait dashboard")
	return cmd
}

func runInit(runtime, projectID string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	straitJSONPath := filepath.Join(cwd, "strait.json")
	if _, err := os.Stat(straitJSONPath); err == nil {
		return fmt.Errorf("strait.json already exists in this directory — remove it first or edit it directly")
	}

	// Auto-detect runtime if not provided.
	if runtime == "" {
		runtime = detectRuntimeInDir(cwd)
		if runtime == "" {
			runtime = "typescript"
			fmt.Fprintln(os.Stderr, "Runtime not detected — defaulting to typescript. Use --runtime to override.")
		} else {
			fmt.Fprintf(os.Stderr, "Detected runtime: %s\n", runtime)
		}
	}

	cfg := buildStraitJSON(runtime, projectID)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal strait.json: %w", err)
	}
	if err := os.WriteFile(straitJSONPath, append(data, '\n'), 0o644); err != nil { //nolint:gosec // G306: strait.json is a committed project config, not a secret
		return fmt.Errorf("write strait.json: %w", err)
	}
	fmt.Printf("Created %s\n", straitJSONPath)

	// Write .straitignore only if it doesn't already exist.
	ignorePath := filepath.Join(cwd, ".straitignore")
	if _, err := os.Stat(ignorePath); os.IsNotExist(err) {
		ignoreContent := straitignoreTemplate
		if extra, ok := runtimeStraitignore[runtime]; ok {
			ignoreContent += extra
		}
		if err := os.WriteFile(ignorePath, []byte(ignoreContent), 0o644); err != nil { //nolint:gosec // G306: .straitignore is a committed project config, not a secret
			return fmt.Errorf("write .straitignore: %w", err)
		}
		fmt.Printf("Created %s\n", ignorePath)
	}

	fmt.Println()
	if projectID == "" {
		fmt.Println("Next steps:")
		fmt.Println("  1. Set project.id in strait.json (from the Strait dashboard)")
		fmt.Println("  2. Run: strait auth login")
		fmt.Println("  3. Run: strait deploy --job <slug>")
	} else {
		fmt.Println("Next steps:")
		fmt.Println("  1. Run: strait auth login")
		fmt.Println("  2. Run: strait deploy --job <slug>")
	}
	return nil
}

func buildStraitJSON(runtime, projectID string) map[string]any {
	proj := map[string]any{
		"id": projectID,
	}
	if projectID == "" {
		proj["id"] = "proj_REPLACE_ME"
	}

	deploy := map[string]any{
		"runtime": runtime,
	}
	switch runtime {
	case "typescript":
		deploy["build_command"] = "npm run build"
		deploy["output_dir"] = "dist"
	case "python":
		// no build command typically needed
	case "go":
		deploy["build_command"] = "go build ./..."
	}

	return map[string]any{
		"$schema": "https://api.strait.dev/schemas/v1/strait.json",
		"project": proj,
		"deploy":  deploy,
		"worker": map[string]any{
			"concurrency": 4,
		},
	}
}

// detectRuntimeInDir auto-detects the runtime from files present in dir.
func detectRuntimeInDir(dir string) string {
	checks := []struct {
		file    string
		runtime string
	}{
		{"package.json", "typescript"},
		{"requirements.txt", "python"},
		{"pyproject.toml", "python"},
		{"go.mod", "go"},
		{"Gemfile", "ruby"},
		{"Cargo.toml", "rust"},
	}
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(dir, c.file)); err == nil {
			return c.runtime
		}
	}
	return ""
}
