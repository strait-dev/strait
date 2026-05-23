package bundle

import (
	"encoding/json"
	"time"

	"strait/internal/domain"
)

// flattenSingletonKey reduces the stored key-expression envelope to its bare
// template string for the bundle format. Empty raw or a parse failure yields an
// empty template, which marks the resource as non-singleton in the export.
func flattenSingletonKey(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	expr, err := domain.ParseSingletonKeyExpr(raw)
	if err != nil {
		return ""
	}
	return expr.Template
}

// ExportBundle creates a Bundle from project resources.
func ExportBundle(
	projectID string,
	jobs []domain.Job,
	workflows []domain.Workflow,
	steps map[string][]domain.WorkflowStep,
	environments []domain.Environment,
	jobIDToSlug map[string]string,
	envIDToSlug map[string]string,
) *Bundle {
	b := &Bundle{
		Version:         Version,
		ExportedAt:      time.Now().UTC(),
		SourceProjectID: projectID,
	}

	b.Resources.Jobs = exportJobs(jobs, envIDToSlug)
	b.Resources.Workflows = exportWorkflows(workflows, steps, jobIDToSlug)
	b.Resources.Environments = exportEnvironments(environments)

	return b
}

func exportJobs(jobs []domain.Job, envIDToSlug map[string]string) []JobSpec {
	specs := make([]JobSpec, 0, len(jobs))
	for _, j := range jobs {
		spec := JobSpec{
			Slug:                      j.Slug,
			Name:                      j.Name,
			Description:               j.Description,
			EndpointURL:               j.EndpointURL,
			FallbackEndpointURL:       j.FallbackEndpointURL,
			MaxAttempts:               j.MaxAttempts,
			TimeoutSecs:               j.TimeoutSecs,
			MaxConcurrency:            j.MaxConcurrency,
			Cron:                      j.Cron,
			Timezone:                  j.Timezone,
			PayloadSchema:             j.PayloadSchema,
			Tags:                      j.Tags,
			RetryStrategy:             j.RetryStrategy,
			Enabled:                   j.Enabled,
			OnCompleteTriggerWorkflow: j.OnCompleteTriggerWorkflow,
			SingletonKey:              flattenSingletonKey(j.SingletonKeyExpr),
			SingletonOnConflict:       string(j.SingletonOnConflict),
			SingletonMaxQueueDepth:    j.SingletonMaxQueueDepth,
		}

		// Redact webhook secrets.
		if j.WebhookURL != "" {
			spec.WebhookURL = j.WebhookURL
		}

		// Resolve environment ID to slug.
		if j.EnvironmentID != "" {
			if slug, ok := envIDToSlug[j.EnvironmentID]; ok {
				spec.EnvironmentSlug = slug
			}
		}

		specs = append(specs, spec)
	}
	return specs
}

func exportWorkflows(workflows []domain.Workflow, steps map[string][]domain.WorkflowStep, jobIDToSlug map[string]string) []WorkflowSpec {
	specs := make([]WorkflowSpec, 0, len(workflows))
	for _, w := range workflows {
		spec := WorkflowSpec{
			Slug:                   w.Slug,
			Name:                   w.Name,
			Description:            w.Description,
			MaxConcurrentRuns:      w.MaxConcurrentRuns,
			SingletonKey:           flattenSingletonKey(w.SingletonKeyExpr),
			SingletonOnConflict:    string(w.SingletonOnConflict),
			SingletonMaxQueueDepth: w.SingletonMaxQueueDepth,
		}

		// Export steps with job slugs instead of IDs.
		if wfSteps, ok := steps[w.ID]; ok {
			for _, s := range wfSteps {
				stepSpec := WorkflowStepSpec{
					StepRef:   s.StepRef,
					DependsOn: s.DependsOn,
					Condition: string(s.Condition),
					OnFailure: string(s.OnFailure),
				}
				if s.JobID != "" {
					if slug, ok := jobIDToSlug[s.JobID]; ok {
						stepSpec.JobSlug = slug
					}
				}
				spec.Steps = append(spec.Steps, stepSpec)
			}
		}

		specs = append(specs, spec)
	}
	return specs
}

func exportEnvironments(environments []domain.Environment) []EnvironmentSpec {
	specs := make([]EnvironmentSpec, 0, len(environments))
	for _, e := range environments {
		spec := EnvironmentSpec{
			Name:       e.Name,
			Slug:       e.Slug,
			IsStandard: e.IsStandard,
		}

		// Redact sensitive variable values.
		if len(e.Variables) > 0 {
			redacted := make(map[string]string, len(e.Variables))
			for k, v := range e.Variables {
				_ = v
				redacted[k] = RedactedPlaceholder
			}
			spec.Variables = redacted
		}

		specs = append(specs, spec)
	}
	return specs
}
