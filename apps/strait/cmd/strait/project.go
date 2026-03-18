package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"strait/internal/bundle"
	"strait/internal/cli/client"
	"strait/internal/domain"

	"github.com/spf13/cobra"
)

func newProjectCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage project bundles for GitOps workflows",
	}

	cmd.AddCommand(newProjectExportCommand(state))
	cmd.AddCommand(newProjectImportCommand(state))

	return cmd
}

func newProjectExportCommand(state *appState) *cobra.Command {
	var projectID string
	var outputFile string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a project as a GitOps YAML bundle",
		Long:  "Exports all jobs, workflows, environments, and webhook subscriptions from a project into a single YAML bundle file.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			ctx := cmd.Context()

			jobs, err := cli.ListJobs(ctx, projectID)
			if err != nil {
				return fmt.Errorf("listing jobs: %w", err)
			}

			workflows, err := cli.ListWorkflows(ctx, projectID)
			if err != nil {
				return fmt.Errorf("listing workflows: %w", err)
			}

			environments, err := cli.ListEnvironments(ctx, projectID)
			if err != nil {
				return fmt.Errorf("listing environments: %w", err)
			}

			// Build lookup maps for ID-to-slug resolution.
			jobIDToSlug := make(map[string]string, len(jobs))
			for _, j := range jobs {
				jobIDToSlug[j.ID] = j.Slug
			}
			envIDToSlug := make(map[string]string, len(environments))
			for _, e := range environments {
				envIDToSlug[e.ID] = e.Slug
			}

			// Fetch workflow steps.
			stepsMap := make(map[string][]domain.WorkflowStep, len(workflows))
			for _, wf := range workflows {
				detail, detailErr := cli.GetWorkflow(ctx, wf.ID)
				if detailErr != nil {
					return fmt.Errorf("fetching workflow %s: %w", wf.ID, detailErr)
				}
				stepsMap[wf.ID] = detail.Steps
			}

			b := bundle.ExportBundle(projectID, jobs, workflows, stepsMap, environments, jobIDToSlug, envIDToSlug)

			data, err := bundle.MarshalYAML(b)
			if err != nil {
				return fmt.Errorf("marshaling bundle: %w", err)
			}

			if outputFile == "" {
				_, err = os.Stdout.Write(data)
				return err
			}

			if err := os.WriteFile(outputFile, data, 0o600); err != nil {
				return fmt.Errorf("writing bundle file: %w", err)
			}

			return printData(state, map[string]any{
				"project_id":   projectID,
				"output":       outputFile,
				"jobs":         len(jobs),
				"workflows":    len(workflows),
				"environments": len(environments),
			})
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVarP(&outputFile, "file", "f", "", "output file path (default: stdout)")

	return cmd
}

func newProjectImportCommand(state *appState) *cobra.Command {
	var projectID string
	var inputFile string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import a GitOps YAML bundle into a project",
		Long:  "Reads a YAML bundle file and creates or updates resources in the target project. Use --dry-run to preview changes without applying them.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			if inputFile == "" {
				return fmt.Errorf("--file is required")
			}

			data, err := os.ReadFile(inputFile) //nolint:gosec // CLI reads user-specified file path
			if err != nil {
				return fmt.Errorf("reading bundle file: %w", err)
			}

			b, err := bundle.UnmarshalYAML(data)
			if err != nil {
				return fmt.Errorf("parsing bundle: %w", err)
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			ctx := cmd.Context()

			// Build existing resource maps for diff computation.
			existingJobs, err := cli.ListJobs(ctx, projectID)
			if err != nil {
				return fmt.Errorf("listing existing jobs: %w", err)
			}
			existingJobSlugs := make(map[string]bool, len(existingJobs))
			existingJobsBySlug := make(map[string]domain.Job, len(existingJobs))
			for _, j := range existingJobs {
				existingJobSlugs[j.Slug] = true
				existingJobsBySlug[j.Slug] = j
			}

			existingWorkflows, err := cli.ListWorkflows(ctx, projectID)
			if err != nil {
				return fmt.Errorf("listing existing workflows: %w", err)
			}
			existingWFSlugs := make(map[string]bool, len(existingWorkflows))
			existingWFsBySlug := make(map[string]domain.Workflow, len(existingWorkflows))
			for _, wf := range existingWorkflows {
				existingWFSlugs[wf.Slug] = true
				existingWFsBySlug[wf.Slug] = wf
			}

			existingEnvs, err := cli.ListEnvironments(ctx, projectID)
			if err != nil {
				return fmt.Errorf("listing existing environments: %w", err)
			}
			existingEnvSlugs := make(map[string]bool, len(existingEnvs))
			for _, e := range existingEnvs {
				existingEnvSlugs[e.Slug] = true
			}

			diff := bundle.ComputeDiff(b, existingJobSlugs, existingWFSlugs, existingEnvSlugs)

			if dryRun {
				return printData(state, map[string]any{
					"dry_run": true,
					"diff":    diff,
				})
			}

			result := bundle.ImportResult{}
			for _, entry := range diff {
				switch entry.Action {
				case bundle.DiffSkip:
					result.Skipped++
				case bundle.DiffCreate:
					if applyErr := applyCreate(ctx, cli, projectID, entry, b, existingJobsBySlug); applyErr != nil {
						result.Failed++
						result.Errors = append(result.Errors, fmt.Sprintf("%s/%s: %v", entry.ResourceType, entry.Slug, applyErr))
					} else {
						result.Created++
					}
				case bundle.DiffUpdate:
					if applyErr := applyUpdate(ctx, cli, entry, b, existingJobsBySlug, existingWFsBySlug); applyErr != nil {
						result.Failed++
						result.Errors = append(result.Errors, fmt.Sprintf("%s/%s: %v", entry.ResourceType, entry.Slug, applyErr))
					} else {
						result.Updated++
					}
				}
			}

			return printData(state, result)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVarP(&inputFile, "file", "f", "", "bundle YAML file path (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview changes without applying")

	return cmd
}

func applyCreate(ctx context.Context, cli *client.Client, projectID string, entry bundle.DiffEntry, b *bundle.Bundle, existingJobs map[string]domain.Job) error {
	switch entry.ResourceType {
	case "job":
		spec := findJobSpec(b, entry.Slug)
		if spec == nil {
			return fmt.Errorf("job spec not found in bundle")
		}
		_, err := cli.CreateJob(ctx, client.CreateJobRequest{
			ProjectID:   projectID,
			Name:        spec.Name,
			Slug:        spec.Slug,
			Description: spec.Description,
			EndpointURL: spec.EndpointURL,
			MaxAttempts: spec.MaxAttempts,
			TimeoutSecs: spec.TimeoutSecs,
			Cron:        spec.Cron,
		})
		return err

	case "workflow":
		spec := findWorkflowSpec(b, entry.Slug)
		if spec == nil {
			return fmt.Errorf("workflow spec not found in bundle")
		}
		steps := buildWorkflowSteps(spec.Steps, existingJobs)
		_, err := cli.CreateWorkflow(ctx, client.CreateWorkflowRequest{
			ProjectID:   projectID,
			Name:        spec.Name,
			Slug:        spec.Slug,
			Description: spec.Description,
			Steps:       steps,
		})
		return err

	default:
		return nil
	}
}

func applyUpdate(ctx context.Context, cli *client.Client, entry bundle.DiffEntry, b *bundle.Bundle, existingJobs map[string]domain.Job, existingWFs map[string]domain.Workflow) error {
	switch entry.ResourceType {
	case "job":
		spec := findJobSpec(b, entry.Slug)
		if spec == nil {
			return fmt.Errorf("job spec not found in bundle")
		}
		existing, ok := existingJobs[entry.Slug]
		if !ok {
			return fmt.Errorf("existing job not found for update")
		}
		_, err := cli.UpdateJob(ctx, existing.ID, client.UpdateJobRequest{
			Name:        &spec.Name,
			Description: &spec.Description,
			EndpointURL: &spec.EndpointURL,
			MaxAttempts: &spec.MaxAttempts,
			TimeoutSecs: &spec.TimeoutSecs,
			Cron:        &spec.Cron,
			Enabled:     &spec.Enabled,
		})
		return err

	case "workflow":
		spec := findWorkflowSpec(b, entry.Slug)
		if spec == nil {
			return fmt.Errorf("workflow spec not found in bundle")
		}
		existing, ok := existingWFs[entry.Slug]
		if !ok {
			return fmt.Errorf("existing workflow not found for update")
		}
		steps := buildWorkflowSteps(spec.Steps, existingJobs)
		_, err := cli.UpdateWorkflow(ctx, existing.ID, client.UpdateWorkflowRequest{
			Name:        &spec.Name,
			Description: &spec.Description,
			Steps:       &steps,
		})
		return err

	default:
		return nil
	}
}

func buildWorkflowSteps(specSteps []bundle.WorkflowStepSpec, existingJobs map[string]domain.Job) []client.WorkflowStepRequest {
	steps := make([]client.WorkflowStepRequest, 0, len(specSteps))
	for _, s := range specSteps {
		step := client.WorkflowStepRequest{
			StepRef:   s.StepRef,
			DependsOn: s.DependsOn,
			OnFailure: s.OnFailure,
		}
		if s.Condition != "" {
			step.Condition = json.RawMessage(s.Condition)
		}
		if s.JobSlug != "" {
			if j, ok := existingJobs[s.JobSlug]; ok {
				step.JobID = j.ID
			}
		}
		steps = append(steps, step)
	}
	return steps
}

func findJobSpec(b *bundle.Bundle, slug string) *bundle.JobSpec {
	for i := range b.Resources.Jobs {
		if b.Resources.Jobs[i].Slug == slug {
			return &b.Resources.Jobs[i]
		}
	}
	return nil
}

func findWorkflowSpec(b *bundle.Bundle, slug string) *bundle.WorkflowSpec {
	for i := range b.Resources.Workflows {
		if b.Resources.Workflows[i].Slug == slug {
			return &b.Resources.Workflows[i]
		}
	}
	return nil
}
