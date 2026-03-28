package api

import (
	"context"
	"errors"

	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

type ListWorkflowVersionsInput struct {
	WorkflowID string `path:"workflowID"`
	Limit      string `query:"limit"`
	Cursor     string `query:"cursor"`
}

type ListWorkflowVersionsOutput struct {
	Body any
}

func (s *Server) handleListWorkflowVersions(ctx context.Context, input *ListWorkflowVersionsInput) (*ListWorkflowVersionsOutput, error) {
	limit, _, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	versions, err := s.store.ListWorkflowVersions(ctx, input.WorkflowID, limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow versions")
	}
	return &ListWorkflowVersionsOutput{Body: versions}, nil
}

type GetWorkflowVersionInput struct {
	WorkflowID string `path:"workflowID"`
	VersionID  string `path:"versionID"`
}

type GetWorkflowVersionOutput struct {
	Body any
}

func (s *Server) handleGetWorkflowVersion(ctx context.Context, input *GetWorkflowVersionInput) (*GetWorkflowVersionOutput, error) {
	version, err := s.store.GetWorkflowVersionByVersionID(ctx, input.WorkflowID, input.VersionID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowVersionNotFound) {
			return nil, huma.Error404NotFound("workflow version not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow version")
	}
	return &GetWorkflowVersionOutput{Body: version}, nil
}

type ListWorkflowVersionStepsInput struct {
	WorkflowID string `path:"workflowID"`
	VersionID  string `path:"versionID"`
}

type ListWorkflowVersionStepsOutput struct {
	Body any
}

func (s *Server) handleListWorkflowVersionSteps(ctx context.Context, input *ListWorkflowVersionStepsInput) (*ListWorkflowVersionStepsOutput, error) {
	version, err := s.store.GetWorkflowVersionByVersionID(ctx, input.WorkflowID, input.VersionID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowVersionNotFound) {
			return nil, huma.Error404NotFound("workflow version not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow version")
	}
	steps, err := s.store.ListStepsByWorkflowVersion(ctx, input.WorkflowID, version.Version)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow version steps")
	}
	respSteps, err := s.workflowStepResponses(ctx, version.ProjectID, steps)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow version steps")
	}
	return &ListWorkflowVersionStepsOutput{Body: respSteps}, nil
}

type WorkflowVersionDiffInput struct {
	WorkflowID    string `path:"workflowID"`
	FromVersionID string `path:"fromVersionID"`
	ToVersionID   string `path:"toVersionID"`
}

type WorkflowVersionDiffOutput struct {
	Body map[string]any
}

func (s *Server) handleWorkflowVersionDiff(ctx context.Context, input *WorkflowVersionDiffInput) (*WorkflowVersionDiffOutput, error) {
	fromVersion, err := s.store.GetWorkflowVersionByVersionID(ctx, input.WorkflowID, input.FromVersionID)
	if err != nil {
		return nil, huma.Error404NotFound("from workflow version not found")
	}
	toVersion, err := s.store.GetWorkflowVersionByVersionID(ctx, input.WorkflowID, input.ToVersionID)
	if err != nil {
		return nil, huma.Error404NotFound("to workflow version not found")
	}
	fromSteps, err := s.store.ListStepsByWorkflowVersion(ctx, input.WorkflowID, fromVersion.Version)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list from steps")
	}
	toSteps, err := s.store.ListStepsByWorkflowVersion(ctx, input.WorkflowID, toVersion.Version)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list to steps")
	}
	fromMap := map[string]bool{}
	for _, st := range fromSteps {
		fromMap[st.StepRef] = true
	}
	toMap := map[string]bool{}
	for _, st := range toSteps {
		toMap[st.StepRef] = true
	}
	added := []string{}
	removed := []string{}
	for ref := range toMap {
		if !fromMap[ref] {
			added = append(added, ref)
		}
	}
	for ref := range fromMap {
		if !toMap[ref] {
			removed = append(removed, ref)
		}
	}
	return &WorkflowVersionDiffOutput{Body: map[string]any{"from_version_id": input.FromVersionID, "to_version_id": input.ToVersionID, "added_steps": added, "removed_steps": removed}}, nil
}

type WorkflowVersionImpactInput struct {
	WorkflowID string `path:"workflowID"`
	VersionID  string `path:"versionID"`
}

type WorkflowVersionImpactOutput struct {
	Body map[string]any
}

func (s *Server) handleWorkflowVersionImpact(ctx context.Context, input *WorkflowVersionImpactInput) (*WorkflowVersionImpactOutput, error) {
	version, err := s.store.GetWorkflowVersionByVersionID(ctx, input.WorkflowID, input.VersionID)
	if err != nil {
		return nil, huma.Error404NotFound("workflow version not found")
	}
	runs, err := s.store.ListWorkflowRuns(ctx, input.WorkflowID, 500, nil)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow runs")
	}
	pinned := 0
	for _, run := range runs {
		if run.WorkflowVersion == version.Version {
			pinned++
		}
	}
	return &WorkflowVersionImpactOutput{Body: map[string]any{"version_id": input.VersionID, "matching_runs": pinned, "sampled_runs": len(runs)}}, nil
}
