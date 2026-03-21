package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	climanifest "strait/internal/cli/manifest"

	"github.com/spf13/cobra"
)

func newBuildCommand(state *appState) *cobra.Command {
	var configPath string
	var outDir string
	var dryRun bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Compile project config into a deployment manifest",
		Long: `Loads a strait.json or strait.config.yaml/.yml config file, compiles it into a
deterministic manifest with sorted resources and a SHA-256 checksum,
and writes it to the output directory.`,
		Example: `  strait build
  strait build --config strait.json --out-dir .strait
  strait build --dry-run --json`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if configPath == "" {
				configPath = climanifest.FindConfigFile(".")
				if configPath == "" {
					return fmt.Errorf("no config file found; create strait.json or strait.config.yaml/.yml, or use --config")
				}
			}

			cfg, err := climanifest.LoadProjectConfig(configPath)
			if err != nil {
				return err
			}

			m := climanifest.BuildManifest(cfg)

			encoded, err := json.MarshalIndent(m, "", "  ")
			if err != nil {
				return fmt.Errorf("encode manifest: %w", err)
			}

			if dryRun || asJSON {
				fmt.Println(string(encoded))
				return nil
			}

			if outDir == "" {
				outDir = cfg.Build.OutDir
			}
			if outDir == "" {
				outDir = ".strait"
			}

			if err := os.MkdirAll(outDir, 0o750); err != nil {
				return fmt.Errorf("create output directory: %w", err)
			}

			target := filepath.Join(outDir, "manifest.json")
			if err := os.WriteFile(target, append(encoded, '\n'), 0o600); err != nil {
				return fmt.Errorf("write manifest: %w", err)
			}

			return printData(state, map[string]any{
				"config":    configPath,
				"output":    target,
				"checksum":  m.Checksum,
				"jobs":      len(m.Jobs),
				"workflows": len(m.Workflows),
			})
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "path to config file")
	cmd.Flags().StringVar(&outDir, "out-dir", "", "output directory (default: .strait)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print manifest without writing")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print manifest JSON to stdout")

	return cmd
}
