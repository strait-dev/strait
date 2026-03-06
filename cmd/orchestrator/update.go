package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	updateCheckCacheDuration = 24 * time.Hour
	githubReleasesURL        = "https://api.github.com/repos/leonardomso/orchestrator/releases/latest"
)

type updateCheckCache struct {
	LatestVersion string    `json:"latest_version"`
	CheckedAt     time.Time `json:"checked_at"`
}

// checkForUpdate queries GitHub releases API for the latest version.
// Returns the latest version tag or empty string on error.
func checkForUpdate() string {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(githubReleasesURL) //nolint:noctx // fire-and-forget background check
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return ""
	}

	return strings.TrimPrefix(release.TagName, "v")
}

// getCachedUpdate returns the cached latest version if the cache is fresh.
func getCachedUpdate() (string, bool) {
	cachePath := updateCachePath()
	if cachePath == "" {
		return "", false
	}

	data, err := os.ReadFile(cachePath) //nolint:gosec // cache file from known path
	if err != nil {
		return "", false
	}

	var cache updateCheckCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return "", false
	}

	if time.Since(cache.CheckedAt) > updateCheckCacheDuration {
		return "", false
	}

	return cache.LatestVersion, true
}

// setCachedUpdate writes the latest version to the cache file.
func setCachedUpdate(latestVersion string) {
	cachePath := updateCachePath()
	if cachePath == "" {
		return
	}

	cache := updateCheckCache{
		LatestVersion: latestVersion,
		CheckedAt:     time.Now(),
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}

	dir := filepath.Dir(cachePath)
	_ = os.MkdirAll(dir, 0o750)
	_ = os.WriteFile(cachePath, data, 0o644) //nolint:gosec // cache file with standard permissions
}

func updateCachePath() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "orchestrator", "update-check.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "orchestrator", "update-check.json")
}

func newUpgradeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Check for CLI updates",
		RunE: func(_ *cobra.Command, _ []string) error {
			latest := checkForUpdate()
			if latest == "" {
				return fmt.Errorf("failed to check for updates")
			}

			setCachedUpdate(latest)

			current := strings.TrimPrefix(version, "v")
			if current == latest {
				fmt.Printf("Already up to date (v%s)\n", current)
				return nil
			}

			fmt.Printf("Current: v%s\nLatest:  v%s\n", current, latest)
			fmt.Println("\nTo upgrade, re-install via your package manager or download from:")
			fmt.Printf("  https://github.com/leonardomso/orchestrator/releases/tag/v%s\n", latest)
			return nil
		},
	}

	return cmd
}
