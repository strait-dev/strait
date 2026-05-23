package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/bundle"
	"strait/internal/domain"
)

// bundleListLimit bounds the per-resource reads that build the diff and resolve
// slugs during import. A project's job/workflow/environment counts sit well
// below this, so a single page is always the full set.
const bundleListLimit = 10000

// ImportBundleInput is the typed input for the config-as-code import endpoint.
// dry_run computes the diff without writing anything.
type ImportBundleInput struct {
	ProjectID string `path:"projectID"`
	DryRun    bool   `query:"dry_run" doc:"Compute and return the diff without applying any changes"`
	Body      bundle.Bundle
}

// ImportBundleOutput wraps the import result (diff plus per-action counts).
type ImportBundleOutput struct {
	Body bundle.ImportResult
}

// handleImportBundle applies a configuration bundle to a project. On dry_run it
// returns the per-resource diff (CREATE/UPDATE/SKIP) without writing. Otherwise
// it applies environments, jobs, then workflows in a single transaction:
// any failure rolls the whole import back, so the project is never left in a
// partially-applied state.
func (s *Server) handleImportBundle(ctx context.Context, input *ImportBundleInput) (*ImportBundleOutput, error) {
	projectID := input.ProjectID
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if err := requireProjectMatch(ctx, projectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}

	b := &input.Body

	// Validate singleton config and run plan gates up front so a malformed or
	// plan-gated bundle fails fast, before any write transaction is opened.
	if err := s.validateBundleSingletons(ctx, projectID, b); err != nil {
		return nil, err
	}

	existing, err := s.exportExistingState(ctx, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to read existing project state")
	}
	plan := bundle.ComputePlan(b, existing)

	if input.DryRun {
		return &ImportBundleOutput{Body: bundle.ImportResult{
			Created: countAction(plan, bundle.DiffCreate),
			Updated: countAction(plan, bundle.DiffUpdate),
			Skipped: countAction(plan, bundle.DiffSkip),
			Diff:    plan,
		}}, nil
	}

	result, err := s.applyBundle(ctx, projectID, b, plan)
	if err != nil {
		return nil, err
	}
	return &ImportBundleOutput{Body: result}, nil
}

// validateBundleSingletons runs the same singleton validation and plan gating
// that the job and workflow create/update handlers apply, across every resource
// in the bundle. Reusing JobSpecToDomain/WorkflowSpecToDomain gives us the
// stored key-expression envelope without duplicating the flatten/wrap logic.
func (s *Server) validateBundleSingletons(ctx context.Context, projectID string, b *bundle.Bundle) error {
	for _, spec := range b.Resources.Jobs {
		mapped := bundle.JobSpecToDomain(spec, projectID, "")
		if err := validateSingletonConfig(mapped.SingletonKeyExpr, spec.SingletonOnConflict, spec.SingletonMaxQueueDepth); err != nil {
			return err
		}
		if err := s.checkSingletonOnConflict(ctx, projectID, spec.SingletonOnConflict); err != nil {
			return err
		}
	}
	for _, spec := range b.Resources.Workflows {
		mapped := bundle.WorkflowSpecToDomain(spec, projectID)
		if err := validateSingletonConfig(mapped.SingletonKeyExpr, spec.SingletonOnConflict, spec.SingletonMaxQueueDepth); err != nil {
			return err
		}
		if err := s.checkSingletonOnConflict(ctx, projectID, spec.SingletonOnConflict); err != nil {
			return err
		}
	}
	return nil
}

// exportExistingState reads the project's live resources and runs them through
// the exporter so the incoming bundle can be diffed field-by-field against the
// same spec shape it was authored in.
func (s *Server) exportExistingState(ctx context.Context, projectID string) (bundle.ExistingState, error) {
	jobs, err := s.store.ListJobs(ctx, projectID, bundleListLimit, nil)
	if err != nil {
		return bundle.ExistingState{}, fmt.Errorf("list jobs: %w", err)
	}
	workflows, err := s.store.ListWorkflows(ctx, projectID, bundleListLimit, nil)
	if err != nil {
		return bundle.ExistingState{}, fmt.Errorf("list workflows: %w", err)
	}
	environments, err := s.store.ListEnvironments(ctx, projectID, bundleListLimit, nil)
	if err != nil {
		return bundle.ExistingState{}, fmt.Errorf("list environments: %w", err)
	}

	jobIDToSlug := make(map[string]string, len(jobs))
	for _, j := range jobs {
		jobIDToSlug[j.ID] = j.Slug
	}
	envIDToSlug := make(map[string]string, len(environments))
	for _, e := range environments {
		envIDToSlug[e.ID] = e.Slug
	}
	steps := make(map[string][]domain.WorkflowStep, len(workflows))
	for _, w := range workflows {
		st, stepErr := s.store.ListStepsByWorkflow(ctx, w.ID)
		if stepErr != nil {
			return bundle.ExistingState{}, fmt.Errorf("list steps for workflow %s: %w", w.ID, stepErr)
		}
		steps[w.ID] = st
	}

	exported := bundle.ExportBundle(projectID, jobs, workflows, steps, environments, jobIDToSlug, envIDToSlug)
	return bundle.ExistingStateFromBundle(exported), nil
}

// applyBundle performs the atomic import. It threads slug-to-ID maps from
// environments to jobs to workflows so later resources can resolve the IDs of
// resources created earlier in the same transaction.
func (s *Server) applyBundle(ctx context.Context, projectID string, b *bundle.Bundle, plan []bundle.DiffEntry) (bundle.ImportResult, error) {
	planByKey := make(map[string]bundle.DiffAction, len(plan))
	for _, e := range plan {
		planByKey[e.ResourceType+"/"+e.Slug] = e.Action
	}

	result := bundle.ImportResult{Diff: plan}
	err := s.runInTx(ctx, func(tx APIStore) error {
		envSlugToID, err := s.applyBundleEnvironments(ctx, tx, projectID, b, planByKey, &result)
		if err != nil {
			return err
		}
		jobSlugToID, err := s.applyBundleJobs(ctx, tx, projectID, b, planByKey, envSlugToID, &result)
		if err != nil {
			return err
		}
		return s.applyBundleWorkflows(ctx, tx, projectID, b, planByKey, jobSlugToID, &result)
	})
	if err != nil {
		var statusErr huma.StatusError
		if errors.As(err, &statusErr) {
			return bundle.ImportResult{}, err
		}
		slog.Error("failed to apply config bundle", "project_id", projectID, "error", err)
		return bundle.ImportResult{}, huma.Error500InternalServerError("failed to apply bundle")
	}

	s.emitAuditEvent(ctx, domain.AuditActionBundleImported, "project", projectID, map[string]any{
		"created": result.Created,
		"updated": result.Updated,
		"skipped": result.Skipped,
	})
	return result, nil
}

// applyBundleEnvironments creates or updates the bundle's environments and
// returns a slug-to-ID map covering every environment in the project (existing
// and newly created) so jobs can resolve their environment_slug references.
func (s *Server) applyBundleEnvironments(
	ctx context.Context, tx APIStore, projectID string,
	b *bundle.Bundle, planByKey map[string]bundle.DiffAction, result *bundle.ImportResult,
) (map[string]string, error) {
	existing, err := tx.ListEnvironments(ctx, projectID, bundleListLimit, nil)
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	bySlug := make(map[string]*domain.Environment, len(existing))
	slugToID := make(map[string]string, len(existing))
	for i := range existing {
		bySlug[existing[i].Slug] = &existing[i]
		slugToID[existing[i].Slug] = existing[i].ID
	}

	for _, spec := range b.Resources.Environments {
		// Standard environments are auto-provisioned; never created or mutated
		// by import, but their IDs still feed job environment resolution.
		if spec.IsStandard {
			result.Skipped++
			continue
		}
		switch planByKey["environment/"+spec.Slug] {
		case bundle.DiffSkip:
			result.Skipped++
		case bundle.DiffCreate:
			vars, verr := bundle.ResolveEnvVariables(spec.Variables, nil, true)
			if verr != nil {
				return nil, huma.Error400BadRequest(verr.Error())
			}
			parentID, perr := resolveParentEnv(spec, slugToID)
			if perr != nil {
				return nil, perr
			}
			env := &domain.Environment{
				ProjectID: projectID, Name: spec.Name, Slug: spec.Slug,
				ParentID: parentID, Variables: vars,
			}
			if err := tx.CreateEnvironment(ctx, env); err != nil {
				return nil, fmt.Errorf("create environment %q: %w", spec.Slug, err)
			}
			slugToID[spec.Slug] = env.ID
			result.Created++
		case bundle.DiffUpdate:
			cur := bySlug[spec.Slug]
			realVars, gerr := tx.GetResolvedEnvironmentVariables(ctx, cur.ID)
			if gerr != nil {
				return nil, fmt.Errorf("resolve variables for environment %q: %w", spec.Slug, gerr)
			}
			vars, verr := bundle.ResolveEnvVariables(spec.Variables, realVars, false)
			if verr != nil {
				return nil, huma.Error400BadRequest(verr.Error())
			}
			parentID, perr := resolveParentEnv(spec, slugToID)
			if perr != nil {
				return nil, perr
			}
			cur.Name = spec.Name
			cur.Variables = vars
			cur.ParentID = parentID
			if err := tx.UpdateEnvironment(ctx, cur); err != nil {
				return nil, fmt.Errorf("update environment %q: %w", spec.Slug, err)
			}
			result.Updated++
		}
	}
	return slugToID, nil
}

// resolveParentEnv maps an environment's parent_slug to a stored ID. An unset
// parent is allowed; a parent that is not already present (existing or created
// earlier in this import) is a 400 so callers get a clear ordering error.
func resolveParentEnv(spec bundle.EnvironmentSpec, slugToID map[string]string) (string, error) {
	if spec.ParentSlug == "" {
		return "", nil
	}
	id, ok := slugToID[spec.ParentSlug]
	if !ok {
		return "", huma.Error400BadRequest(
			fmt.Sprintf("environment %q references unknown parent_slug %q", spec.Slug, spec.ParentSlug))
	}
	return id, nil
}

// applyBundleJobs creates or updates the bundle's jobs and returns a slug-to-ID
// map (existing and newly created) for workflow step resolution.
func (s *Server) applyBundleJobs(
	ctx context.Context, tx APIStore, projectID string,
	b *bundle.Bundle, planByKey map[string]bundle.DiffAction, envSlugToID map[string]string, result *bundle.ImportResult,
) (map[string]string, error) {
	existing, err := tx.ListJobs(ctx, projectID, bundleListLimit, nil)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	bySlug := make(map[string]*domain.Job, len(existing))
	slugToID := make(map[string]string, len(existing))
	for i := range existing {
		bySlug[existing[i].Slug] = &existing[i]
		slugToID[existing[i].Slug] = existing[i].ID
	}

	actor := actorFromContext(ctx)
	for _, spec := range b.Resources.Jobs {
		envID, eerr := resolveJobEnvironment(spec, envSlugToID)
		if eerr != nil {
			return nil, eerr
		}
		switch planByKey["job/"+spec.Slug] {
		case bundle.DiffSkip:
			result.Skipped++
		case bundle.DiffCreate:
			job := bundle.JobSpecToDomain(spec, projectID, envID)
			job.VersionPolicy = domain.VersionPolicyPin
			job.CreatedBy = actor
			job.UpdatedBy = actor
			if err := tx.CreateJob(ctx, &job); err != nil {
				return nil, fmt.Errorf("create job %q: %w", spec.Slug, err)
			}
			slugToID[spec.Slug] = job.ID
			result.Created++
		case bundle.DiffUpdate:
			cur := bySlug[spec.Slug]
			applyJobSpecOnto(cur, spec, projectID, envID)
			cur.UpdatedBy = actor
			if err := tx.UpdateJob(ctx, cur); err != nil {
				return nil, fmt.Errorf("update job %q: %w", spec.Slug, err)
			}
			result.Updated++
		}
	}
	return slugToID, nil
}

// resolveJobEnvironment maps a job spec's environment_slug to a stored ID.
func resolveJobEnvironment(spec bundle.JobSpec, envSlugToID map[string]string) (string, error) {
	if spec.EnvironmentSlug == "" {
		return "", nil
	}
	id, ok := envSlugToID[spec.EnvironmentSlug]
	if !ok {
		return "", huma.Error400BadRequest(
			fmt.Sprintf("job %q references unknown environment_slug %q", spec.Slug, spec.EnvironmentSlug))
	}
	return id, nil
}

// applyJobSpecOnto overlays the bundle-carried fields onto an existing job,
// leaving every column the bundle format does not model (signing secrets, rate
// limits, tool budgets, and so on) untouched so an update never silently
// clears them. The bundle-owned set is taken from JobSpecToDomain to keep the
// mapping in one place.
func applyJobSpecOnto(job *domain.Job, spec bundle.JobSpec, projectID, envID string) {
	mapped := bundle.JobSpecToDomain(spec, projectID, envID)
	job.Name = mapped.Name
	job.Description = mapped.Description
	job.EndpointURL = mapped.EndpointURL
	job.FallbackEndpointURL = mapped.FallbackEndpointURL
	job.MaxAttempts = mapped.MaxAttempts
	job.TimeoutSecs = mapped.TimeoutSecs
	job.MaxConcurrency = mapped.MaxConcurrency
	job.Cron = mapped.Cron
	job.Timezone = mapped.Timezone
	job.PayloadSchema = mapped.PayloadSchema
	job.Tags = mapped.Tags
	job.RetryStrategy = mapped.RetryStrategy
	job.Enabled = mapped.Enabled
	job.WebhookURL = mapped.WebhookURL
	job.EnvironmentID = mapped.EnvironmentID
	job.OnCompleteTriggerWorkflow = mapped.OnCompleteTriggerWorkflow
	job.SingletonKeyExpr = mapped.SingletonKeyExpr
	job.SingletonOnConflict = mapped.SingletonOnConflict
	job.SingletonMaxQueueDepth = mapped.SingletonMaxQueueDepth
}

// applyBundleWorkflows creates or updates workflows and reconciles their steps.
// On update the steps are fully replaced (delete then recreate) so the bundle
// is the declarative source of truth for the step set.
func (s *Server) applyBundleWorkflows(
	ctx context.Context, tx APIStore, projectID string,
	b *bundle.Bundle, planByKey map[string]bundle.DiffAction, jobSlugToID map[string]string, result *bundle.ImportResult,
) error {
	existing, err := tx.ListWorkflows(ctx, projectID, bundleListLimit, nil)
	if err != nil {
		return fmt.Errorf("list workflows: %w", err)
	}
	bySlug := make(map[string]*domain.Workflow, len(existing))
	for i := range existing {
		bySlug[existing[i].Slug] = &existing[i]
	}

	actor := actorFromContext(ctx)
	for _, spec := range b.Resources.Workflows {
		switch planByKey["workflow/"+spec.Slug] {
		case bundle.DiffSkip:
			result.Skipped++
		case bundle.DiffCreate:
			wf := bundle.WorkflowSpecToDomain(spec, projectID)
			wf.Enabled = true
			wf.VersionPolicy = domain.VersionPolicyPin
			wf.CreatedBy = actor
			wf.UpdatedBy = actor
			if err := tx.CreateWorkflow(ctx, &wf); err != nil {
				return fmt.Errorf("create workflow %q: %w", spec.Slug, err)
			}
			if err := s.applyBundleSteps(ctx, tx, wf.ID, spec, jobSlugToID); err != nil {
				return err
			}
			if err := tx.CreateWorkflowVersionSnapshot(ctx, wf.ID, wf.Version); err != nil {
				return fmt.Errorf("snapshot workflow %q: %w", spec.Slug, err)
			}
			result.Created++
		case bundle.DiffUpdate:
			cur := bySlug[spec.Slug]
			applyWorkflowSpecOnto(cur, spec)
			cur.UpdatedBy = actor
			if err := tx.UpdateWorkflow(ctx, cur); err != nil {
				return fmt.Errorf("update workflow %q: %w", spec.Slug, err)
			}
			if err := tx.DeleteStepsByWorkflow(ctx, cur.ID); err != nil {
				return fmt.Errorf("reconcile steps for workflow %q: %w", spec.Slug, err)
			}
			if err := s.applyBundleSteps(ctx, tx, cur.ID, spec, jobSlugToID); err != nil {
				return err
			}
			result.Updated++
		}
	}
	return nil
}

// applyWorkflowSpecOnto overlays the bundle-carried fields onto an existing
// workflow. Enabled and other non-bundle columns are left untouched.
func applyWorkflowSpecOnto(wf *domain.Workflow, spec bundle.WorkflowSpec) {
	mapped := bundle.WorkflowSpecToDomain(spec, wf.ProjectID)
	wf.Name = mapped.Name
	wf.Description = mapped.Description
	wf.MaxConcurrentRuns = mapped.MaxConcurrentRuns
	wf.SingletonKeyExpr = mapped.SingletonKeyExpr
	wf.SingletonOnConflict = mapped.SingletonOnConflict
	wf.SingletonMaxQueueDepth = mapped.SingletonMaxQueueDepth
}

// applyBundleSteps inserts a workflow's steps, resolving each step's job_slug to
// a job ID created or already present in this import.
func (s *Server) applyBundleSteps(ctx context.Context, tx APIStore, workflowID string, spec bundle.WorkflowSpec, jobSlugToID map[string]string) error {
	for _, ss := range spec.Steps {
		jobID := ""
		if ss.JobSlug != "" {
			id, ok := jobSlugToID[ss.JobSlug]
			if !ok {
				return huma.Error400BadRequest(
					fmt.Sprintf("workflow %q step %q references unknown job_slug %q", spec.Slug, ss.StepRef, ss.JobSlug))
			}
			jobID = id
		}
		step := bundle.StepSpecToDomain(ss, workflowID, jobID)
		if err := tx.CreateWorkflowStep(ctx, &step); err != nil {
			return fmt.Errorf("create workflow %q step %q: %w", spec.Slug, ss.StepRef, err)
		}
	}
	return nil
}

// countAction returns the number of plan entries with the given action.
func countAction(plan []bundle.DiffEntry, action bundle.DiffAction) int {
	n := 0
	for _, e := range plan {
		if e.Action == action {
			n++
		}
	}
	return n
}
