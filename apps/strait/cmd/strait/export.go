package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"strait/internal/cli/client"
	"strait/internal/cli/styles"

	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
)

type exportDocument struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   manifestMeta   `yaml:"metadata"`
	Spec       map[string]any `yaml:"spec"`
}

func newExportCommand(state *appState) *cobra.Command {
	var projectID string
	var outputDir string
	var nameContains string
	var dryRun bool
	var forceOverwrite bool

	cmd := &cobra.Command{
		Use:       "export <resource>",
		Short:     "Export server state as declarative YAML",
		Long:      "Exports jobs, workflows, and API keys from the server into declarative YAML documents.",
		Example:   "strait export jobs --project proj_1\n  strait export all --project proj_1 --output-dir definitions\n  strait export workflows --name-contains billing --dry-run",
		ValidArgs: []string{"jobs", "workflows", "api-keys", "all"},
		Args:      cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resource := strings.ToLower(strings.TrimSpace(args[0]))
			var err error
			projectID, err = requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			docs, err := exportDocuments(cmd.Context(), cli, projectID, resource)
			if err != nil {
				return err
			}

			if strings.TrimSpace(nameContains) != "" {
				needle := strings.ToLower(strings.TrimSpace(nameContains))
				filtered := make([]exportDocument, 0, len(docs))
				for _, doc := range docs {
					if strings.Contains(strings.ToLower(doc.Metadata.Name), needle) {
						filtered = append(filtered, doc)
					}
				}
				docs = filtered
			}

			if len(docs) == 0 {
				return fmt.Errorf("no resources exported")
			}

			if dryRun {
				counts := map[string]int{}
				for _, doc := range docs {
					counts[strings.ToLower(doc.Kind)]++
				}
				return printData(state, map[string]any{
					"dry_run":  true,
					"resource": resource,
					"count":    len(docs),
					"kinds":    counts,
				})
			}

			if outputDir == "" {
				return writeYAMLStream(os.Stdout, docs)
			}

			if err := os.MkdirAll(outputDir, 0o750); err != nil {
				return fmt.Errorf("creating output directory: %w", err)
			}

			paths, err := writeYAMLFiles(outputDir, docs, forceOverwrite)
			if err != nil {
				return err
			}

			if isTTYRich(state) {
				for _, p := range paths {
					fmt.Fprintln(os.Stderr, styles.Success("Exported "+styles.FilePath(p)))
				}
				return nil
			}
			return printData(state, map[string]any{
				"resource":   resource,
				"output_dir": outputDir,
				"count":      len(paths),
				"files":      paths,
			})
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "write one file per manifest into directory")
	cmd.Flags().StringVar(&nameContains, "name-contains", "", "filter manifests by metadata.name substring")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview what would be exported without writing output")
	cmd.Flags().BoolVar(&forceOverwrite, "force-overwrite", false, "overwrite existing output files in --output-dir mode")

	_ = cmd.RegisterFlagCompletionFunc("project", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		if strings.TrimSpace(state.opts.projectID) == "" {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return []string{state.opts.projectID}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func exportDocuments(ctx context.Context, cli *client.Client, projectID, resource string) ([]exportDocument, error) {
	switch resource {
	case "jobs", "job":
		return exportJobs(ctx, cli, projectID)
	case "workflows", "workflow":
		return exportWorkflows(ctx, cli, projectID)
	case "api-keys", "apikeys", "api-key":
		return exportAPIKeys(ctx, cli, projectID)
	case "all":
		jobs, err := exportJobs(ctx, cli, projectID)
		if err != nil {
			return nil, fmt.Errorf("exporting jobs: %w", err)
		}
		workflows, err := exportWorkflows(ctx, cli, projectID)
		if err != nil {
			return nil, fmt.Errorf("exporting workflows: %w", err)
		}
		keys, err := exportAPIKeys(ctx, cli, projectID)
		if err != nil {
			return nil, fmt.Errorf("exporting API keys: %w", err)
		}
		out := make([]exportDocument, 0, len(jobs)+len(workflows)+len(keys))
		out = append(out, jobs...)
		out = append(out, workflows...)
		out = append(out, keys...)
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported resource %q", resource)
	}
}

func exportJobs(ctx context.Context, cli *client.Client, projectID string) ([]exportDocument, error) {
	jobs, err := cli.ListJobs(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing jobs: %w", err)
	}
	docs := make([]exportDocument, 0, len(jobs))
	for _, job := range jobs {
		docs = append(docs, exportDocument{
			APIVersion: "v1",
			Kind:       "Job",
			Metadata:   manifestMeta{Name: job.Name},
			Spec: map[string]any{
				"project_id":   job.ProjectID,
				"slug":         job.Slug,
				"description":  job.Description,
				"cron":         job.Cron,
				"endpoint_url": job.EndpointURL,
				"max_attempts": job.MaxAttempts,
				"timeout_secs": job.TimeoutSecs,
				"enabled":      job.Enabled,
				"run_ttl_secs": job.RunTTLSecs,
			},
		})
	}
	return docs, nil
}

func exportWorkflows(ctx context.Context, cli *client.Client, projectID string) ([]exportDocument, error) {
	workflows, err := cli.ListWorkflows(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing workflows: %w", err)
	}
	docs := make([]exportDocument, 0, len(workflows))
	for _, wf := range workflows {
		detail, err := cli.GetWorkflow(ctx, wf.ID)
		if err != nil {
			return nil, fmt.Errorf("fetching workflow %s: %w", wf.ID, err)
		}

		steps := make([]map[string]any, 0, len(detail.Steps))
		for _, step := range detail.Steps {
			entry := map[string]any{
				"job_id":     step.JobID,
				"step_ref":   step.StepRef,
				"depends_on": step.DependsOn,
				"on_failure": step.OnFailure,
			}
			if len(step.Condition) > 0 {
				entry["condition"] = string(step.Condition)
			}
			if len(step.Payload) > 0 {
				entry["payload"] = string(step.Payload)
			}
			steps = append(steps, entry)
		}

		docs = append(docs, exportDocument{
			APIVersion: "v1",
			Kind:       "Workflow",
			Metadata:   manifestMeta{Name: wf.Name},
			Spec: map[string]any{
				"project_id":  wf.ProjectID,
				"slug":        wf.Slug,
				"description": wf.Description,
				"enabled":     wf.Enabled,
				"steps":       steps,
			},
		})
	}
	return docs, nil
}

func exportAPIKeys(ctx context.Context, cli *client.Client, projectID string) ([]exportDocument, error) {
	keys, err := cli.ListAPIKeys(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing API keys: %w", err)
	}
	docs := make([]exportDocument, 0, len(keys))
	for _, key := range keys {
		docs = append(docs, exportDocument{
			APIVersion: "v1",
			Kind:       "APIKey",
			Metadata:   manifestMeta{Name: key.Name},
			Spec: map[string]any{
				"project_id": key.ProjectID,
				"scopes":     key.Scopes,
				"key_prefix": key.KeyPrefix,
			},
		})
	}
	return docs, nil
}

func writeYAMLStream(w io.Writer, docs []exportDocument) error {
	enc := yaml.NewEncoder(w)
	defer enc.Close()
	for _, doc := range docs {
		if err := enc.Encode(doc); err != nil {
			return err
		}
	}
	return nil
}

func writeYAMLFiles(outputDir string, docs []exportDocument, forceOverwrite bool) ([]string, error) {
	paths := make([]string, 0, len(docs))
	for i, doc := range docs {
		name := sanitizeFilename(doc.Metadata.Name)
		if name == "" {
			name = fmt.Sprintf("%s-%d", strings.ToLower(doc.Kind), i+1)
		}
		path := filepath.Join(outputDir, fmt.Sprintf("%s.yaml", name))
		if !forceOverwrite {
			if _, err := os.Stat(path); err == nil {
				return nil, fmt.Errorf("refusing to overwrite existing file %q without --force-overwrite", path)
			} else if !os.IsNotExist(err) {
				return nil, err
			}
		}
		content, err := yaml.Marshal(doc)
		if err != nil {
			return nil, fmt.Errorf("marshaling YAML for %s: %w", doc.Metadata.Name, err)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			return nil, fmt.Errorf("writing %s: %w", path, err)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func sanitizeFilename(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	return s
}
