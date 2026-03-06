package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	cobraDoc "github.com/spf13/cobra/doc"
)

func newDocsCommand(root *cobra.Command) *cobra.Command {
	var man bool
	var markdown bool
	var outDir string

	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Generate CLI documentation",
		RunE: func(_ *cobra.Command, _ []string) error {
			if !man && !markdown {
				return fmt.Errorf("at least one of --man or --markdown is required")
			}

			dir := outDir
			if dir == "" {
				dir = filepath.Join("docs", "cli")
			}
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return err
			}

			if man {
				header := &cobraDoc.GenManHeader{
					Title:   "ORCHESTRATOR",
					Section: "1",
					Source:  "orchestrator",
					Manual:  "Orchestrator CLI",
					Date:    &time.Time{},
				}
				if err := cobraDoc.GenManTree(root, header, filepath.Join(dir, "man")); err != nil {
					return err
				}
			}

			if markdown {
				if err := cobraDoc.GenMarkdownTree(root, filepath.Join(dir, "markdown")); err != nil {
					return err
				}
			}

			return printData(&appState{opts: &rootOptions{outputFormat: "table"}}, map[string]any{
				"generated": true,
				"output":    dir,
				"man":       man,
				"markdown":  markdown,
			})
		},
	}

	cmd.Flags().BoolVar(&man, "man", false, "generate man pages")
	cmd.Flags().BoolVar(&markdown, "markdown", false, "generate markdown docs")
	cmd.Flags().StringVar(&outDir, "output-dir", "", "output directory")

	return cmd
}
