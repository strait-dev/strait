package api

import (
	"net/http"
	"sort"

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
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow policy")
		return
	}
	if policy == nil {
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
	// Topological sort via Kahn's algorithm for deterministic predicted order.
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
	for ref, deg := range indegree {
		if deg == 0 {
			queue = append(queue, ref)
		}
	}
	sort.Strings(queue)

	order := make([]string, 0, len(steps))
	for len(queue) > 0 {
		ref := queue[0]
		queue = queue[1:]
		order = append(order, ref)
		for _, dep := range adj[ref] {
			indegree[dep]--
			if indegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
		sort.Strings(queue)
	}

	// Fallback: if cycle detected (should not happen post-validation), use insertion order.
	if len(order) != len(steps) {
		order = order[:0]
		for _, st := range steps {
			order = append(order, st.StepRef)
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{"workflow_id": workflowID, "version": wf.Version, "predicted_order": order, "step_count": len(order)})
}
