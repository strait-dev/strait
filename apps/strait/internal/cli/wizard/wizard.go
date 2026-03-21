// Package wizard provides interactive terminal forms for CLI commands.
// It uses charmbracelet/huh for rich interactive prompts with validation,
// and falls back to flag-based input in non-TTY environments.
package wizard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
)

// InitResult holds the collected values from the init wizard.
type InitResult struct {
	ProjectName string
	Runtime     string
	WithJob     bool
	JobName     string
	JobEndpoint string
	JobCron     string
}

// RunInitWizard runs the interactive project initialization wizard.
// It collects project name, runtime, and optionally a starter job.
func RunInitWizard() (*InitResult, error) {
	result := &InitResult{}

	nameInput := huh.NewInput().
		Title("Project name").
		Placeholder("my-api").
		Value(&result.ProjectName).
		Validate(ValidateProjectName)

	runtimeSelect := huh.NewSelect[string]().
		Title("Runtime").
		Options(
			huh.NewOption("Node.js", "node"),
			huh.NewOption("Bun", "bun"),
			huh.NewOption("Python", "python"),
			huh.NewOption("Go", "go"),
			huh.NewOption("Docker", "docker"),
		).
		Value(&result.Runtime)

	withJobConfirm := huh.NewConfirm().
		Title("Add a starter job?").
		Affirmative("Yes").
		Negative("No").
		Value(&result.WithJob)

	form := huh.NewForm(
		huh.NewGroup(nameInput, runtimeSelect, withJobConfirm),
	)

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("init wizard: %w", err)
	}

	if result.WithJob {
		if err := runJobDetailsForm(result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func runJobDetailsForm(result *InitResult) error {
	jobNameInput := huh.NewInput().
		Title("Job name").
		Placeholder("process-payment").
		Value(&result.JobName).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("job name is required")
			}
			return nil
		})

	jobEndpointInput := huh.NewInput().
		Title("Endpoint URL").
		Placeholder("http://localhost:3000/jobs/process-payment").
		Value(&result.JobEndpoint).
		Validate(ValidateEndpoint)

	jobCronInput := huh.NewInput().
		Title("Schedule (cron, optional)").
		Placeholder("*/5 * * * *").
		Value(&result.JobCron).
		Validate(ValidateCron)

	form := huh.NewForm(
		huh.NewGroup(jobNameInput, jobEndpointInput, jobCronInput),
	)

	return form.Run()
}

// JobResult holds the collected values from the create job wizard.
type JobResult struct {
	Name        string
	Slug        string
	Endpoint    string
	Cron        string
	Timeout     int
	MaxAttempts int
}

// RunCreateJobWizard runs the interactive job creation wizard.
func RunCreateJobWizard() (*JobResult, error) {
	result := &JobResult{
		Timeout:     60,
		MaxAttempts: 3,
	}

	nameInput := huh.NewInput().
		Title("Job name").
		Placeholder("sync-inventory").
		Value(&result.Name).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("job name is required")
			}
			return nil
		})

	endpointInput := huh.NewInput().
		Title("Endpoint URL").
		Placeholder("https://api.example.com/jobs/sync").
		Value(&result.Endpoint).
		Validate(ValidateEndpoint)

	cronInput := huh.NewInput().
		Title("Schedule (cron, optional)").
		Placeholder("@hourly").
		Value(&result.Cron).
		Validate(ValidateCron)

	form := huh.NewForm(
		huh.NewGroup(nameInput, endpointInput, cronInput),
	)

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("create job wizard: %w", err)
	}

	// Auto-generate slug from name
	result.Slug = GenerateSlug(result.Name)

	return result, nil
}

// WorkflowStepInput holds a single step from the workflow wizard.
type WorkflowStepInput struct {
	StepRef   string
	JobSlug   string
	DependsOn string
}

// WorkflowResult holds the collected values from the create workflow wizard.
type WorkflowResult struct {
	Name        string
	Slug        string
	Description string
	Steps       []WorkflowStepInput
}

// RunCreateWorkflowWizard runs the interactive workflow creation wizard.
func RunCreateWorkflowWizard() (*WorkflowResult, error) {
	result := &WorkflowResult{}

	nameInput := huh.NewInput().
		Title("Workflow name").
		Placeholder("data-pipeline").
		Value(&result.Name).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("workflow name is required")
			}
			return nil
		})

	descInput := huh.NewInput().
		Title("Description (optional)").
		Value(&result.Description)

	form := huh.NewForm(
		huh.NewGroup(nameInput, descInput),
	)

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("create workflow wizard: %w", err)
	}

	result.Slug = GenerateSlug(result.Name)

	// Collect steps in a loop
	addMore := true
	for addMore {
		step := WorkflowStepInput{}

		stepRef := huh.NewInput().
			Title("Step ref").
			Placeholder("extract").
			Value(&step.StepRef).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("step ref is required")
				}
				return ValidateSlug(s)
			})

		jobSlug := huh.NewInput().
			Title("Job slug").
			Placeholder("extract-data").
			Value(&step.JobSlug).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("job slug is required")
				}
				return nil
			})

		dependsOn := huh.NewInput().
			Title("Depends on (comma-separated step refs, optional)").
			Value(&step.DependsOn)

		stepForm := huh.NewForm(
			huh.NewGroup(stepRef, jobSlug, dependsOn),
		)

		if err := stepForm.Run(); err != nil {
			return nil, fmt.Errorf("workflow step wizard: %w", err)
		}

		// Validate dependencies reference existing steps
		if step.DependsOn != "" {
			deps := splitAndTrim(step.DependsOn)
			knownRefs := make(map[string]bool, len(result.Steps))
			for _, s := range result.Steps {
				knownRefs[s.StepRef] = true
			}
			for _, dep := range deps {
				if !knownRefs[dep] {
					return nil, fmt.Errorf("step %q depends on unknown step %q", step.StepRef, dep)
				}
			}
		}

		result.Steps = append(result.Steps, step)

		confirmForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Add another step?").
					Affirmative("Yes").
					Negative("No").
					Value(&addMore),
			),
		)
		if err := confirmForm.Run(); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// GenerateSlug converts a name to a URL-safe slug.
func GenerateSlug(name string) string {
	slug := strings.ToLower(name)
	slug = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '_' || r == '-' {
			return '-'
		}
		return -1
	}, slug)
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	return slug
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
