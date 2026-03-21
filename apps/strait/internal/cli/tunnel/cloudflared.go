package tunnel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// DetectCloudflared checks PATH for the cloudflared binary and returns its
// absolute path. Returns an empty string if the binary is not found.
func DetectCloudflared() string {
	path, err := exec.LookPath("cloudflared")
	if err != nil {
		return ""
	}
	return path
}

// DownloadURL returns the platform-specific download URL for the cloudflared
// binary from Cloudflare's official releases. It supports darwin/arm64,
// darwin/amd64, linux/amd64, and linux/arm64.
func DownloadURL() (string, error) {
	const base = "https://github.com/cloudflare/cloudflared/releases/latest/download"

	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64":
		return base + "/cloudflared-darwin-arm64.tgz", nil
	case "darwin/amd64":
		return base + "/cloudflared-darwin-amd64.tgz", nil
	case "linux/amd64":
		return base + "/cloudflared-linux-amd64", nil
	case "linux/arm64":
		return base + "/cloudflared-linux-arm64", nil
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

// CachePath returns the path where the downloaded cloudflared binary is cached.
// The binary is stored under the strait config directory at bin/cloudflared.
func CachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "strait", "bin", "cloudflared")
}
