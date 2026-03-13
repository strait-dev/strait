package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"strait/internal/cli/client"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type manifest struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   manifestMeta   `yaml:"metadata"`
	Spec       map[string]any `yaml:"spec"`
}

type manifestMeta struct {
	Name string `yaml:"name"`
}

type loadedManifest struct {
	Source string
	Index  int
	Data   manifest
}

func newValidateCommand(state *appState) *cobra.Command {
	var files []string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate declarative definition files",
		RunE: func(_ *cobra.Command, _ []string) error {
			manifests, err := loadManifestInputs(files)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(manifests))
			for _, item := range manifests {
				if err := validateManifest(item.Data); err != nil {
					return fmt.Errorf("%s#%d: %w", item.Source, item.Index, err)
				}
				rows = append(rows, map[string]any{
					"source": item.Source,
					"kind":   item.Data.Kind,
					"name":   item.Data.Metadata.Name,
					"valid":  true,
				})
			}

			if len(rows) == 0 {
				return fmt.Errorf("no manifests found")
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringArrayVarP(&files, "file", "f", nil, "manifest file, directory, or - for stdin")

	return cmd
}

func newApplyCommand(state *appState) *cobra.Command {
	var files []string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply declarative definitions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			manifests, err := loadManifestInputs(files)
			if err != nil {
				return err
			}
			if len(manifests) == 0 {
				return fmt.Errorf("no manifests found")
			}

			for _, item := range manifests {
				if err := validateManifest(item.Data); err != nil {
					return fmt.Errorf("%s#%d: %w", item.Source, item.Index, err)
				}
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(manifests))
			for _, item := range manifests {
				if dryRun {
					rows = append(rows, map[string]any{"action": "dry-run", "kind": item.Data.Kind, "name": item.Data.Metadata.Name})
					continue
				}

				result, err := applyManifest(cmd.Context(), cli, item.Data)
				if err != nil {
					return fmt.Errorf("apply %s/%s: %w", item.Data.Kind, item.Data.Metadata.Name, err)
				}
				rows = append(rows, result)
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringArrayVarP(&files, "file", "f", nil, "manifest file, directory, or - for stdin")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate and preview without creating resources")

	return cmd
}

func newDiffCommand(state *appState) *cobra.Command {
	var files []string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show declarative changes against server state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			manifests, err := loadManifestInputs(files)
			if err != nil {
				return err
			}
			if len(manifests) == 0 {
				return fmt.Errorf("no manifests found")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(manifests))
			for _, item := range manifests {
				if err := validateManifest(item.Data); err != nil {
					return fmt.Errorf("%s#%d: %w", item.Source, item.Index, err)
				}
				action, err := diffManifest(cmd.Context(), cli, state, item.Data)
				if err != nil {
					return err
				}
				rows = append(rows, map[string]any{
					"kind":   item.Data.Kind,
					"name":   item.Data.Metadata.Name,
					"action": action,
				})
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringArrayVarP(&files, "file", "f", nil, "manifest file, directory, or - for stdin")

	return cmd
}

func loadManifestInputs(inputs []string) ([]loadedManifest, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("at least one -f/--file is required")
	}

	out := make([]loadedManifest, 0)
	for _, input := range inputs {
		if strings.TrimSpace(input) == "-" {
			items, err := decodeManifestReader("stdin", os.Stdin)
			if err != nil {
				return nil, err
			}
			out = append(out, items...)
			continue
		}

		st, err := os.Stat(input)
		if err != nil {
			return nil, err
		}
		if st.IsDir() {
			items, err := decodeManifestDirectory(input)
			if err != nil {
				return nil, err
			}
			out = append(out, items...)
			continue
		}

		f, err := os.Open(input) //nolint:gosec // input is a CLI-provided manifest file path
		if err != nil {
			return nil, err
		}
		items, err := decodeManifestReader(input, f)
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}

	return out, nil
}

func decodeManifestDirectory(dir string) ([]loadedManifest, error) {
	out := make([]loadedManifest, 0)
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			return nil
		}
		f, err := os.Open(path) //nolint:gosec // path is from filepath.WalkDir within user-provided directory
		if err != nil {
			return err
		}
		items, err := decodeManifestReader(path, f)
		_ = f.Close()
		if err != nil {
			return err
		}
		out = append(out, items...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func decodeManifestReader(source string, r io.Reader) ([]loadedManifest, error) {
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, bufio.NewReader(r)); err != nil {
		return nil, err
	}

	dec := yaml.NewDecoder(bytes.NewReader(buf.Bytes()))
	out := make([]loadedManifest, 0)
	index := 0
	for {
		index++
		var m manifest
		err := dec.Decode(&m)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s#%d: %w", source, index, err)
		}
		if strings.TrimSpace(m.Kind) == "" {
			continue
		}
		out = append(out, loadedManifest{Source: source, Index: index, Data: m})
	}

	return out, nil
}

func validateManifest(m manifest) error {
	if strings.TrimSpace(m.Kind) == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(m.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if m.Spec == nil {
		return fmt.Errorf("spec is required")
	}

	switch strings.ToLower(m.Kind) {
	case "job":
		if getString(m.Spec, "project_id") == "" || getString(m.Spec, "endpoint_url") == "" {
			return fmt.Errorf("job spec requires project_id and endpoint_url")
		}
	case "workflow":
		if getString(m.Spec, "project_id") == "" {
			return fmt.Errorf("workflow spec requires project_id")
		}
	case "api-key", "apikey", "apikeys":
		if getString(m.Spec, "project_id") == "" {
			return fmt.Errorf("api-key spec requires project_id")
		}
	default:
		return fmt.Errorf("unsupported kind %q", m.Kind)
	}

	return nil
}

func applyManifest(ctx context.Context, cli *client.Client, m manifest) (map[string]any, error) {
	kind := strings.ToLower(m.Kind)
	name := strings.TrimSpace(m.Metadata.Name)

	switch kind {
	case "job":
		slug := getSlugOrName(m.Spec, name)
		existing, err := findExistingJob(ctx, cli, getString(m.Spec, "project_id"), slug, name)
		if err != nil {
			return nil, err
		}

		req := client.CreateJobRequest{
			ProjectID:   getString(m.Spec, "project_id"),
			Name:        name,
			Slug:        slug,
			Description: getString(m.Spec, "description"),
			Cron:        getString(m.Spec, "cron"),
			EndpointURL: getString(m.Spec, "endpoint_url"),
			MaxAttempts: getIntDefault(m.Spec, "max_attempts", 3),
			TimeoutSecs: getIntDefault(m.Spec, "timeout_secs", 60),
			RunTTLSecs:  getIntDefault(m.Spec, "run_ttl_secs", 0),
		}

		if existing != nil {
			nameVal := req.Name
			slugVal := req.Slug
			descVal := req.Description
			cronVal := req.Cron
			endpointVal := req.EndpointURL
			maxAttemptsVal := req.MaxAttempts
			timeoutSecsVal := req.TimeoutSecs
			runTTLVal := req.RunTTLSecs

			upd := client.UpdateJobRequest{
				Name:        &nameVal,
				Slug:        &slugVal,
				Description: &descVal,
				Cron:        &cronVal,
				EndpointURL: &endpointVal,
				MaxAttempts: &maxAttemptsVal,
				TimeoutSecs: &timeoutSecsVal,
				RunTTLSecs:  &runTTLVal,
			}
			job, updateErr := cli.UpdateJob(ctx, existing.ID, upd)
			if updateErr != nil {
				return nil, updateErr
			}
			return map[string]any{"action": "updated", "kind": "Job", "name": name, "id": job.ID}, nil
		}

		job, err := cli.CreateJob(ctx, req)
		if err != nil {
			return nil, err
		}
		return map[string]any{"action": "created", "kind": "Job", "name": name, "id": job.ID}, nil
	case "workflow":
		slug := getSlugOrName(m.Spec, name)
		existing, err := findExistingWorkflow(ctx, cli, getString(m.Spec, "project_id"), slug, name)
		if err != nil {
			return nil, err
		}

		req := client.CreateWorkflowRequest{
			ProjectID:   getString(m.Spec, "project_id"),
			Name:        name,
			Slug:        slug,
			Description: getString(m.Spec, "description"),
		}
		if rawSteps, ok := m.Spec["steps"]; ok {
			raw, _ := json.Marshal(rawSteps)
			_ = json.Unmarshal(raw, &req.Steps)
		}

		if existing != nil {
			steps := req.Steps
			nameVal := req.Name
			slugVal := req.Slug
			descVal := req.Description
			upd := client.UpdateWorkflowRequest{
				Name:        &nameVal,
				Slug:        &slugVal,
				Description: &descVal,
				Steps:       &steps,
			}
			wf, updateErr := cli.UpdateWorkflow(ctx, existing.ID, upd)
			if updateErr != nil {
				return nil, updateErr
			}
			return map[string]any{"action": "updated", "kind": "Workflow", "name": name, "id": wf.ID}, nil
		}

		wf, err := cli.CreateWorkflow(ctx, req)
		if err != nil {
			return nil, err
		}
		return map[string]any{"action": "created", "kind": "Workflow", "name": name, "id": wf.ID}, nil
	case "api-key", "apikey", "apikeys":
		req := client.CreateAPIKeyRequest{
			ProjectID: getString(m.Spec, "project_id"),
			Name:      name,
		}
		if scopes, ok := m.Spec["scopes"]; ok {
			raw, _ := json.Marshal(scopes)
			_ = json.Unmarshal(raw, &req.Scopes)
		}
		key, err := cli.CreateAPIKey(ctx, req)
		if err != nil {
			return nil, err
		}
		return map[string]any{"action": "created", "kind": "APIKey", "name": name, "id": key.ID}, nil
	default:
		return nil, fmt.Errorf("unsupported kind %q", m.Kind)
	}
}

type manifestResourceRef struct {
	ID string
}

func findExistingJob(ctx context.Context, cli *client.Client, projectID, slug, name string) (*manifestResourceRef, error) {
	jobs, err := cli.ListJobs(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		if job.Slug == slug || job.Name == name {
			return &manifestResourceRef{ID: job.ID}, nil
		}
	}
	return nil, nil //nolint:nilnil // nil signals resource not found.
}

func findExistingWorkflow(ctx context.Context, cli *client.Client, projectID, slug, name string) (*manifestResourceRef, error) {
	workflows, err := cli.ListWorkflows(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, wf := range workflows {
		if wf.Slug == slug || wf.Name == name {
			return &manifestResourceRef{ID: wf.ID}, nil
		}
	}
	return nil, nil //nolint:nilnil // nil signals resource not found.
}

func diffManifest(ctx context.Context, cli *client.Client, state *appState, m manifest) (string, error) {
	kind := strings.ToLower(m.Kind)
	name := strings.TrimSpace(m.Metadata.Name)

	switch kind {
	case "job":
		jobs, err := cli.ListJobs(ctx, getString(m.Spec, "project_id"))
		if err != nil {
			return "", err
		}
		slug := getSlugOrName(m.Spec, name)
		for _, job := range jobs {
			if job.Slug == slug || job.Name == name {
				return "update", nil
			}
		}
		return "create", nil
	case "workflow":
		workflows, err := cli.ListWorkflows(ctx, getString(m.Spec, "project_id"))
		if err != nil {
			return "", err
		}
		slug := getSlugOrName(m.Spec, name)
		for _, wf := range workflows {
			if wf.Slug == slug || wf.Name == name {
				return "update", nil
			}
		}
		return "create", nil
	case "api-key", "apikey", "apikeys":
		projectID := getString(m.Spec, "project_id")
		if projectID == "" {
			projectID = state.opts.projectID
		}
		keys, err := cli.ListAPIKeys(ctx, projectID)
		if err != nil {
			return "", err
		}
		for _, key := range keys {
			if key.Name == name {
				return "update", nil
			}
		}
		return "create", nil
	default:
		return "", fmt.Errorf("unsupported kind %q", m.Kind)
	}
}

func getString(spec map[string]any, key string) string {
	v, ok := spec[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func getSlugOrName(spec map[string]any, fallback string) string {
	v := getString(spec, "slug")
	if v == "" {
		return fallback
	}
	return v
}

func getIntDefault(spec map[string]any, key string, fallback int) int {
	v, ok := spec[key]
	if !ok {
		return fallback
	}
	var out int
	if _, err := fmt.Sscanf(fmt.Sprint(v), "%d", &out); err != nil {
		return fallback
	}
	if out <= 0 {
		return fallback
	}
	return out
}
