package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

func newDebugCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "debug",
		Short: "Debugging tools",
	}

	cmd.AddCommand(newDebugBundleCommand(state))

	return cmd
}

func newDebugBundleCommand(state *appState) *cobra.Command {
	var outputPath string
	var noEvents bool

	cmd := &cobra.Command{
		Use:   "bundle <run-id>",
		Short: "Collect diagnostics into a shareable archive",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			run, err := cli.GetRun(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("fetch run: %w", err)
			}

			job, _ := cli.GetJob(cmd.Context(), run.JobID)

			var events any
			if !noEvents {
				evts, evtErr := cli.ListRunEvents(cmd.Context(), runID, "", "")
				if evtErr == nil {
					events = evts
				}
			}

			env := map[string]string{
				"go_version":  runtime.Version(),
				"os_arch":     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
				"cli_version": version,
				"server_url":  state.opts.serverURL,
				"api_key":     maskKey(state.opts.apiKey),
				"project_id":  state.opts.projectID,
			}

			if outputPath == "" {
				outputPath = fmt.Sprintf("strait-debug-%s-%d.zip", runID, time.Now().Unix())
			}

			f, err := os.Create(outputPath) //nolint:gosec // user-controlled output path for debug bundle
			if err != nil {
				return fmt.Errorf("create zip: %w", err)
			}
			defer f.Close()

			w := zip.NewWriter(f)
			defer w.Close()

			writeJSON(w, "run.json", run)
			if job != nil {
				writeJSON(w, "job.json", job)
			}
			if events != nil {
				writeJSON(w, "events.json", events)
			}
			writeJSON(w, "env.json", env)

			absPath, _ := filepath.Abs(outputPath)
			return printData(state, map[string]any{
				"bundle": absPath,
				"run_id": runID,
			})
		},
	}

	cmd.Flags().StringVar(&outputPath, "output", "", "output file path")
	cmd.Flags().BoolVar(&noEvents, "no-events", false, "skip event collection")

	return cmd
}

func writeJSON(w *zip.Writer, name string, data any) {
	f, err := w.Create(name)
	if err != nil {
		return
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)
}

func maskKey(key string) string {
	if len(key) <= 4 {
		return "***"
	}
	return "..." + key[len(key)-4:]
}
