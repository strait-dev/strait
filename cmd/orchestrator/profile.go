package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newProfileCommand(state *appState) *cobra.Command {
	var (
		output   string
		duration time.Duration
		kind     string
	)

	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Capture pprof profile from the running server",
		Long: `Downloads a pprof profile from the server's /debug/pprof endpoint.

Requires the server to be running with pprof enabled. Supports CPU, heap,
goroutine, allocs, block, mutex, and threadcreate profiles.`,
		Example: `  orchestrator profile
  orchestrator profile --type cpu --duration 30s --output cpu.prof
  orchestrator profile --type heap --output heap.prof
  orchestrator profile --type goroutine`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			serverURL := strings.TrimRight(state.opts.serverURL, "/")
			if serverURL == "" {
				return fmt.Errorf("server URL required: set --server or configure a context")
			}

			profileURL := buildProfileURL(serverURL, kind, duration)

			if output == "" {
				output = fmt.Sprintf("%s-%s.prof", kind, time.Now().UTC().Format("20060102-150405"))
			}

			if state.opts.verbose {
				fmt.Fprintf(os.Stderr, "fetching %s profile from %s\n", kind, profileURL)
			}

			client := &http.Client{
				Timeout: duration + 10*time.Second, // extra headroom beyond profile duration
			}

			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, profileURL, nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}

			if state.opts.apiKey != "" {
				req.Header.Set("Authorization", "Bearer "+state.opts.apiKey)
			}

			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("failed to fetch profile: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
				return fmt.Errorf("server returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
			}

			f, err := os.Create(output)
			if err != nil {
				return fmt.Errorf("failed to create output file: %w", err)
			}
			defer f.Close()

			n, err := io.Copy(f, resp.Body)
			if err != nil {
				return fmt.Errorf("failed to write profile data: %w", err)
			}

			fmt.Fprintf(os.Stderr, "profile written to %s (%d bytes)\n", output, n)

			if kind == "cpu" {
				fmt.Fprintf(os.Stderr, "analyze with: go tool pprof %s\n", output)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path (default: <type>-<timestamp>.prof)")
	cmd.Flags().DurationVar(&duration, "duration", 30*time.Second, "profile duration (for CPU profiles)")
	cmd.Flags().StringVar(&kind, "type", "cpu", "profile type: cpu, heap, goroutine, allocs, block, mutex, threadcreate")

	return cmd
}

func buildProfileURL(serverURL, kind string, duration time.Duration) string {
	base := serverURL + "/debug/pprof"

	switch kind {
	case "cpu":
		return fmt.Sprintf("%s/profile?seconds=%d", base, int(duration.Seconds()))
	case "heap":
		return base + "/heap"
	case "goroutine":
		return base + "/goroutine"
	case "allocs":
		return base + "/allocs"
	case "block":
		return base + "/block"
	case "mutex":
		return base + "/mutex"
	case "threadcreate":
		return base + "/threadcreate"
	default:
		return fmt.Sprintf("%s/%s", base, kind)
	}
}
