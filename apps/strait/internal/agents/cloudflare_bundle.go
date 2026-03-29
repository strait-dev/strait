package agents

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
)

//go:generate bash -c "cd ../../../../apps/agents && bun run build:runtime && cp dist/runtime/worker.js ../../apps/strait/internal/agents/runtime_worker_bundle.js"

//go:embed runtime_worker_bundle.js
var embeddedCloudflareRuntimeWorker string

var cloudflareRuntimeBundleCandidates = []string{
	filepath.Join("apps", "agents", "dist", "runtime", "worker.js"),
	filepath.Join("..", "agents", "dist", "runtime", "worker.js"),
	filepath.Join("dist", "runtime", "worker.js"),
}

func cloudflareRuntimeSource() string {
	return cloudflareRuntimeSourceFromPaths(cloudflareRuntimeBundleCandidates)
}

func cloudflareRuntimeSourceFromPaths(paths []string) string {
	for _, candidate := range paths {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		raw, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		if source := strings.TrimSpace(string(raw)); source != "" {
			return source
		}
	}

	return strings.TrimSpace(embeddedCloudflareRuntimeWorker)
}
