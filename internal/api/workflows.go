package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
)

type workflowStepRequest struct {
	JobID     string               `json:"job_id"`
	StepRef   string               `json:"step_ref"`
	DependsOn []string             `json:"depends_on,omitempty"`
	Condition json.RawMessage      `json:"condition,omitempty"`
	OnFailure domain.FailurePolicy `json:"on_failure,omitempty"`
	Payload   json.RawMessage      `json:"payload,omitempty"`
}

type createWorkflowRequest struct {
	ProjectID   string                `json:"project_id"`
	Name        string                `json:"name"`
	Slug        string                `json:"slug"`
	Description string                `json:"description,omitempty"`
	Enabled     *bool                 `json:"enabled,omitempty"`
	Steps       []workflowStepRequest `json:"steps,omitempty"`
}

type updateWorkflowRequest struct {
	Name        *string                `json:"name,omitempty"`
	Slug        *string                `json:"slug,omitempty"`
	Description *string                `json:"description,omitempty"`
	Enabled     *bool                  `json:"enabled,omitempty"`
	Steps       *[]workflowStepRequest `json:"steps,omitempty"`
}

type triggerWorkflowRequest struct {
	ProjectID   string          `json:"project_id,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	TriggeredBy string          `json:"triggered_by,omitempty"`
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
		ProjectID:   req.ProjectID,
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		Enabled:     enabled,
	}

	if err := s.store.CreateWorkflow(r.Context(), wf); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create workflow")
		return
	}

	steps := make([]domain.WorkflowStep, 0, len(req.Steps))
	for _, stepReq := range req.Steps {
		step := domain.WorkflowStep{
			WorkflowID: wf.ID,
			JobID:      stepReq.JobID,
			StepRef:    stepReq.StepRef,
			DependsOn:  stepReq.DependsOn,
			Condition:  stepReq.Condition,
			OnFailure:  stepReq.OnFailure,
			Payload:    stepReq.Payload,
		}
		if err := s.store.CreateWorkflowStep(r.Context(), &step); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to create workflow step")
			return
		}
		steps = append(steps, step)
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
				WorkflowID: wf.ID,
				JobID:      stepReq.JobID,
				StepRef:    stepReq.StepRef,
				DependsOn:  stepReq.DependsOn,
				Condition:  stepReq.Condition,
				OnFailure:  stepReq.OnFailure,
				Payload:    stepReq.Payload,
			}
			if err := s.store.CreateWorkflowStep(r.Context(), step); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to create workflow step")
				return
			}
		}
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

	respondJSON(w, http.StatusCreated, run)
}

func validateWorkflowSteps(steps []workflowStepRequest) error {
	for _, step := range steps {
		if step.JobID == "" || step.StepRef == "" {
			return errors.New("each step requires job_id and step_ref")
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
