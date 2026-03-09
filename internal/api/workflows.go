package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/workflow"

	"github.com/go-chi/chi/v5"
	"github.com/robfig/cron/v3"
	"github.com/samber/lo"
)

type workflowStepRequest struct {
	JobID                 string                    `json:"job_id,omitempty"`
	StepRef               string                    `json:"step_ref" validate:"required"`
	DependsOn             []string                  `json:"depends_on,omitempty"`
	Condition             json.RawMessage           `json:"condition,omitempty"`
	OnFailure             domain.FailurePolicy      `json:"on_failure,omitempty"`
	Payload               json.RawMessage           `json:"payload,omitempty"`
	StepType              domain.WorkflowStepType   `json:"step_type,omitempty"`
	ApprovalTimeoutSecs   int                       `json:"approval_timeout_secs,omitempty"`
	ApprovalApprovers     []string                  `json:"approval_approvers,omitempty"`
	RetryMaxAttempts      int                       `json:"retry_max_attempts,omitempty"`
	RetryBackoff          domain.RetryBackoffPolicy `json:"retry_backoff,omitempty"`
	RetryInitialDelaySecs int                       `json:"retry_initial_delay_secs,omitempty"`
	RetryMaxDelaySecs     int                       `json:"retry_max_delay_secs,omitempty"`
	TimeoutSecsOverride   int                       `json:"timeout_secs_override,omitempty"`
	OutputTransform       string                    `json:"output_transform,omitempty"`
	SubWorkflowID         string                    `json:"sub_workflow_id,omitempty"`
	MaxNestingDepth       int                       `json:"max_nesting_depth,omitempty"`
}

type createWorkflowRequest struct {
	ProjectID         string                `json:"project_id" validate:"required"`
	Name              string                `json:"name" validate:"required"`
	Slug              string                `json:"slug" validate:"required"`
	Description       string                `json:"description,omitempty"`
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
}

type updateWorkflowRequest struct {
	Name              *string                `json:"name,omitempty"`
	Slug              *string                `json:"slug,omitempty"`
	Description       *string                `json:"description,omitempty"`
	Tags              *map[string]string     `json:"tags,omitempty"`
	Enabled           *bool                  `json:"enabled,omitempty"`
	TimeoutSecs       *int                   `json:"timeout_secs,omitempty"`
	MaxConcurrentRuns *int                   `json:"max_concurrent_runs,omitempty"`
	MaxParallelSteps  *int                   `json:"max_parallel_steps,omitempty"`
	Cron              *string                `json:"cron,omitempty"`
	CronTimezone      *string                `json:"cron_timezone,omitempty"`
	SkipIfRunning     *bool                  `json:"skip_if_running,omitempty"`
	VersionPolicy     *string                `json:"version_policy,omitempty" validate:"omitempty,oneof=pin latest minor"`
	Steps             *[]workflowStepRequest `json:"steps,omitempty"`
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
	StepOverrides []domain.StepOverride `json:"step_overrides,omitempty"`
}

type workflowResponse struct {
	*domain.Workflow
	Steps []domain.WorkflowStep `json:"steps"`
}

func (s *Server) handleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
	var req createWorkflowRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	if err := validateWorkflowSteps(req.Steps); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if len(req.Tags) > 0 {
		if err := validateTags(req.Tags); err != nil {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
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
		CreatedBy:         actorFromContext(r.Context()),
		UpdatedBy:         actorFromContext(r.Context()),
	}

	if req.VersionPolicy != "" {
		wf.VersionPolicy = domain.VersionPolicy(req.VersionPolicy)
	}
	if err := validateWorkflowConfig(wf.Cron, wf.CronTimezone, wf.MaxParallelSteps); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.CreateWorkflow(r.Context(), wf); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create workflow")
		return
	}

	steps := make([]domain.WorkflowStep, 0, len(req.Steps))
	for _, stepReq := range req.Steps {
		step := domain.WorkflowStep{
			WorkflowID:            wf.ID,
			JobID:                 stepReq.JobID,
			StepRef:               stepReq.StepRef,
			DependsOn:             stepReq.DependsOn,
			Condition:             stepReq.Condition,
			OnFailure:             stepReq.OnFailure,
			Payload:               stepReq.Payload,
			StepType:              stepReq.StepType,
			ApprovalTimeoutSecs:   stepReq.ApprovalTimeoutSecs,
			ApprovalApprovers:     stepReq.ApprovalApprovers,
			RetryMaxAttempts:      stepReq.RetryMaxAttempts,
			RetryBackoff:          stepReq.RetryBackoff,
			RetryInitialDelaySecs: stepReq.RetryInitialDelaySecs,
			RetryMaxDelaySecs:     stepReq.RetryMaxDelaySecs,
			TimeoutSecsOverride:   stepReq.TimeoutSecsOverride,
			OutputTransform:       stepReq.OutputTransform,
			SubWorkflowID:         stepReq.SubWorkflowID,
			MaxNestingDepth:       stepReq.MaxNestingDepth,
		}
		if err := s.store.CreateWorkflowStep(r.Context(), &step); err != nil {
			slog.Error("failed to create workflow step", "error", err, "step_ref", step.StepRef, "workflow_id", wf.ID)
			respondError(w, r, http.StatusInternalServerError, "failed to create workflow step")
			return
		}
		steps = append(steps, step)
	}

	if err := s.store.CreateWorkflowVersionSnapshot(r.Context(), wf.ID, wf.Version); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to snapshot workflow version")
		return
	}

	respondJSON(w, http.StatusCreated, workflowResponse{Workflow: wf, Steps: steps})
}

func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	wf, err := s.store.GetWorkflow(r.Context(), workflowID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow")
		return
	}

	steps, err := s.store.ListStepsByWorkflow(r.Context(), wf.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow steps")
		return
	}

	respondJSON(w, http.StatusOK, workflowResponse{Workflow: wf, Steps: steps})
}

func (s *Server) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	projectID := query.Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	tagKey := query.Get("tag_key")
	tagValue := query.Get("tag_value")
	if tagValue != "" && tagKey == "" {
		respondError(w, r, http.StatusBadRequest, "tag_key is required when tag_value is provided")
		return
	}

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	var workflows []domain.Workflow
	if tagKey != "" {
		workflows, err = s.store.ListWorkflowsByTag(r.Context(), projectID, tagKey, tagValue, limit+1, cursor)
	} else {
		workflows, err = s.store.ListWorkflows(r.Context(), projectID, limit+1, cursor)
	}
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflows")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(workflows, limit, func(wf domain.Workflow) string {
		return wf.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleUpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	wf, err := s.store.GetWorkflow(r.Context(), workflowID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow")
		return
	}

	var req updateWorkflowRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
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
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		wf.Tags = *req.Tags
	}
	if req.VersionPolicy != nil {
		wf.VersionPolicy = domain.VersionPolicy(*req.VersionPolicy)
	}
	if err := validateWorkflowConfig(wf.Cron, wf.CronTimezone, wf.MaxParallelSteps); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	wf.UpdatedBy = actorFromContext(r.Context())

	if err := s.store.UpdateWorkflow(r.Context(), wf); err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to update workflow")
		return
	}

	if req.Steps != nil {
		if err := validateWorkflowSteps(*req.Steps); err != nil {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.store.DeleteStepsByWorkflow(r.Context(), wf.ID); err != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to replace workflow steps")
			return
		}
		for _, stepReq := range *req.Steps {
			step := &domain.WorkflowStep{
				WorkflowID:            wf.ID,
				JobID:                 stepReq.JobID,
				StepRef:               stepReq.StepRef,
				DependsOn:             stepReq.DependsOn,
				Condition:             stepReq.Condition,
				OnFailure:             stepReq.OnFailure,
				Payload:               stepReq.Payload,
				StepType:              stepReq.StepType,
				ApprovalTimeoutSecs:   stepReq.ApprovalTimeoutSecs,
				ApprovalApprovers:     stepReq.ApprovalApprovers,
				RetryMaxAttempts:      stepReq.RetryMaxAttempts,
				RetryBackoff:          stepReq.RetryBackoff,
				RetryInitialDelaySecs: stepReq.RetryInitialDelaySecs,
				RetryMaxDelaySecs:     stepReq.RetryMaxDelaySecs,
				TimeoutSecsOverride:   stepReq.TimeoutSecsOverride,
				OutputTransform:       stepReq.OutputTransform,
				SubWorkflowID:         stepReq.SubWorkflowID,
				MaxNestingDepth:       stepReq.MaxNestingDepth,
			}
			if err := s.store.CreateWorkflowStep(r.Context(), step); err != nil {
				respondError(w, r, http.StatusInternalServerError, "failed to create workflow step")
				return
			}
		}
	}

	if err := s.store.CreateWorkflowVersionSnapshot(r.Context(), wf.ID, wf.Version); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to snapshot workflow version")
		return
	}

	steps, err := s.store.ListStepsByWorkflow(r.Context(), wf.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow steps")
		return
	}

	respondJSON(w, http.StatusOK, workflowResponse{Workflow: wf, Steps: steps})
}

func (s *Server) handleDeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	if err := s.store.DeleteWorkflow(r.Context(), workflowID); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to delete workflow")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}

func (s *Server) handleTriggerWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.workflowEngine == nil {
		respondError(w, r, http.StatusServiceUnavailable, "workflow engine unavailable")
		return
	}

	workflowID := chi.URLParam(r, "workflowID")
	wf, err := s.store.GetWorkflow(r.Context(), workflowID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow")
		return
	}
	if wf == nil {
		respondError(w, r, http.StatusNotFound, "workflow not found")
		return
	}
	if !wf.Enabled {
		respondError(w, r, http.StatusConflict, "workflow is disabled")
		return
	}

	var req triggerWorkflowRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	triggeredBy := req.TriggeredBy
	if triggeredBy == "" {
		triggeredBy = domain.TriggerManual
	}

	run, err := s.workflowEngine.TriggerWorkflow(r.Context(), workflowID, req.ProjectID, req.Payload, triggeredBy, req.StepOverrides)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow not found")
			return
		}
		slog.Error("failed to trigger workflow", "error", err, "workflow_id", workflowID)
		respondError(w, r, http.StatusInternalServerError, "failed to trigger workflow")
		return
	}

	// Stamp audit field — engine doesn't have access to actor context.
	if actor := actorFromContext(r.Context()); actor != "" {
		run.CreatedBy = actor
	}

	if len(req.Labels) > 0 {
		if err := s.store.CreateWorkflowRunLabels(r.Context(), run.ID, req.Labels); err != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to persist workflow run labels")
			return
		}
	}
	s.publishWorkflowRunHook(r.Context(), run, domain.WfStatusPending, run.Status, "trigger")

	respondJSON(w, http.StatusCreated, run)
}

func validateWorkflowSteps(steps []workflowStepRequest) error {
	for _, step := range steps {
		if step.StepRef == "" {
			return errors.New("each step requires step_ref")
		}
		if step.StepType == "" {
			step.StepType = domain.WorkflowStepTypeJob
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
		if step.TimeoutSecsOverride < 0 {
			return errors.New("timeout_secs_override must be >= 0")
		}
		if len(step.DependsOn) == 0 {
			continue
		}
		for _, dep := range step.DependsOn {
			if dep == "" {
				return errors.New("depends_on cannot contain empty values")
			}
			if dep == step.StepRef {
				return errors.New("step cannot depend on itself")
			}
		}
	}

	return nil
}

func (s *Server) handleDryRunWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")

	var req dryRunWorkflowRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	if len(req.Steps) == 0 {
		steps, err := s.store.ListStepsByWorkflow(r.Context(), workflowID)
		if err != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to list workflow steps")
			return
		}
		if err := workflow.ValidateDAG(steps); err != nil {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"valid": true, "step_count": len(steps)})
		return
	}

	if err := validateWorkflowSteps(req.Steps); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	steps := make([]domain.WorkflowStep, 0, len(req.Steps))
	for _, sreq := range req.Steps {
		steps = append(steps, domain.WorkflowStep{StepRef: sreq.StepRef, DependsOn: sreq.DependsOn})
	}
	if err := workflow.ValidateDAG(steps); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"valid": true, "step_count": len(steps)})
}

func (s *Server) handleWorkflowGraph(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	format := strings.ToLower(r.URL.Query().Get("format"))

	steps, err := s.store.ListStepsByWorkflow(r.Context(), workflowID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow steps")
		return
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
		respondJSON(w, http.StatusOK, resp)
		return
	}
	resp.Adjacency = adj
	respondJSON(w, http.StatusOK, resp)
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

func (s *Server) handleCloneWorkflow(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "workflowID")

	sourceWf, err := s.store.GetWorkflow(r.Context(), sourceID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow")
		return
	}

	var req cloneWorkflowRequest
	if err := s.decodeJSON(r, &req); err != nil {
		// Body is optional for clone — use defaults.
		req = cloneWorkflowRequest{}
	}

	newName := sourceWf.Name + " (copy)"
	if req.Name != "" {
		newName = req.Name
	}
	newSlug := sourceWf.Slug + "-copy"
	if req.Slug != "" {
		newSlug = req.Slug
	}
	projectID := sourceWf.ProjectID
	if req.ProjectID != "" {
		projectID = req.ProjectID
	}

	newWf := &domain.Workflow{
		ProjectID:         projectID,
		Name:              newName,
		Slug:              newSlug,
		Description:       sourceWf.Description,
		Enabled:           sourceWf.Enabled,
		TimeoutSecs:       sourceWf.TimeoutSecs,
		MaxConcurrentRuns: sourceWf.MaxConcurrentRuns,
		MaxParallelSteps:  sourceWf.MaxParallelSteps,
		Cron:              sourceWf.Cron,
		CronTimezone:      sourceWf.CronTimezone,
		SkipIfRunning:     sourceWf.SkipIfRunning,
	}
	if err := s.store.CreateWorkflow(r.Context(), newWf); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create cloned workflow")
		return
	}

	sourceSteps, err := s.store.ListStepsByWorkflow(r.Context(), sourceID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list source workflow steps")
		return
	}

	newSteps := make([]domain.WorkflowStep, 0, len(sourceSteps))
	for _, src := range sourceSteps {
		step := domain.WorkflowStep{
			WorkflowID:            newWf.ID,
			JobID:                 src.JobID,
			StepRef:               src.StepRef,
			DependsOn:             src.DependsOn,
			Condition:             src.Condition,
			OnFailure:             src.OnFailure,
			Payload:               src.Payload,
			StepType:              src.StepType,
			ApprovalTimeoutSecs:   src.ApprovalTimeoutSecs,
			ApprovalApprovers:     src.ApprovalApprovers,
			RetryMaxAttempts:      src.RetryMaxAttempts,
			RetryBackoff:          src.RetryBackoff,
			RetryInitialDelaySecs: src.RetryInitialDelaySecs,
			RetryMaxDelaySecs:     src.RetryMaxDelaySecs,
			TimeoutSecsOverride:   src.TimeoutSecsOverride,
			OutputTransform:       src.OutputTransform,
			SubWorkflowID:         src.SubWorkflowID,
			MaxNestingDepth:       src.MaxNestingDepth,
		}
		if err := s.store.CreateWorkflowStep(r.Context(), &step); err != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to create cloned workflow step")
			return
		}
		newSteps = append(newSteps, step)
	}

	if err := s.store.CreateWorkflowVersionSnapshot(r.Context(), newWf.ID, newWf.Version); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to snapshot cloned workflow version")
		return
	}

	respondJSON(w, http.StatusCreated, workflowResponse{Workflow: newWf, Steps: newSteps})
}
