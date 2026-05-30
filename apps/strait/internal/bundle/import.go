package bundle

import (
	"encoding/json"
	"fmt"
	"reflect"

	"strait/internal/domain"
)

// singletonExpr wraps a flat key template back into the stored envelope.
// An empty template yields nil, marking the resource as non-singleton. It is
// the inverse of flattenSingletonKey.
func singletonExpr(template string) json.RawMessage {
	if template == "" {
		return nil
	}
	raw, err := json.Marshal(domain.SingletonKeyExpr{Template: template})
	if err != nil {
		return nil
	}
	return raw
}

// JobSpecToDomain maps a bundle JobSpec onto a domain.Job for the given project.
// environmentID is the resolved ID for spec.EnvironmentSlug (empty when unset).
// It is the inverse of exportJobs and only sets the fields the bundle format
// carries; identity, version, and runtime columns are left to the store.
func JobSpecToDomain(spec JobSpec, projectID, environmentID string) domain.Job {
	return domain.Job{
		ProjectID:                 projectID,
		Slug:                      spec.Slug,
		Name:                      spec.Name,
		Description:               spec.Description,
		EndpointURL:               spec.EndpointURL,
		FallbackEndpointURL:       spec.FallbackEndpointURL,
		MaxAttempts:               spec.MaxAttempts,
		TimeoutSecs:               spec.TimeoutSecs,
		MaxConcurrency:            spec.MaxConcurrency,
		Cron:                      spec.Cron,
		Timezone:                  spec.Timezone,
		PayloadSchema:             spec.PayloadSchema,
		Tags:                      spec.Tags,
		RetryStrategy:             spec.RetryStrategy,
		Enabled:                   spec.Enabled,
		WebhookURL:                spec.WebhookURL,
		EnvironmentID:             environmentID,
		OnCompleteTriggerWorkflow: spec.OnCompleteTriggerWorkflow,
		SingletonKeyExpr:          singletonExpr(spec.SingletonKey),
		SingletonOnConflict:       domain.SingletonOnConflict(spec.SingletonOnConflict),
		SingletonMaxQueueDepth:    spec.SingletonMaxQueueDepth,
	}
}

// WorkflowSpecToDomain maps a bundle WorkflowSpec onto a domain.Workflow.
// Steps are mapped separately via StepSpecToDomain once the workflow ID exists.
func WorkflowSpecToDomain(spec WorkflowSpec, projectID string) domain.Workflow {
	return domain.Workflow{
		ProjectID:              projectID,
		Slug:                   spec.Slug,
		Name:                   spec.Name,
		Description:            spec.Description,
		MaxConcurrentRuns:      spec.MaxConcurrentRuns,
		SingletonKeyExpr:       singletonExpr(spec.SingletonKey),
		SingletonOnConflict:    domain.SingletonOnConflict(spec.SingletonOnConflict),
		SingletonMaxQueueDepth: spec.SingletonMaxQueueDepth,
	}
}

// StepSpecToDomain maps a workflow step spec onto a domain.WorkflowStep.
// jobID is the resolved ID for spec.JobSlug (empty when the step has no job).
func StepSpecToDomain(spec WorkflowStepSpec, workflowID, jobID string) domain.WorkflowStep {
	step := domain.WorkflowStep{
		WorkflowID: workflowID,
		JobID:      jobID,
		StepRef:    spec.StepRef,
		DependsOn:  spec.DependsOn,
		OnFailure:  domain.FailurePolicy(spec.OnFailure),
	}
	if spec.Condition != "" {
		step.Condition = json.RawMessage(spec.Condition)
	}
	if step.DependsOn == nil {
		step.DependsOn = []string{}
	}
	return step
}

// ResolveEnvVariables applies the redacted-secret rule to an environment's
// variables on import. A value equal to RedactedPlaceholder means "keep what is
// already stored": on update it is replaced with the existing value, and on
// create (no existing value) it is an error. Real values pass through
// unchanged. An empty input yields a nil result.
func ResolveEnvVariables(incoming, existing map[string]string, isCreate bool) (map[string]string, error) {
	if len(incoming) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(incoming))
	for k, v := range incoming {
		if v != RedactedPlaceholder {
			out[k] = v
			continue
		}
		if isCreate {
			return nil, fmt.Errorf("environment variable %q is redacted; supply a real value to create it", k)
		}
		ev, ok := existing[k]
		if !ok {
			return nil, fmt.Errorf("environment variable %q is redacted but has no existing value to preserve", k)
		}
		out[k] = ev
	}
	return out, nil
}

// ExistingState holds the current resources of the target project, keyed by
// slug and already in bundle-spec form (produced by exporting the live rows).
// It lets ComputePlan compare an incoming bundle field-by-field.
type ExistingState struct {
	Jobs         map[string]JobSpec
	Workflows    map[string]WorkflowSpec
	Environments map[string]EnvironmentSpec
}

// ExistingStateFromBundle indexes an exported bundle of the live project
// resources by slug for use with ComputePlan.
func ExistingStateFromBundle(b *Bundle) ExistingState {
	st := ExistingState{
		Jobs:         make(map[string]JobSpec, len(b.Resources.Jobs)),
		Workflows:    make(map[string]WorkflowSpec, len(b.Resources.Workflows)),
		Environments: make(map[string]EnvironmentSpec, len(b.Resources.Environments)),
	}
	for _, j := range b.Resources.Jobs {
		st.Jobs[j.Slug] = j
	}
	for _, w := range b.Resources.Workflows {
		st.Workflows[w.Slug] = w
	}
	for _, e := range b.Resources.Environments {
		st.Environments[e.Slug] = e
	}
	return st
}

// ComputePlan compares a bundle against existing project state and returns the
// per-resource action in dependency order (environments, jobs, workflows). A
// resource absent from the state is CREATE; one identical to the existing spec
// is SKIP; anything else is UPDATE. Standard environments are always SKIP.
//
// Comparison is a deep equality of the spec values, so a hand-authored bundle
// that omits fields the export would populate may be reported as UPDATE rather
// than SKIP. That is benign: the apply re-writes the resource and the store
// records a new version, leaving the same end state.
func ComputePlan(b *Bundle, existing ExistingState) []DiffEntry {
	var entries []DiffEntry
	for _, env := range b.Resources.Environments {
		if env.IsStandard {
			entries = append(entries, DiffEntry{
				ResourceType: "environment", Slug: env.Slug, Action: DiffSkip,
				Details: "standard environment (auto-created)",
			})
			continue
		}
		entries = append(entries, planEntry("environment", env.Slug, existing.Environments, env))
	}
	for _, job := range b.Resources.Jobs {
		entries = append(entries, planEntry("job", job.Slug, existing.Jobs, job))
	}
	for _, wf := range b.Resources.Workflows {
		entries = append(entries, planEntry("workflow", wf.Slug, existing.Workflows, wf))
	}
	return entries
}

func planEntry[T any](resourceType, slug string, existing map[string]T, incoming T) DiffEntry {
	cur, ok := existing[slug]
	switch {
	case !ok:
		return DiffEntry{ResourceType: resourceType, Slug: slug, Action: DiffCreate}
	case reflect.DeepEqual(cur, incoming):
		return DiffEntry{ResourceType: resourceType, Slug: slug, Action: DiffSkip, Details: "no changes"}
	default:
		return DiffEntry{ResourceType: resourceType, Slug: slug, Action: DiffUpdate}
	}
}
