package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"
	"orchestrator/internal/workflow"

	"github.com/go-chi/chi/v5"
	"github.com/robfig/cron/v3"
)

type workflowStepRequest struct {
	JobID                 string                    `json:"job_id,omitempty"`
	StepRef               string                    `json:"step_ref"`
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
}

type createWorkflowRequest struct {
	ProjectID         string                `json:"project_id"`
	Name              string                `json:"name"`
	Slug              string                `json:"slug"`
	Description       string                `json:"description,omitempty"`
	Enabled           *bool                 `json:"enabled,omitempty"`
	TimeoutSecs       int                   `json:"timeout_secs,omitempty"`
	MaxConcurrentRuns int                   `json:"max_concurrent_runs,omitempty"`
	MaxParallelSteps  int                   `json:"max_parallel_steps,omitempty"`
	Cron              string                `json:"cron,omitempty"`
	CronTimezone      string                `json:"cron_timezone,omitempty"`
	SkipIfRunning     bool                  `json:"skip_if_running,omitempty"`
	Steps             []workflowStepRequest `json:"steps,omitempty"`
}

type updateWorkflowRequest struct {
	Name              *string                `json:"name,omitempty"`
	Slug              *string                `json:"slug,omitempty"`
	Description       *string                `json:"description,omitempty"`
	Enabled           *bool                  `json:"enabled,omitempty"`
	TimeoutSecs       *int                   `json:"timeout_secs,omitempty"`
	MaxConcurrentRuns *int                   `json:"max_concurrent_runs,omitempty"`
	MaxParallelSteps  *int                   `json:"max_parallel_steps,omitempty"`
	Cron              *string                `json:"cron,omitempty"`
	CronTimezone      *string                `json:"cron_timezone,omitempty"`
	SkipIfRunning     *bool                  `json:"skip_if_running,omitempty"`
	Steps             *[]workflowStepRequest `json:"steps,omitempty"`
}

type dryRunWorkflowRequest struct {
	Steps []workflowStepRequest `json:"steps"`
}

type workflowGraphResponse struct {
	WorkflowID string              `json:"workflow_id"`
	Roots      []string            `json:"roots"`
	Adjacency  map[string][]string `json:"adjacency,omitempty"`
	DOT        string              `json:"dot,omitempty"`
}

type triggerWorkflowRequest struct {
	ProjectID   string            `json:"project_id,omitempty"`
	Payload     json.RawMessage   `json:"payload,omitempty"`
	TriggeredBy string            `json:"triggered_by,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type workflowResponse struct {
	*domain.Workflow
	Steps []domain.WorkflowStep `json:"steps"`
}

func (s *Server) handleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
	var req createWorkflowRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ProjectID == "" || req.Name == "" || req.Slug == "" {
		respondError(w, http.StatusBadRequest, "missing required fields")
		return
	}

	if err := validateWorkflowSteps(req.Steps); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	wf := &domain.Workflow{
		ProjectID:         req.ProjectID,
		Name:              req.Name,
		Slug:              req.Slug,
		Description:       req.Description,
		Enabled:           enabled,
		TimeoutSecs:       req.TimeoutSecs,
		MaxConcurrentRuns: req.MaxConcurrentRuns,
		MaxParallelSteps:  req.MaxParallelSteps,
		Cron:              req.Cron,
		CronTimezone:      req.CronTimezone,
		SkipIfRunning:     req.SkipIfRunning,
	}
	if err := validateWorkflowConfig(wf.Cron, wf.CronTimezone, wf.MaxParallelSteps); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.CreateWorkflow(r.Context(), wf); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create workflow")
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
		}
		if err := s.store.CreateWorkflowStep(r.Context(), &step); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to create workflow step")
			return
		}
		steps = append(steps, step)
	}

	if err := s.store.CreateWorkflowVersionSnapshot(r.Context(), wf.ID, wf.Version); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to snapshot workflow version")
		return
	}

	respondJSON(w, http.StatusCreated, workflowResponse{Workflow: wf, Steps: steps})
}

func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	wf, err := s.store.GetWorkflow(r.Context(), workflowID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			respondError(w, http.StatusNotFound, "workflow not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get workflow")
		return
	}

	steps, err := s.store.ListStepsByWorkflow(r.Context(), wf.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list workflow steps")
		return
	}

	respondJSON(w, http.StatusOK, workflowResponse{Workflow: wf, Steps: steps})
}

func (s *Server) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	workflows, err := s.store.ListWorkflows(r.Context(), projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list workflows")
		return
	}

	respondJSON(w, http.StatusOK, workflows)
}

func (s *Server) handleUpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	wf, err := s.store.GetWorkflow(r.Context(), workflowID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			respondError(w, http.StatusNotFound, "workflow not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get workflow")
		return
	}

	var req updateWorkflowRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
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
	if err := validateWorkflowConfig(wf.Cron, wf.CronTimezone, wf.MaxParallelSteps); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.UpdateWorkflow(r.Context(), wf); err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			respondError(w, http.StatusNotFound, "workflow not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to update workflow")
		return
	}

	if req.Steps != nil {
		if err := validateWorkflowSteps(*req.Steps); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.store.DeleteStepsByWorkflow(r.Context(), wf.ID); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to replace workflow steps")
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
			}
			if err := s.store.CreateWorkflowStep(r.Context(), step); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to create workflow step")
				return
			}
		}
	}

	if err := s.store.CreateWorkflowVersionSnapshot(r.Context(), wf.ID, wf.Version); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to snapshot workflow version")
		return
	}

	steps, err := s.store.ListStepsByWorkflow(r.Context(), wf.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list workflow steps")
		return
	}

	respondJSON(w, http.StatusOK, workflowResponse{Workflow: wf, Steps: steps})
}

func (s *Server) handleDeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	if err := s.store.DeleteWorkflow(r.Context(), workflowID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete workflow")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}

func (s *Server) handleTriggerWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.workflowEngine == nil {
		respondError(w, http.StatusServiceUnavailable, "workflow engine unavailable")
		return
	}

	workflowID := chi.URLParam(r, "workflowID")
	wf, err := s.store.GetWorkflow(r.Context(), workflowID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			respondError(w, http.StatusNotFound, "workflow not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get workflow")
		return
	}
	if wf == nil {
		respondError(w, http.StatusNotFound, "workflow not found")
		return
	}
	if !wf.Enabled {
		respondError(w, http.StatusConflict, "workflow is disabled")
		return
	}

	var req triggerWorkflowRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	triggeredBy := req.TriggeredBy
	if triggeredBy == "" {
		triggeredBy = domain.TriggerManual
	}

	run, err := s.workflowEngine.TriggerWorkflow(r.Context(), workflowID, req.ProjectID, req.Payload, triggeredBy)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			respondError(w, http.StatusNotFound, "workflow not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to trigger workflow")
		return
	}
	if len(req.Labels) > 0 {
		if err := s.store.CreateWorkflowRunLabels(r.Context(), run.ID, req.Labels); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to persist workflow run labels")
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
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Steps) == 0 {
		steps, err := s.store.ListStepsByWorkflow(r.Context(), workflowID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to list workflow steps")
			return
		}
		if err := workflow.ValidateDAG(steps); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"valid": true, "step_count": len(steps)})
		return
	}

	if err := validateWorkflowSteps(req.Steps); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	steps := make([]domain.WorkflowStep, 0, len(req.Steps))
	for _, sreq := range req.Steps {
		steps = append(steps, domain.WorkflowStep{StepRef: sreq.StepRef, DependsOn: sreq.DependsOn})
	}
	if err := workflow.ValidateDAG(steps); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"valid": true, "step_count": len(steps)})
}

func (s *Server) handleWorkflowGraph(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	format := strings.ToLower(r.URL.Query().Get("format"))

	steps, err := s.store.ListStepsByWorkflow(r.Context(), workflowID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list workflow steps")
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
	keys := make([]string, 0, len(adjacency))
	for k := range adjacency {
		keys = append(keys, k)
	}
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
