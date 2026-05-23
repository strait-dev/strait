package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/workflow"

	"github.com/danielgtaylor/huma/v2"
	"github.com/robfig/cron/v3"
	"github.com/samber/lo"
)

type workflowStepRequest struct {
	JobID                   string                    `json:"job_id,omitempty"`
	StepRef                 string                    `json:"step_ref" validate:"required"`
	DependsOn               []string                  `json:"depends_on,omitempty"`
	Condition               json.RawMessage           `json:"condition,omitempty"`
	OnFailure               domain.FailurePolicy      `json:"on_failure,omitempty"`
	Payload                 json.RawMessage           `json:"payload,omitempty"`
	StepType                domain.WorkflowStepType   `json:"step_type,omitempty"`
	ApprovalTimeoutSecs     int                       `json:"approval_timeout_secs,omitempty"`
	ApprovalApprovers       []string                  `json:"approval_approvers,omitempty"`
	RetryMaxAttempts        int                       `json:"retry_max_attempts,omitempty"`
	RetryBackoff            domain.RetryBackoffPolicy `json:"retry_backoff,omitempty"`
	RetryInitialDelaySecs   int                       `json:"retry_initial_delay_secs,omitempty"`
	RetryMaxDelaySecs       int                       `json:"retry_max_delay_secs,omitempty"`
	TimeoutSecsOverride     int                       `json:"timeout_secs_override,omitempty"`
	OutputTransform         string                    `json:"output_transform,omitempty"`
	SubWorkflowID           string                    `json:"sub_workflow_id,omitempty"`
	MaxNestingDepth         int                       `json:"max_nesting_depth,omitempty"`
	EventKey                string                    `json:"event_key,omitempty"`
	EventTimeoutSecs        int                       `json:"event_timeout_secs,omitempty"`
	EventNotifyURL          string                    `json:"event_notify_url,omitempty"`
	EventEmitKey            string                    `json:"event_emit_key,omitempty"`
	SleepDurationSecs       int                       `json:"sleep_duration_secs,omitempty"`
	ConcurrencyKey          string                    `json:"concurrency_key,omitempty"`
	ResourceClass           string                    `json:"resource_class,omitempty"`
	ExpectedDurationSecs    int                       `json:"expected_duration_secs,omitempty"`
	StageNotifications      json.RawMessage           `json:"stage_notifications,omitempty"`
	CompensationJobID       string                    `json:"compensation_job_id,omitempty"`
	CompensationTimeoutSecs int                       `json:"compensation_timeout_secs,omitempty"`
}

type createWorkflowRequest struct {
	ProjectID         string                `json:"project_id" validate:"required"`
	Name              string                `json:"name" validate:"required,max=255"`
	Slug              string                `json:"slug" validate:"required,max=255"`
	Description       string                `json:"description,omitempty" validate:"max=2000"`
	Tags              map[string]string     `json:"tags,omitempty"`
	Enabled           *bool                 `json:"enabled,omitempty"`
	TimeoutSecs       int                   `json:"timeout_secs,omitempty"`
	MaxConcurrentRuns int                   `json:"max_concurrent_runs,omitempty"`
	MaxParallelSteps  int                   `json:"max_parallel_steps,omitempty"`
	Cron              string                `json:"cron,omitempty"`
	CronTimezone      string                `json:"cron_timezone,omitempty"`
	SkipIfRunning     bool                  `json:"skip_if_running,omitempty"`
	VersionPolicy     string                `json:"version_policy,omitempty" validate:"omitempty,oneof=pin latest minor"`
	Steps             []workflowStepRequest `json:"steps,omitempty"`

	SingletonKeyExpr       json.RawMessage `json:"singleton_key_expr,omitempty" doc:"Singleton key expression envelope ({\"template\":\"...\"}). When set, the workflow runs as a singleton keyed by the resolved template."`
	SingletonOnConflict    string          `json:"singleton_on_conflict,omitempty" validate:"omitempty,oneof=queue drop replace" doc:"Collision policy when the singleton key is already held: queue, drop, or replace."`
	SingletonMaxQueueDepth *int            `json:"singleton_max_queue_depth,omitempty" validate:"omitempty,min=0" doc:"Optional per-key cap on queued-behind runs for the queue policy. NULL means unbounded."`
}

type updateWorkflowRequest struct {
	Name                *string                `json:"name,omitempty"`
	Slug                *string                `json:"slug,omitempty"`
	Description         *string                `json:"description,omitempty"`
	Tags                *map[string]string     `json:"tags,omitempty"`
	Enabled             *bool                  `json:"enabled,omitempty"`
	TimeoutSecs         *int                   `json:"timeout_secs,omitempty"`
	MaxConcurrentRuns   *int                   `json:"max_concurrent_runs,omitempty"`
	MaxParallelSteps    *int                   `json:"max_parallel_steps,omitempty"`
	Cron                *string                `json:"cron,omitempty"`
	CronTimezone        *string                `json:"cron_timezone,omitempty"`
	SkipIfRunning       *bool                  `json:"skip_if_running,omitempty"`
	VersionPolicy       *string                `json:"version_policy,omitempty" validate:"omitempty,oneof=pin latest minor"`
	BackwardsCompatible *bool                  `json:"backwards_compatible,omitempty"`
	Steps               *[]workflowStepRequest `json:"steps,omitempty"`
	BreakingChange      *bool                  `json:"breaking_change,omitempty"`

	SingletonKeyExpr       *json.RawMessage `json:"singleton_key_expr,omitempty" doc:"Singleton key expression envelope ({\"template\":\"...\"}). When set, the workflow runs as a singleton keyed by the resolved template."`
	SingletonOnConflict    *string          `json:"singleton_on_conflict,omitempty" validate:"omitempty,oneof=queue drop replace" doc:"Collision policy when the singleton key is already held: queue, drop, or replace."`
	SingletonMaxQueueDepth *int             `json:"singleton_max_queue_depth,omitempty" validate:"omitempty,min=0" doc:"Optional per-key cap on queued-behind runs for the queue policy. NULL means unbounded."`
}

type dryRunWorkflowRequest struct {
	Steps []workflowStepRequest `json:"steps" validate:"required"`
}

type workflowGraphResponse struct {
	WorkflowID string              `json:"workflow_id"`
	Roots      []string            `json:"roots"`
	Adjacency  map[string][]string `json:"adjacency,omitempty"`
	DOT        string              `json:"dot,omitempty"`
}

type triggerWorkflowRequest struct {
	ProjectID     string                `json:"project_id,omitempty"`
	Payload       json.RawMessage       `json:"payload,omitempty"`
	TriggeredBy   string                `json:"triggered_by,omitempty"`
	Labels        map[string]string     `json:"labels,omitempty"`
	Tags          map[string]string     `json:"tags,omitempty"`
	StepOverrides []domain.StepOverride `json:"step_overrides,omitempty"`
}

type workflowResponse struct {
	*domain.Workflow
	Steps []domain.WorkflowStep `json:"steps"`
}

type CreateWorkflowInput struct {
	Body createWorkflowRequest
}
type CreateWorkflowOutput struct {
	Body workflowResponse
}

func (s *Server) handleCreateWorkflow(ctx context.Context, input *CreateWorkflowInput) (*CreateWorkflowOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	if err := validateWorkflowSteps(req.Steps); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := s.validateWorkflowStepProjectReferences(ctx, req.ProjectID, req.Steps); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := s.checkWorkflowStepLimit(ctx, req.ProjectID, len(req.Steps)); err != nil {
		return nil, err
	}
	if err := s.checkWorkflowStepFeatures(ctx, req.ProjectID, req.Steps); err != nil {
		return nil, err
	}
	candidateSteps := workflowStepsFromRequests(req.Steps)
	if err := s.validateWorkflowPolicy(ctx, req.ProjectID, candidateSteps); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := validateSingletonConfig(req.SingletonKeyExpr, req.SingletonOnConflict, req.SingletonMaxQueueDepth); err != nil {
		return nil, err
	}
	if err := s.checkSingletonOnConflict(ctx, req.ProjectID, req.SingletonOnConflict); err != nil {
		return nil, err
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if len(req.Tags) > 0 {
		if err := validateTags(req.Tags); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
	}

	wf := &domain.Workflow{
		ProjectID:         req.ProjectID,
		Name:              req.Name,
		Slug:              req.Slug,
		Description:       req.Description,
		Tags:              req.Tags,
		Enabled:           enabled,
		TimeoutSecs:       req.TimeoutSecs,
		MaxConcurrentRuns: req.MaxConcurrentRuns,
		MaxParallelSteps:  req.MaxParallelSteps,
		Cron:              req.Cron,
		CronTimezone:      req.CronTimezone,
		SkipIfRunning:     req.SkipIfRunning,
		VersionPolicy:     domain.VersionPolicyPin,
		CreatedBy:         actorFromContext(ctx),
		UpdatedBy:         actorFromContext(ctx),

		SingletonKeyExpr:       req.SingletonKeyExpr,
		SingletonOnConflict:    domain.SingletonOnConflict(req.SingletonOnConflict),
		SingletonMaxQueueDepth: req.SingletonMaxQueueDepth,
	}

	if req.VersionPolicy != "" {
		wf.VersionPolicy = domain.VersionPolicy(req.VersionPolicy)
	}
	if err := validateWorkflowConfig(wf.Cron, wf.CronTimezone, wf.MaxParallelSteps); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := s.checkScheduleLimit(ctx, req.ProjectID, wf.Cron); err != nil {
		return nil, err
	}

	var steps []domain.WorkflowStep
	if err := s.runInTx(ctx, func(txStore APIStore) error {
		if err := txStore.CreateWorkflow(ctx, wf); err != nil {
			return fmt.Errorf("create workflow: %w", err)
		}
		for i := range candidateSteps {
			candidateSteps[i].WorkflowID = wf.ID
			if err := txStore.CreateWorkflowStep(ctx, &candidateSteps[i]); err != nil {
				return fmt.Errorf("create workflow step %q: %w", candidateSteps[i].StepRef, err)
			}
			steps = append(steps, candidateSteps[i])
		}
		if err := txStore.CreateWorkflowVersionSnapshot(ctx, wf.ID, wf.Version); err != nil {
			return fmt.Errorf("create workflow version snapshot: %w", err)
		}
		return nil
	}); err != nil {
		slog.Error("failed to create workflow", "error", err)
		return nil, huma.Error500InternalServerError("failed to create workflow")
	}

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowCreated, "workflow", wf.ID, map[string]any{
		"name":       wf.Name,
		"slug":       wf.Slug,
		"step_count": len(steps),
		"enabled":    wf.Enabled,
		"cron":       wf.Cron,
	})

	return &CreateWorkflowOutput{Body: workflowResponse{Workflow: wf, Steps: steps}}, nil
}

type GetWorkflowInput struct {
	WorkflowID string `path:"workflowID"`
}
type GetWorkflowOutput struct {
	Body workflowResponse
}

func (s *Server) handleGetWorkflow(ctx context.Context, input *GetWorkflowInput) (*GetWorkflowOutput, error) {
	wf, err := s.store.GetWorkflow(ctx, input.WorkflowID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			return nil, huma.Error404NotFound("workflow not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow")
	}

	if err := requireProjectMatch(ctx, wf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	steps, err := s.store.ListStepsByWorkflow(ctx, wf.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow steps")
	}

	return &GetWorkflowOutput{Body: workflowResponse{Workflow: wf, Steps: steps}}, nil
}

type ListWorkflowsInput struct {
	TagKey   string `query:"tag_key"`
	TagValue string `query:"tag_value"`
	Limit    string `query:"limit"`
	Cursor   string `query:"cursor"`
}
type ListWorkflowsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListWorkflows(ctx context.Context, input *ListWorkflowsInput) (*ListWorkflowsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	if input.TagValue != "" && input.TagKey == "" {
		return nil, huma.Error400BadRequest("tag_key is required when tag_value is provided")
	}

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	var workflows []domain.Workflow
	if input.TagKey != "" {
		workflows, err = s.store.ListWorkflowsByTag(ctx, projectID, input.TagKey, input.TagValue, limit+1, cursor)
	} else {
		workflows, err = s.store.ListWorkflows(ctx, projectID, limit+1, cursor)
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflows")
	}

	return &ListWorkflowsOutput{Body: paginatedResult(workflows, limit, func(wf domain.Workflow) string {
		return wf.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

type UpdateWorkflowInput struct {
	WorkflowID string `path:"workflowID"`
	Body       updateWorkflowRequest
}

type updateWorkflowResponseBody struct {
	*domain.Workflow
	Steps                       []domain.WorkflowStep `json:"steps"`
	ActiveRunsOnPreviousVersion *int                  `json:"active_runs_on_previous_version,omitempty"`
	PreviousVersionID           string                `json:"previous_version_id,omitempty"`
}

type UpdateWorkflowOutput struct {
	Body updateWorkflowResponseBody
}

//nolint:funlen,gocognit,gocyclo,cyclop
func (s *Server) handleUpdateWorkflow(ctx context.Context, input *UpdateWorkflowInput) (*UpdateWorkflowOutput, error) {
	wf, err := s.store.GetWorkflow(ctx, input.WorkflowID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			return nil, huma.Error404NotFound("workflow not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow")
	}

	if err := requireProjectMatch(ctx, wf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	// Capture the pre-update version for breaking change detection later.
	previousVersionID := wf.VersionID
	previousVersion := wf.Version
	previousCron := wf.Cron

	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	var candidateSteps []domain.WorkflowStep
	if req.Steps != nil {
		if err := validateWorkflowSteps(*req.Steps); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if err := s.validateWorkflowStepProjectReferences(ctx, wf.ProjectID, *req.Steps); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if err := s.checkWorkflowStepLimit(ctx, wf.ProjectID, len(*req.Steps)); err != nil {
			return nil, err
		}
		if err := s.checkWorkflowStepFeatures(ctx, wf.ProjectID, *req.Steps); err != nil {
			return nil, err
		}

		candidateSteps = workflowStepsFromRequests(*req.Steps)
		if err := s.validateWorkflowPolicy(ctx, wf.ProjectID, candidateSteps); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}

		existingSteps, err := s.store.ListStepsByWorkflow(ctx, wf.ID)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to load existing workflow steps")
		}
		existingRefs := make(map[string]struct{}, len(existingSteps))
		for _, st := range existingSteps {
			existingRefs[st.StepRef] = struct{}{}
		}
		requestedRefs := make(map[string]struct{}, len(*req.Steps))
		for _, stepReq := range *req.Steps {
			requestedRefs[stepReq.StepRef] = struct{}{}
		}
		for ref := range existingRefs {
			if _, ok := requestedRefs[ref]; !ok {
				return nil, huma.Error400BadRequest(fmt.Sprintf("removing step_ref %q is not supported; disable it with step overrides instead", ref))
			}
		}
	}

	if req.Name != nil {
		wf.Name = *req.Name
	}
	if req.Slug != nil {
		wf.Slug = *req.Slug
	}
	if req.Description != nil {
		wf.Description = *req.Description
	}
	if req.Enabled != nil {
		wf.Enabled = *req.Enabled
	}
	if req.TimeoutSecs != nil {
		wf.TimeoutSecs = *req.TimeoutSecs
	}
	if req.MaxConcurrentRuns != nil {
		wf.MaxConcurrentRuns = *req.MaxConcurrentRuns
	}
	if req.MaxParallelSteps != nil {
		wf.MaxParallelSteps = *req.MaxParallelSteps
	}
	if req.Cron != nil {
		wf.Cron = *req.Cron
	}
	if req.CronTimezone != nil {
		wf.CronTimezone = *req.CronTimezone
	}
	if req.SkipIfRunning != nil {
		wf.SkipIfRunning = *req.SkipIfRunning
	}
	if req.Tags != nil {
		if err := validateTags(*req.Tags); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		wf.Tags = *req.Tags
	}
	if req.VersionPolicy != nil {
		wf.VersionPolicy = domain.VersionPolicy(*req.VersionPolicy)
	}
	if req.BackwardsCompatible != nil {
		wf.BackwardsCompatible = *req.BackwardsCompatible
	}
	if req.SingletonKeyExpr != nil {
		wf.SingletonKeyExpr = *req.SingletonKeyExpr
	}
	if req.SingletonOnConflict != nil {
		wf.SingletonOnConflict = domain.SingletonOnConflict(*req.SingletonOnConflict)
	}
	if req.SingletonMaxQueueDepth != nil {
		wf.SingletonMaxQueueDepth = req.SingletonMaxQueueDepth
	}
	if req.SingletonKeyExpr != nil || req.SingletonOnConflict != nil || req.SingletonMaxQueueDepth != nil {
		if err := validateSingletonConfig(wf.SingletonKeyExpr, string(wf.SingletonOnConflict), wf.SingletonMaxQueueDepth); err != nil {
			return nil, err
		}
		if err := s.checkSingletonOnConflict(ctx, wf.ProjectID, string(wf.SingletonOnConflict)); err != nil {
			return nil, err
		}
	}
	if err := validateWorkflowConfig(wf.Cron, wf.CronTimezone, wf.MaxParallelSteps); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if previousCron == "" && wf.Cron != "" {
		if err := s.checkScheduleLimit(ctx, wf.ProjectID, wf.Cron); err != nil {
			return nil, err
		}
	}

	wf.UpdatedBy = actorFromContext(ctx)

	if err := s.runInTx(ctx, func(txStore APIStore) error {
		if err := txStore.UpdateWorkflow(ctx, wf); err != nil {
			return fmt.Errorf("update workflow: %w", err)
		}
		if req.Steps != nil {
			if err := txStore.DeleteStepsByWorkflow(ctx, wf.ID); err != nil {
				return fmt.Errorf("delete workflow steps: %w", err)
			}
			for i := range candidateSteps {
				candidateSteps[i].WorkflowID = wf.ID
				if err := txStore.CreateWorkflowStep(ctx, &candidateSteps[i]); err != nil {
					return fmt.Errorf("create workflow step %q: %w", candidateSteps[i].StepRef, err)
				}
			}
		}
		if err := txStore.CreateWorkflowVersionSnapshot(ctx, wf.ID, wf.Version); err != nil {
			return fmt.Errorf("create workflow version snapshot: %w", err)
		}
		return nil
	}); err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			return nil, huma.Error404NotFound("workflow not found")
		}
		slog.Error("failed to update workflow", "error", err)
		return nil, huma.Error500InternalServerError("failed to update workflow")
	}

	steps, err := s.store.ListStepsByWorkflow(ctx, wf.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow steps")
	}

	resp := updateWorkflowResponseBody{Workflow: wf, Steps: steps}

	// Check for active runs on the previous version to warn about breaking changes.
	if previousVersionID != "" && previousVersion >= 1 {
		count, countErr := s.store.CountActiveWorkflowRunsByVersion(ctx, wf.ID, previousVersionID)
		if countErr != nil {
			slog.Warn("failed to count active runs on previous version", "workflow_id", wf.ID, "version_id", previousVersionID, "error", countErr)
		}
		if countErr == nil && count > 0 {
			resp.ActiveRunsOnPreviousVersion = &count
			resp.PreviousVersionID = previousVersionID
		}
		if req.BreakingChange != nil && *req.BreakingChange && countErr == nil && count > 0 {
			s.emitAuditEvent(ctx, domain.AuditActionWorkflowUpdatedBreaking, "workflow", wf.ID, map[string]any{
				"previous_version_id":             previousVersionID,
				"active_runs_on_previous_version": count,
				"new_version":                     wf.Version,
			})
		} else {
			s.emitAuditEvent(ctx, domain.AuditActionWorkflowUpdated, "workflow", wf.ID, map[string]any{
				"changes":             req,
				"name":                wf.Name,
				"previous_version_id": previousVersionID,
				"new_version":         wf.Version,
			})
		}
	} else {
		s.emitAuditEvent(ctx, domain.AuditActionWorkflowUpdated, "workflow", wf.ID, map[string]any{
			"changes":     req,
			"name":        wf.Name,
			"new_version": wf.Version,
		})
	}

	return &UpdateWorkflowOutput{Body: resp}, nil
}

type DeleteWorkflowInput struct {
	WorkflowID string `path:"workflowID"`
}

func (s *Server) handleDeleteWorkflow(ctx context.Context, input *DeleteWorkflowInput) (*struct{}, error) {
	wf, err := s.store.GetWorkflow(ctx, input.WorkflowID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			return nil, huma.Error404NotFound("workflow not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow")
	}

	if err := requireProjectMatch(ctx, wf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	activeCount, countErr := s.store.CountRunningWorkflowRuns(ctx, input.WorkflowID)
	if countErr != nil {
		return nil, huma.Error500InternalServerError("failed to check active workflow runs")
	}
	if activeCount > 0 {
		return nil, huma.Error409Conflict(fmt.Sprintf("workflow has %d active run(s) -- cancel them before deleting", activeCount))
	}

	if err := s.store.DeleteWorkflow(ctx, input.WorkflowID); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete workflow")
	}

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowDeleted, "workflow", input.WorkflowID, map[string]any{
		"name": wf.Name,
		"slug": wf.Slug,
	})

	return nil, nil
}

type TriggerWorkflowInput struct {
	WorkflowID string `path:"workflowID"`
	Body       triggerWorkflowRequest
}
type TriggerWorkflowOutput struct {
	Body any
}

// triggeredWorkflowRunResponse is the trigger response when a singleton policy
// applied and a run exists (dispatched/queued_behind/replaced). The embedded
// run's fields are promoted to the top level so the response keeps the normal
// WorkflowRun shape, with the two singleton fields added additively.
type triggeredWorkflowRunResponse struct {
	*domain.WorkflowRun
	SingletonOutcome     string `json:"singleton_outcome,omitempty"`
	SingletonHolderRunID string `json:"singleton_holder_run_id,omitempty"`
}

func (s *Server) handleTriggerWorkflow(ctx context.Context, input *TriggerWorkflowInput) (*TriggerWorkflowOutput, error) {
	if s.workflowEngine == nil {
		return nil, huma.Error503ServiceUnavailable("workflow engine unavailable")
	}

	workflowID := input.WorkflowID
	wf, err := s.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			return nil, huma.Error404NotFound("workflow not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow")
	}
	if wf == nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	if err := requireProjectMatch(ctx, wf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	if !wf.Enabled {
		return nil, huma.Error409Conflict("workflow is disabled")
	}

	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	if len(req.Tags) > 0 {
		if err := validateTags(req.Tags); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, workflowID, wf.Version)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load workflow steps")
	}
	if err := s.validateWorkflowPolicy(ctx, wf.ProjectID, steps); err != nil {
		return nil, huma.Error409Conflict(err.Error())
	}

	triggeredBy := req.TriggeredBy
	if triggeredBy == "" {
		triggeredBy = domain.TriggerManual
	}

	run, singletonOutcome, singletonHolderRunID, err := s.workflowEngine.TriggerWorkflowWithOutcome(ctx, workflowID, req.ProjectID, req.Payload, triggeredBy, req.StepOverrides, req.Tags)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			return nil, huma.Error404NotFound("workflow not found")
		}
		if errors.Is(err, domain.ErrSingletonKeyUnresolvable) {
			return nil, huma.Error400BadRequest(err.Error())
		}
		slog.Error("failed to trigger workflow", "error", err, "workflow_id", workflowID)
		return nil, huma.Error500InternalServerError("failed to trigger workflow")
	}

	// Singleton drop: no run is created. Report the outcome and the holder it
	// lost to, mirroring the job trigger response.
	if singletonOutcome == domain.SingletonOutcomeDropped {
		s.emitAuditEventAsync(ctx, domain.AuditActionWorkflowTriggered, "workflow", workflowID, map[string]any{
			"triggered_by":      triggeredBy,
			"tag_keys":          tagKeys(req.Tags),
			"singleton_outcome": string(singletonOutcome),
		})
		body := map[string]any{"singleton_outcome": string(singletonOutcome)}
		if singletonHolderRunID != "" {
			body["singleton_holder_run_id"] = singletonHolderRunID
		}
		return &TriggerWorkflowOutput{Body: body}, nil
	}

	// Stamp audit field -- engine doesn't have access to actor context.
	if actor := actorFromContext(ctx); actor != "" {
		run.CreatedBy = actor
	}

	if len(req.Labels) > 0 {
		if err := s.store.CreateWorkflowRunLabels(ctx, run.ID, req.Labels); err != nil {
			return nil, huma.Error500InternalServerError("failed to persist workflow run labels")
		}
	}
	s.publishWorkflowRunHook(ctx, run, domain.WfStatusPending, run.Status, "trigger")

	auditMeta := map[string]any{
		"run_id":       run.ID,
		"triggered_by": triggeredBy,
		"tag_keys":     tagKeys(req.Tags),
	}
	if singletonOutcome != "" {
		auditMeta["singleton_outcome"] = string(singletonOutcome)
		auditMeta["singleton_key_hash"] = hashIdempotencyKey(run.SingletonKey)
	}
	s.emitAuditEventAsync(ctx, domain.AuditActionWorkflowTriggered, "workflow", workflowID, auditMeta)

	if singletonOutcome != "" {
		return &TriggerWorkflowOutput{Body: &triggeredWorkflowRunResponse{
			WorkflowRun:          run,
			SingletonOutcome:     string(singletonOutcome),
			SingletonHolderRunID: singletonHolderRunID,
		}}, nil
	}

	return &TriggerWorkflowOutput{Body: run}, nil
}

func workflowStepsFromRequests(stepReqs []workflowStepRequest) []domain.WorkflowStep {
	steps := make([]domain.WorkflowStep, 0, len(stepReqs))
	for _, stepReq := range stepReqs {
		steps = append(steps, domain.WorkflowStep{
			JobID:                   stepReq.JobID,
			StepRef:                 stepReq.StepRef,
			DependsOn:               stepReq.DependsOn,
			Condition:               stepReq.Condition,
			OnFailure:               stepReq.OnFailure,
			Payload:                 stepReq.Payload,
			StepType:                stepReq.StepType,
			ApprovalTimeoutSecs:     stepReq.ApprovalTimeoutSecs,
			ApprovalApprovers:       stepReq.ApprovalApprovers,
			RetryMaxAttempts:        stepReq.RetryMaxAttempts,
			RetryBackoff:            stepReq.RetryBackoff,
			RetryInitialDelaySecs:   stepReq.RetryInitialDelaySecs,
			RetryMaxDelaySecs:       stepReq.RetryMaxDelaySecs,
			TimeoutSecsOverride:     stepReq.TimeoutSecsOverride,
			OutputTransform:         stepReq.OutputTransform,
			SubWorkflowID:           stepReq.SubWorkflowID,
			MaxNestingDepth:         stepReq.MaxNestingDepth,
			EventKey:                stepReq.EventKey,
			EventTimeoutSecs:        stepReq.EventTimeoutSecs,
			EventNotifyURL:          stepReq.EventNotifyURL,
			EventEmitKey:            stepReq.EventEmitKey,
			SleepDurationSecs:       stepReq.SleepDurationSecs,
			ConcurrencyKey:          stepReq.ConcurrencyKey,
			ResourceClass:           stepReq.ResourceClass,
			ExpectedDurationSecs:    stepReq.ExpectedDurationSecs,
			StageNotifications:      stepReq.StageNotifications,
			CompensationJobID:       stepReq.CompensationJobID,
			CompensationTimeoutSecs: stepReq.CompensationTimeoutSecs,
		})
	}
	return steps
}

func (s *Server) validateWorkflowStepProjectReferences(ctx context.Context, projectID string, steps []workflowStepRequest) error {
	seenJobs := make(map[string]struct{})
	for _, step := range steps {
		for _, jobID := range []string{step.JobID, step.CompensationJobID} {
			if jobID == "" {
				continue
			}
			if _, ok := seenJobs[jobID]; ok {
				continue
			}
			seenJobs[jobID] = struct{}{}
			job, err := s.store.GetJob(ctx, jobID)
			if err != nil {
				return fmt.Errorf("step %s references unknown job %s", step.StepRef, jobID)
			}
			if job != nil && job.ProjectID != projectID {
				return fmt.Errorf("step %s references job outside workflow project", step.StepRef)
			}
		}
		if step.SubWorkflowID == "" {
			continue
		}
		wf, err := s.store.GetWorkflow(ctx, step.SubWorkflowID)
		if err != nil {
			return fmt.Errorf("step %s references unknown sub_workflow %s", step.StepRef, step.SubWorkflowID)
		}
		if wf != nil && wf.ProjectID != projectID {
			return fmt.Errorf("step %s references sub_workflow outside workflow project", step.StepRef)
		}
	}
	return nil
}

//nolint:gocognit
func (s *Server) validateWorkflowPolicy(ctx context.Context, projectID string, steps []domain.WorkflowStep) error {
	policy, err := s.store.GetWorkflowPolicyByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("load workflow policy: %w", err)
	}
	if policy == nil {
		return nil
	}

	if policy.MaxFanOut > 0 {
		fanOutByRef := make(map[string]int, len(steps))
		for _, step := range steps {
			for _, dep := range step.DependsOn {
				fanOutByRef[dep]++
			}
		}
		for ref, fanOut := range fanOutByRef {
			if fanOut > policy.MaxFanOut {
				return fmt.Errorf("workflow policy violation: step %s fan-out %d exceeds max_fan_out %d", ref, fanOut, policy.MaxFanOut)
			}
		}
	}

	if policy.MaxDepth > 0 {
		stepByRef := lo.KeyBy(steps, func(step domain.WorkflowStep) string { return step.StepRef })
		memo := make(map[string]int, len(steps))
		visiting := make(map[string]bool, len(steps))

		var depth func(ref string) (int, error)
		depth = func(ref string) (int, error) {
			if d, ok := memo[ref]; ok {
				return d, nil
			}
			if visiting[ref] {
				return 0, fmt.Errorf("cycle detected at step_ref %q", ref)
			}
			step, ok := stepByRef[ref]
			if !ok {
				return 0, nil
			}
			visiting[ref] = true
			maxParentDepth := 0
			for _, dep := range step.DependsOn {
				depDepth, err := depth(dep)
				if err != nil {
					return 0, err
				}
				if depDepth > maxParentDepth {
					maxParentDepth = depDepth
				}
			}
			visiting[ref] = false
			memo[ref] = maxParentDepth + 1
			return memo[ref], nil
		}

		maxDepth := 0
		for _, step := range steps {
			d, err := depth(step.StepRef)
			if err != nil {
				return fmt.Errorf("workflow policy violation: %w", err)
			}
			if d > maxDepth {
				maxDepth = d
			}
		}
		if maxDepth > policy.MaxDepth {
			return fmt.Errorf("workflow policy violation: workflow depth %d exceeds max_depth %d", maxDepth, policy.MaxDepth)
		}
	}

	if len(policy.ForbiddenStepTypes) > 0 {
		forbidden := make(map[string]struct{}, len(policy.ForbiddenStepTypes))
		for _, stepType := range policy.ForbiddenStepTypes {
			forbidden[strings.ToLower(strings.TrimSpace(stepType))] = struct{}{}
		}
		for _, step := range steps {
			stepType := step.StepType
			if stepType == "" {
				stepType = domain.WorkflowStepTypeJob
			}
			if _, blocked := forbidden[strings.ToLower(string(stepType))]; blocked {
				return fmt.Errorf("workflow policy violation: step %s uses forbidden step_type %s", step.StepRef, stepType)
			}
		}
	}

	if policy.RequireApprovalForDeploy {
		hasApproval := lo.ContainsBy(steps, func(step domain.WorkflowStep) bool {
			return step.StepType == domain.WorkflowStepTypeApproval
		})
		hasDeployLikeStep := lo.ContainsBy(steps, func(step domain.WorkflowStep) bool {
			stepType := step.StepType
			if stepType == "" {
				stepType = domain.WorkflowStepTypeJob
			}
			if stepType != domain.WorkflowStepTypeJob {
				return false
			}
			ref := strings.ToLower(step.StepRef)
			return strings.Contains(ref, "deploy") || strings.Contains(ref, "release")
		})
		if hasDeployLikeStep && !hasApproval {
			return fmt.Errorf("workflow policy violation: approval step is required for deploy-like workflows")
		}
	}

	return nil
}

const maxWorkflowSteps = 1000

//nolint:gocognit,gocyclo,cyclop
func validateWorkflowSteps(steps []workflowStepRequest) error {
	if len(steps) > maxWorkflowSteps {
		return fmt.Errorf("workflow cannot have more than %d steps", maxWorkflowSteps)
	}

	// Build set of known step_refs and check for duplicates.
	knownRefs := make(map[string]bool, len(steps))
	for _, step := range steps {
		if step.StepRef == "" {
			return errors.New("each step requires step_ref")
		}
		if knownRefs[step.StepRef] {
			return fmt.Errorf("duplicate step_ref: %s", step.StepRef)
		}
		knownRefs[step.StepRef] = true
	}

	for _, step := range steps {
		if step.StepType == "" {
			step.StepType = domain.WorkflowStepTypeJob
		}
		if !isValidWorkflowStepType(step.StepType) {
			return fmt.Errorf("step %s has invalid step_type %q", step.StepRef, step.StepType)
		}
		if step.StepType == domain.WorkflowStepTypeJob && step.JobID == "" {
			return errors.New("job steps require job_id")
		}
		if step.StepType == domain.WorkflowStepTypeApproval {
			if len(step.ApprovalApprovers) == 0 {
				return errors.New("approval steps require approval_approvers")
			}
			if step.ApprovalTimeoutSecs < 0 {
				return errors.New("approval_timeout_secs must be >= 0")
			}
		}
		if step.StepType == domain.WorkflowStepTypeSubWorkflow {
			if step.SubWorkflowID == "" {
				return errors.New("sub_workflow steps require sub_workflow_id")
			}
			if step.JobID != "" {
				return errors.New("sub_workflow steps must not have job_id")
			}
			if step.MaxNestingDepth < 0 {
				return errors.New("max_nesting_depth must be >= 0")
			}
		}
		if step.StepType == domain.WorkflowStepTypeWaitForEvent {
			if step.EventKey == "" {
				return errors.New("wait_for_event steps require event_key")
			}
			if len(step.EventKey) > 512 {
				return errors.New("event_key must be at most 512 characters")
			}
		}
		if step.EventNotifyURL != "" {
			if err := validateURL(step.EventNotifyURL); err != nil {
				return fmt.Errorf("step %s has invalid event_notify_url: %w", step.StepRef, err)
			}
		}
		if step.StepType == domain.WorkflowStepTypeSleep {
			if step.SleepDurationSecs <= 0 {
				return errors.New("sleep steps require sleep_duration_secs > 0")
			}
			if step.SleepDurationSecs > domain.MaxSleepDurationSecs {
				return fmt.Errorf("sleep_duration_secs must be <= %d", domain.MaxSleepDurationSecs)
			}
		}
		if step.TimeoutSecsOverride < 0 {
			return errors.New("timeout_secs_override must be >= 0")
		}
		if len(step.ConcurrencyKey) > 128 {
			return errors.New("concurrency_key must be at most 128 characters")
		}
		if step.ResourceClass != "" && step.ResourceClass != "small" && step.ResourceClass != "medium" && step.ResourceClass != "large" {
			return errors.New("resource_class must be one of small, medium, large")
		}
		for _, dep := range step.DependsOn {
			if dep == "" {
				return errors.New("depends_on cannot contain empty values")
			}
			if dep == step.StepRef {
				return errors.New("step cannot depend on itself")
			}
			if !knownRefs[dep] {
				return fmt.Errorf("step %q depends on unknown step %q", step.StepRef, dep)
			}
		}
	}

	// Detect cycles via topological sort (Kahn's algorithm).
	inDegree := make(map[string]int, len(steps))
	adj := make(map[string][]string, len(steps))
	for _, step := range steps {
		if _, ok := inDegree[step.StepRef]; !ok {
			inDegree[step.StepRef] = 0
		}
		for _, dep := range step.DependsOn {
			adj[dep] = append(adj[dep], step.StepRef)
			inDegree[step.StepRef]++
		}
	}
	queue := make([]string, 0, len(steps))
	for ref, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, ref)
		}
	}
	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, next := range adj[node] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if visited != len(steps) {
		return errors.New("workflow has circular dependencies")
	}

	return nil
}

func isValidWorkflowStepType(stepType domain.WorkflowStepType) bool {
	switch stepType {
	case domain.WorkflowStepTypeJob,
		domain.WorkflowStepTypeApproval,
		domain.WorkflowStepTypeSubWorkflow,
		domain.WorkflowStepTypeWaitForEvent,
		domain.WorkflowStepTypeSleep:
		return true
	default:
		return false
	}
}

type DryRunWorkflowInput struct {
	WorkflowID string `path:"workflowID"`
	Body       dryRunWorkflowRequest
}
type DryRunWorkflowOutput struct {
	Body any
}

func (s *Server) handleDryRunWorkflow(ctx context.Context, input *DryRunWorkflowInput) (*DryRunWorkflowOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	wf, err := s.store.GetWorkflow(ctx, input.WorkflowID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			return nil, huma.Error404NotFound("workflow not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow")
	}
	if wf == nil {
		return nil, huma.Error404NotFound("workflow not found")
	}
	if err := requireProjectMatch(ctx, wf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	if len(req.Steps) == 0 {
		steps, err := s.store.ListStepsByWorkflow(ctx, input.WorkflowID)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to list workflow steps")
		}
		if err := workflow.ValidateDAG(steps); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		s.emitAuditEvent(ctx, domain.AuditActionWorkflowDryRun, "workflow", input.WorkflowID, map[string]any{
			"step_count": len(steps),
			"mode":       "existing_steps",
		})
		return &DryRunWorkflowOutput{Body: map[string]any{"valid": true, "step_count": len(steps)}}, nil
	}

	if err := validateWorkflowSteps(req.Steps); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	steps := make([]domain.WorkflowStep, 0, len(req.Steps))
	for _, sreq := range req.Steps {
		steps = append(steps, domain.WorkflowStep{StepRef: sreq.StepRef, DependsOn: sreq.DependsOn})
	}
	if err := workflow.ValidateDAG(steps); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowDryRun, "workflow", input.WorkflowID, map[string]any{
		"step_count": len(steps),
		"mode":       "request_steps",
	})

	return &DryRunWorkflowOutput{Body: map[string]any{"valid": true, "step_count": len(steps)}}, nil
}

type workflowPlanRequest struct {
	StepOverrides []domain.StepOverride `json:"step_overrides,omitempty"`
}

type WorkflowPlanInput struct {
	WorkflowID string `path:"workflowID"`
	Body       workflowPlanRequest
}
type WorkflowPlanOutput struct {
	Body any
}

func (s *Server) handleWorkflowPlan(ctx context.Context, input *WorkflowPlanInput) (*WorkflowPlanOutput, error) {
	workflowID := input.WorkflowID
	req := input.Body

	wf, err := s.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get workflow")
	}
	if wf == nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, workflowID, wf.Version)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load workflow steps")
	}

	if len(req.StepOverrides) > 0 {
		steps, err = applyStepOverridesForPlan(steps, req.StepOverrides)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
	}

	if err := workflow.ValidateDAG(steps); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	indegree := make(map[string]int, len(steps))
	adj := make(map[string][]string, len(steps))
	for _, st := range steps {
		indegree[st.StepRef] = 0
		adj[st.StepRef] = []string{}
	}
	for _, st := range steps {
		for _, dep := range st.DependsOn {
			adj[dep] = append(adj[dep], st.StepRef)
			indegree[st.StepRef]++
		}
	}

	queue := make([]string, 0, len(steps))
	roots := make([]string, 0)
	for ref, deg := range indegree {
		if deg == 0 {
			queue = append(queue, ref)
			roots = append(roots, ref)
		}
	}
	sort.Strings(queue)
	sort.Strings(roots)

	topo := make([]string, 0, len(steps))
	for len(queue) > 0 {
		ref := queue[0]
		queue = queue[1:]
		topo = append(topo, ref)
		for _, dep := range adj[ref] {
			indegree[dep]--
			if indegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
		sort.Strings(queue)
	}

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowPlanRequested, "workflow", workflowID, map[string]any{
		"step_count":       len(steps),
		"override_count":   len(req.StepOverrides),
		"workflow_version": wf.Version,
	})

	return &WorkflowPlanOutput{Body: map[string]any{
		"workflow_id":       workflowID,
		"workflow_version":  wf.Version,
		"step_count":        len(steps),
		"roots":             roots,
		"topological_order": topo,
	}}, nil
}

func applyStepOverridesForPlan(steps []domain.WorkflowStep, overrides []domain.StepOverride) ([]domain.WorkflowStep, error) {
	disabled := make(map[string]struct{})
	known := make(map[string]struct{}, len(steps))
	for _, s := range steps {
		known[s.StepRef] = struct{}{}
	}
	for _, o := range overrides {
		if _, ok := known[o.StepRef]; !ok {
			return nil, fmt.Errorf("step override references unknown step_ref %q", o.StepRef)
		}
		if !o.Enabled {
			disabled[o.StepRef] = struct{}{}
		}
	}
	if len(disabled) == 0 {
		return steps, nil
	}

	filtered := make([]domain.WorkflowStep, 0, len(steps))
	for _, s := range steps {
		if _, skip := disabled[s.StepRef]; skip {
			continue
		}
		if len(s.DependsOn) > 0 {
			deps := make([]string, 0, len(s.DependsOn))
			for _, dep := range s.DependsOn {
				if _, skip := disabled[dep]; !skip {
					deps = append(deps, dep)
				}
			}
			s.DependsOn = deps
		}
		filtered = append(filtered, s)
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("all steps disabled by overrides")
	}
	return filtered, nil
}

type WorkflowGraphInput struct {
	WorkflowID string `path:"workflowID"`
	Format     string `query:"format"`
}
type WorkflowGraphOutput struct {
	Body workflowGraphResponse
}

func (s *Server) handleWorkflowGraph(ctx context.Context, input *WorkflowGraphInput) (*WorkflowGraphOutput, error) {
	workflowID := input.WorkflowID
	format := strings.ToLower(input.Format)

	wf, err := s.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			return nil, huma.Error404NotFound("workflow not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow")
	}
	if wf == nil {
		return nil, huma.Error404NotFound("workflow not found")
	}
	if err := requireProjectMatch(ctx, wf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	steps, err := s.store.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow steps")
	}

	adj := make(map[string][]string, len(steps))
	indegree := make(map[string]int, len(steps))
	for _, st := range steps {
		adj[st.StepRef] = []string{}
		indegree[st.StepRef] = 0
	}
	for _, st := range steps {
		for _, dep := range st.DependsOn {
			adj[dep] = append(adj[dep], st.StepRef)
			indegree[st.StepRef]++
		}
	}

	roots := make([]string, 0)
	for ref, degree := range indegree {
		if degree == 0 {
			roots = append(roots, ref)
		}
		sort.Strings(adj[ref])
	}
	sort.Strings(roots)

	resp := workflowGraphResponse{WorkflowID: workflowID, Roots: roots}
	if format == "dot" {
		resp.DOT = buildWorkflowDOT(adj)
		return &WorkflowGraphOutput{Body: resp}, nil
	}
	resp.Adjacency = adj
	return &WorkflowGraphOutput{Body: resp}, nil
}

func buildWorkflowDOT(adjacency map[string][]string) string {
	var b strings.Builder
	b.WriteString("digraph workflow {\n")
	keys := lo.Keys(adjacency)
	sort.Strings(keys)
	for _, src := range keys {
		dsts := adjacency[src]
		if len(dsts) == 0 {
			_, _ = fmt.Fprintf(&b, "  \"%s\";\n", src)
			continue
		}
		for _, dst := range dsts {
			_, _ = fmt.Fprintf(&b, "  \"%s\" -> \"%s\";\n", src, dst)
		}
	}
	b.WriteString("}\n")
	return b.String()
}

func validateWorkflowConfig(cronExpr, cronTimezone string, maxParallelSteps int) error {
	if maxParallelSteps < 0 {
		return errors.New("max_parallel_steps must be >= 0")
	}
	if cronExpr == "" {
		return nil
	}
	if cronTimezone != "" {
		if _, err := time.LoadLocation(cronTimezone); err != nil {
			return errors.New("invalid cron_timezone")
		}
	}
	if _, err := cron.ParseStandard(cronExpr); err != nil {
		return errors.New("invalid cron expression")
	}
	return nil
}

type cloneWorkflowRequest struct {
	Name      string `json:"name,omitempty"`
	Slug      string `json:"slug,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

type CloneWorkflowInput struct {
	WorkflowID string `path:"workflowID"`
	Body       cloneWorkflowRequest
}
type CloneWorkflowOutput struct {
	Body workflowResponse
}

func (s *Server) handleCloneWorkflow(ctx context.Context, input *CloneWorkflowInput) (*CloneWorkflowOutput, error) {
	sourceID := input.WorkflowID

	sourceWf, err := s.store.GetWorkflow(ctx, sourceID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			return nil, huma.Error404NotFound("workflow not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow")
	}

	if err := requireProjectMatch(ctx, sourceWf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	req := input.Body

	newName := sourceWf.Name + " (copy)"
	if req.Name != "" {
		newName = req.Name
	}
	newSlug := sourceWf.Slug + "-copy"
	if req.Slug != "" {
		newSlug = req.Slug
	}
	projectID := sourceWf.ProjectID
	if req.ProjectID != "" && req.ProjectID != sourceWf.ProjectID {
		return nil, huma.Error404NotFound("workflow not found")
	}

	newWf := &domain.Workflow{
		ProjectID:           projectID,
		Name:                newName,
		Slug:                newSlug,
		Description:         sourceWf.Description,
		Enabled:             sourceWf.Enabled,
		TimeoutSecs:         sourceWf.TimeoutSecs,
		MaxConcurrentRuns:   sourceWf.MaxConcurrentRuns,
		MaxParallelSteps:    sourceWf.MaxParallelSteps,
		Cron:                sourceWf.Cron,
		CronTimezone:        sourceWf.CronTimezone,
		SkipIfRunning:       sourceWf.SkipIfRunning,
		Tags:                sourceWf.Tags,
		VersionPolicy:       sourceWf.VersionPolicy,
		BackwardsCompatible: sourceWf.BackwardsCompatible,
		CreatedBy:           actorFromContext(ctx),
		UpdatedBy:           actorFromContext(ctx),
	}
	sourceSteps, err := s.store.ListStepsByWorkflow(ctx, sourceID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list source workflow steps")
	}

	// Enforce plan gates on the cloned workflow's step count and types.
	if err := s.checkWorkflowStepLimit(ctx, projectID, len(sourceSteps)); err != nil {
		return nil, err
	}
	if err := s.checkScheduleLimit(ctx, projectID, newWf.Cron); err != nil {
		return nil, err
	}
	for _, step := range sourceSteps {
		switch step.StepType {
		case domain.WorkflowStepTypeApproval:
			if err := s.checkFeatureAllowed(ctx, projectID, billing.FeatureApprovalGates, "Approval gates"); err != nil {
				return nil, err
			}
		case domain.WorkflowStepTypeSubWorkflow:
			if err := s.checkFeatureAllowed(ctx, projectID, billing.FeatureSubWorkflows, "Sub-workflows"); err != nil {
				return nil, err
			}
		default:
		}
	}

	newSteps := make([]domain.WorkflowStep, 0, len(sourceSteps))
	if err := s.runInTx(ctx, func(txStore APIStore) error {
		if err := txStore.CreateWorkflow(ctx, newWf); err != nil {
			return fmt.Errorf("create cloned workflow: %w", err)
		}
		for _, src := range sourceSteps {
			step := domain.WorkflowStep{
				WorkflowID:              newWf.ID,
				JobID:                   src.JobID,
				StepRef:                 src.StepRef,
				DependsOn:               src.DependsOn,
				Condition:               src.Condition,
				OnFailure:               src.OnFailure,
				Payload:                 src.Payload,
				StepType:                src.StepType,
				ApprovalTimeoutSecs:     src.ApprovalTimeoutSecs,
				ApprovalApprovers:       src.ApprovalApprovers,
				RetryMaxAttempts:        src.RetryMaxAttempts,
				RetryBackoff:            src.RetryBackoff,
				RetryInitialDelaySecs:   src.RetryInitialDelaySecs,
				RetryMaxDelaySecs:       src.RetryMaxDelaySecs,
				TimeoutSecsOverride:     src.TimeoutSecsOverride,
				OutputTransform:         src.OutputTransform,
				SubWorkflowID:           src.SubWorkflowID,
				MaxNestingDepth:         src.MaxNestingDepth,
				EventKey:                src.EventKey,
				EventTimeoutSecs:        src.EventTimeoutSecs,
				EventNotifyURL:          src.EventNotifyURL,
				EventEmitKey:            src.EventEmitKey,
				SleepDurationSecs:       src.SleepDurationSecs,
				ConcurrencyKey:          src.ConcurrencyKey,
				ResourceClass:           src.ResourceClass,
				ExpectedDurationSecs:    src.ExpectedDurationSecs,
				StageNotifications:      src.StageNotifications,
				CompensationJobID:       src.CompensationJobID,
				CompensationTimeoutSecs: src.CompensationTimeoutSecs,
			}
			if err := txStore.CreateWorkflowStep(ctx, &step); err != nil {
				return fmt.Errorf("create cloned workflow step %q: %w", step.StepRef, err)
			}
			newSteps = append(newSteps, step)
		}
		if err := txStore.CreateWorkflowVersionSnapshot(ctx, newWf.ID, newWf.Version); err != nil {
			return fmt.Errorf("snapshot cloned workflow version: %w", err)
		}
		return nil
	}); err != nil {
		slog.Error("failed to clone workflow", "error", err)
		return nil, huma.Error500InternalServerError("failed to clone workflow")
	}

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowCloned, "workflow", newWf.ID, map[string]any{
		"source_workflow_id": sourceID,
		"new_name":           newWf.Name,
		"new_slug":           newWf.Slug,
		"step_count":         len(newSteps),
	})

	return &CloneWorkflowOutput{Body: workflowResponse{Workflow: newWf, Steps: newSteps}}, nil
}

type GetActiveVersionsInput struct {
	WorkflowID string `path:"workflowID"`
}
type activeVersionsResponseBody struct {
	WorkflowID string                `json:"workflow_id"`
	Versions   []store.ActiveVersion `json:"versions"`
}

type GetActiveVersionsOutput struct {
	Body activeVersionsResponseBody
}

func (s *Server) handleGetActiveVersions(ctx context.Context, input *GetActiveVersionsInput) (*GetActiveVersionsOutput, error) {
	versions, err := s.store.ListActiveWorkflowVersions(ctx, input.WorkflowID)
	if err != nil {
		slog.Error("failed to list active versions", "workflow_id", input.WorkflowID, "error", err)
		return nil, huma.Error500InternalServerError("failed to list active versions")
	}
	if versions == nil {
		versions = []store.ActiveVersion{}
	}

	return &GetActiveVersionsOutput{Body: activeVersionsResponseBody{
		WorkflowID: input.WorkflowID,
		Versions:   versions,
	}}, nil
}
