package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

func newOpenCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open [resource-id]",
		Short: "Open dashboard in browser",
		Long: `Opens the strait dashboard in your default browser.
Optionally pass a run ID or job slug to open a specific page.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			base := strings.TrimRight(state.opts.serverURL, "/")
			if base == "" {
				return fmt.Errorf("server URL is required")
			}

			// Derive dashboard URL from server URL
			dashURL := strings.Replace(base, ":8080", ":5173", 1)
			dashURL = strings.Replace(dashURL, "api.", "app.", 1)

			target := dashURL
			if len(args) == 1 {
				resource := args[0]
				if strings.HasPrefix(resource, "run_") || strings.HasPrefix(resource, "run-") {
					target = dashURL + "/runs/" + resource
				} else {
					target = dashURL + "/jobs/" + resource
				}
			}

			return openBrowser(target)
		},
	}

	return cmd
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url) //nolint:gosec // URL is derived from configured server URL
	case "linux":
		cmd = exec.Command("xdg-open", url) //nolint:gosec // URL is derived from configured server URL
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url) //nolint:gosec // URL is derived from configured server URL
	default:
		return fmt.Errorf("unsupported platform for browser open; visit: %s", url)
	}
	return cmd.Start()
}
