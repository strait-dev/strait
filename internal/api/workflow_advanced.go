package api

import (
	"net/http"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

type upsertWorkflowPolicyRequest struct {
	MaxFanOut                int      `json:"max_fan_out"`
	MaxDepth                 int      `json:"max_depth"`
	ForbiddenStepTypes       []string `json:"forbidden_step_types"`
	RequireApprovalForDeploy bool     `json:"require_approval_for_deploy"`
}

func (s *Server) handleUpsertWorkflowPolicy(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	var req upsertWorkflowPolicyRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	policy := &domain.WorkflowPolicy{
		ProjectID:                projectID,
		MaxFanOut:                req.MaxFanOut,
		MaxDepth:                 req.MaxDepth,
		ForbiddenStepTypes:       req.ForbiddenStepTypes,
		RequireApprovalForDeploy: req.RequireApprovalForDeploy,
	}
	if err := s.store.UpsertWorkflowPolicy(r.Context(), policy); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to save workflow policy")
		return
	}
	respondJSON(w, http.StatusOK, policy)
}

func (s *Server) handleGetWorkflowPolicy(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	policy, err := s.store.GetWorkflowPolicyByProject(r.Context(), projectID)
	if err != nil || policy == nil {
		respondError(w, r, http.StatusNotFound, "workflow policy not found")
		return
	}
	respondJSON(w, http.StatusOK, policy)
}

func (s *Server) handleSimulateWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	wf, err := s.store.GetWorkflow(r.Context(), workflowID)
	if err != nil {
		respondError(w, r, http.StatusNotFound, "workflow not found")
		return
	}
	steps, err := s.store.ListStepsByWorkflowVersion(r.Context(), workflowID, wf.Version)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to load workflow steps")
		return
	}
	order := make([]string, 0, len(steps))
	for _, st := range steps {
		order = append(order, st.StepRef)
	}
	respondJSON(w, http.StatusOK, map[string]any{"workflow_id": workflowID, "version": wf.Version, "predicted_order": order, "step_count": len(order)})
}
