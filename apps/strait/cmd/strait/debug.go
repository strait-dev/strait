package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"strait/internal/cli/styles"

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

			job, jobErr := cli.GetJob(cmd.Context(), run.JobID)
			if jobErr != nil {
				fmt.Fprintf(os.Stderr, "warning: could not fetch job %s: %v\n", run.JobID, jobErr)
			}

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
				"api_key":     maskBundleKey(state.opts.apiKey),
				"project_id":  state.opts.projectID,
			}

			if outputPath == "" {
				outputPath = fmt.Sprintf("strait-debug-%s-%d.zip", runID, time.Now().Unix())
			}

			f, err := os.Create(outputPath) //nolint:gosec // user-controlled output path for debug bundle
			if err != nil {
				return fmt.Errorf("create zip: %w", err)
			}

			w := zip.NewWriter(f)

			var writeErr error
			if err := writeJSON(w, "run.json", run); err != nil {
				writeErr = fmt.Errorf("write run.json: %w", err)
			}
			if writeErr == nil && job != nil {
				if err := writeJSON(w, "job.json", job); err != nil {
					writeErr = fmt.Errorf("write job.json: %w", err)
				}
			}
			if writeErr == nil && events != nil {
				if err := writeJSON(w, "events.json", events); err != nil {
					writeErr = fmt.Errorf("write events.json: %w", err)
				}
			}
			if writeErr == nil {
				if err := writeJSON(w, "env.json", env); err != nil {
					writeErr = fmt.Errorf("write env.json: %w", err)
				}
			}

			// Close zip writer before file to ensure data is flushed.
			if closeErr := w.Close(); closeErr != nil && writeErr == nil {
				writeErr = fmt.Errorf("finalize zip: %w", closeErr)
			}
			if closeErr := f.Close(); closeErr != nil && writeErr == nil {
				writeErr = fmt.Errorf("close file: %w", closeErr)
			}

			if writeErr != nil {
				_ = os.Remove(outputPath) // Clean up partial file.
				return writeErr
			}

			absPath, _ := filepath.Abs(outputPath)
			if isTTYRich(state) {
				fmt.Fprintln(os.Stderr, styles.Success("Debug bundle created"))
				fmt.Fprintln(os.Stderr, styles.KeyValue("Path", styles.FilePath(absPath)))
				fmt.Fprintln(os.Stderr, styles.KeyValue("Run", runID))
				return nil
			}
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

func writeJSON(w *zip.Writer, name string, data any) error {
	f, err := w.Create(name)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func maskBundleKey(key string) string {
	if len(key) <= 4 {
		return "***"
	}
	return "..." + key[len(key)-4:]
}
