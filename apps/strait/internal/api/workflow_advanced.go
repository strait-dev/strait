package api

import (
	"context"
	"sort"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

type upsertWorkflowPolicyRequest struct {
	MaxFanOut                int      `json:"max_fan_out"`
	MaxDepth                 int      `json:"max_depth"`
	ForbiddenStepTypes       []string `json:"forbidden_step_types"`
	RequireApprovalForDeploy bool     `json:"require_approval_for_deploy"`
}

type UpsertWorkflowPolicyInput struct {
	ProjectID string `path:"projectID"`
	Body      upsertWorkflowPolicyRequest
}

type UpsertWorkflowPolicyOutput struct {
	Body *domain.WorkflowPolicy
}

func (s *Server) handleUpsertWorkflowPolicy(ctx context.Context, input *UpsertWorkflowPolicyInput) (*UpsertWorkflowPolicyOutput, error) {
	if err := requireProjectMatch(ctx, input.ProjectID); err != nil {
		return nil, huma.Error404NotFound("not found")
	}
	if actorTypeFromContext(ctx) == "api_key" && !isInternalCaller(ctx) {
		return nil, huma.Error403Forbidden("workflow policy changes require an operator or user context")
	}
	if !isInternalCaller(ctx) && !s.hasProjectPermission(ctx, domain.ScopeRBACManage) {
		return nil, huma.Error403Forbidden("workflow policy changes require rbac:manage")
	}
	if err := s.checkRBACLevel(ctx, input.ProjectID, "advanced", "Workflow policies"); err != nil {
		return nil, err
	}
	policy := &domain.WorkflowPolicy{
		ProjectID:                input.ProjectID,
		MaxFanOut:                input.Body.MaxFanOut,
		MaxDepth:                 input.Body.MaxDepth,
		ForbiddenStepTypes:       input.Body.ForbiddenStepTypes,
		RequireApprovalForDeploy: input.Body.RequireApprovalForDeploy,
	}
	if err := s.store.UpsertWorkflowPolicy(ctx, policy); err != nil {
		return nil, huma.Error500InternalServerError("failed to save workflow policy")
	}
	s.emitAuditEvent(ctx, domain.AuditActionWorkflowPolicyUpserted, "workflow_policy", input.ProjectID, map[string]any{
		"project_id":                  input.ProjectID,
		"max_fan_out":                 input.Body.MaxFanOut,
		"max_depth":                   input.Body.MaxDepth,
		"forbidden_step_types":        input.Body.ForbiddenStepTypes,
		"require_approval_for_deploy": input.Body.RequireApprovalForDeploy,
	})
	return &UpsertWorkflowPolicyOutput{Body: policy}, nil
}

type GetWorkflowPolicyInput struct {
	ProjectID string `path:"projectID"`
}

type GetWorkflowPolicyOutput struct {
	Body *domain.WorkflowPolicy
}

func (s *Server) handleGetWorkflowPolicy(ctx context.Context, input *GetWorkflowPolicyInput) (*GetWorkflowPolicyOutput, error) {
	if err := requireProjectMatch(ctx, input.ProjectID); err != nil {
		return nil, huma.Error404NotFound("not found")
	}
	if err := s.checkRBACLevel(ctx, input.ProjectID, "advanced", "Workflow policies"); err != nil {
		return nil, err
	}
	policy, err := s.store.GetWorkflowPolicyByProject(ctx, input.ProjectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get workflow policy")
	}
	if policy == nil {
		return nil, huma.Error404NotFound("workflow policy not found")
	}
	return &GetWorkflowPolicyOutput{Body: policy}, nil
}

type SimulateWorkflowInput struct {
	WorkflowID string `path:"workflowID"`
}

type SimulateWorkflowOutput struct {
	Body map[string]any
}

func (s *Server) handleSimulateWorkflow(ctx context.Context, input *SimulateWorkflowInput) (*SimulateWorkflowOutput, error) {
	wf, err := s.store.GetWorkflow(ctx, input.WorkflowID)
	if err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	if err := requireProjectMatch(ctx, wf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	steps, err := s.store.ListStepsByWorkflowVersion(ctx, input.WorkflowID, wf.Version)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load workflow steps")
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

	return &SimulateWorkflowOutput{
		Body: map[string]any{"workflow_id": input.WorkflowID, "version": wf.Version, "predicted_order": order, "step_count": len(order)},
	}, nil
}
