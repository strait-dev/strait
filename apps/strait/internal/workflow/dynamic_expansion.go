package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"strait/internal/domain"
	storepkg "strait/internal/store"

	"github.com/google/uuid"
)

const (
	maxDynamicExpansionSteps = 100
	maxWorkflowRuntimeSteps  = 1000
)

type dynamicWorkflowStepRequest struct {
	AgentID                 string                    `json:"agent_id,omitempty"`
	JobID                   string                    `json:"job_id,omitempty"`
	StepRef                 string                    `json:"step_ref"`
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

type dynamicWorkflowEnvelope struct {
	DynamicSteps []dynamicWorkflowStepRequest `json:"dynamic_steps"`
}

func (s *StepCallback) parseDynamicWorkflowExpansion(
	ctx context.Context,
	stepRun *domain.WorkflowStepRun,
	wc *wfCtx,
	output json.RawMessage,
) ([]storepkg.DynamicWorkflowExpansion, error) {
	requests, found, err := parseDynamicWorkflowStepRequests(output)
	if err != nil || !found {
		return nil, err
	}
	if len(requests) == 0 {
		return nil, nil
	}
	if len(requests) > maxDynamicExpansionSteps {
		return nil, fmt.Errorf("dynamic_steps cannot contain more than %d steps", maxDynamicExpansionSteps)
	}
	if len(wc.steps)+len(requests) > maxWorkflowRuntimeSteps {
		return nil, fmt.Errorf("workflow cannot have more than %d runtime steps", maxWorkflowRuntimeSteps)
	}

	if err := validateDynamicWorkflowStepRequests(requests, wc.stepByRef); err != nil {
		return nil, err
	}

	resolvedRequests, err := s.resolveDynamicWorkflowStepRequests(ctx, wc.run.ProjectID, requests)
	if err != nil {
		return nil, err
	}
	steps := dynamicWorkflowStepsFromRequests(resolvedRequests)

	combinedSteps := append(slices.Clone(wc.steps), steps...)
	if err := ValidateDAG(combinedSteps); err != nil {
		return nil, fmt.Errorf("dynamic workflow expansion is invalid: %w", err)
	}

	statuses, err := s.store.ListStepRunStatusesByWorkflowRun(ctx, stepRun.WorkflowRunID)
	if err != nil {
		return nil, fmt.Errorf("list step run statuses for dynamic expansion: %w", err)
	}

	expansions := make([]storepkg.DynamicWorkflowExpansion, 0, len(steps))
	for _, step := range steps {
		depsCompleted := advancedDependencyCount(step, stepRun.StepRef, wc, statuses)
		status := domain.StepWaiting
		if depsCompleted == len(step.DependsOn) {
			status = domain.StepPending
		}

		expansions = append(expansions, storepkg.DynamicWorkflowExpansion{
			Step: step,
			StepRun: domain.WorkflowStepRun{
				ID:            uuid.Must(uuid.NewV7()).String(),
				WorkflowRunID: stepRun.WorkflowRunID,
				StepRef:       step.StepRef,
				Status:        status,
				Attempt:       1,
				DepsCompleted: depsCompleted,
				DepsRequired:  len(step.DependsOn),
			},
		})
	}

	return expansions, nil
}

func (s *StepCallback) applyDynamicWorkflowExpansion(
	ctx context.Context,
	stepRun *domain.WorkflowStepRun,
	wc *wfCtx,
) error {
	expansions, err := s.parseDynamicWorkflowExpansion(ctx, stepRun, wc, stepRun.Output)
	if err != nil {
		return err
	}
	return s.persistDynamicWorkflowExpansion(ctx, stepRun, wc, expansions)
}

func (s *StepCallback) persistDynamicWorkflowExpansion(
	ctx context.Context,
	stepRun *domain.WorkflowStepRun,
	wc *wfCtx,
	expansions []storepkg.DynamicWorkflowExpansion,
) error {
	if len(expansions) == 0 {
		return nil
	}
	if err := s.store.CreateWorkflowDynamicExpansion(ctx, stepRun.WorkflowRunID, stepRun.ID, expansions); err != nil {
		return fmt.Errorf("create workflow dynamic expansion: %w", err)
	}
	for _, expansion := range expansions {
		wc.steps = append(wc.steps, expansion.Step)
		wc.stepByRef[expansion.Step.StepRef] = expansion.Step
	}
	return nil
}

func parseDynamicWorkflowStepRequests(output json.RawMessage) ([]dynamicWorkflowStepRequest, bool, error) {
	if len(output) == 0 {
		return nil, false, nil
	}

	var raw any
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, false, fmt.Errorf("parse dynamic expansion output: %w", err)
	}

	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, false, nil
	}
	if _, exists := obj["dynamic_steps"]; !exists {
		return nil, false, nil
	}

	var envelope dynamicWorkflowEnvelope
	if err := json.Unmarshal(output, &envelope); err != nil {
		return nil, true, fmt.Errorf("parse dynamic_steps: %w", err)
	}

	return envelope.DynamicSteps, true, nil
}

func validateDynamicWorkflowStepRequests(
	requests []dynamicWorkflowStepRequest,
	existingSteps map[string]domain.WorkflowStep,
) error {
	knownRefs := make(map[string]struct{}, len(existingSteps)+len(requests))
	for ref := range existingSteps {
		knownRefs[ref] = struct{}{}
	}

	newRefs := make(map[string]struct{}, len(requests))
	for _, step := range requests {
		if step.StepRef == "" {
			return errors.New("dynamic steps require step_ref")
		}
		if _, exists := knownRefs[step.StepRef]; exists {
			return fmt.Errorf("dynamic step_ref %q already exists", step.StepRef)
		}
		if _, exists := newRefs[step.StepRef]; exists {
			return fmt.Errorf("duplicate dynamic step_ref: %s", step.StepRef)
		}
		newRefs[step.StepRef] = struct{}{}
		knownRefs[step.StepRef] = struct{}{}
	}

	for _, step := range requests {
		if err := validateDynamicWorkflowStepShape(step); err != nil {
			return err
		}
		if err := validateDynamicWorkflowStepDependencies(step, knownRefs); err != nil {
			return err
		}
	}

	return nil
}

func validateDynamicWorkflowStepShape(step dynamicWorkflowStepRequest) error {
	stepType := step.StepType
	if stepType == "" {
		stepType = domain.WorkflowStepTypeJob
	}

	if stepType == domain.WorkflowStepTypeJob && step.JobID == "" && step.AgentID == "" {
		return errors.New("job steps require job_id or agent_id")
	}
	if stepType != domain.WorkflowStepTypeJob && step.AgentID != "" {
		return fmt.Errorf("%s steps must not have agent_id", stepType)
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

	switch stepType {
	case domain.WorkflowStepTypeJob:
		return nil
	case domain.WorkflowStepTypeApproval:
		if len(step.ApprovalApprovers) == 0 {
			return errors.New("approval steps require approval_approvers")
		}
		if step.ApprovalTimeoutSecs < 0 {
			return errors.New("approval_timeout_secs must be >= 0")
		}
	case domain.WorkflowStepTypeSubWorkflow:
		if step.SubWorkflowID == "" {
			return errors.New("sub_workflow steps require sub_workflow_id")
		}
		if step.JobID != "" {
			return errors.New("sub_workflow steps must not have job_id")
		}
		if step.MaxNestingDepth < 0 {
			return errors.New("max_nesting_depth must be >= 0")
		}
	case domain.WorkflowStepTypeWaitForEvent:
		if step.EventKey == "" {
			return errors.New("wait_for_event steps require event_key")
		}
		if len(step.EventKey) > 512 {
			return errors.New("event_key must be at most 512 characters")
		}
	case domain.WorkflowStepTypeSleep:
		if step.SleepDurationSecs <= 0 {
			return errors.New("sleep steps require sleep_duration_secs > 0")
		}
	default:
		return nil
	}

	return nil
}

func validateDynamicWorkflowStepDependencies(step dynamicWorkflowStepRequest, knownRefs map[string]struct{}) error {
	for _, dep := range step.DependsOn {
		if dep == "" {
			return errors.New("depends_on cannot contain empty values")
		}
		if dep == step.StepRef {
			return errors.New("step cannot depend on itself")
		}
		if _, exists := knownRefs[dep]; !exists {
			return fmt.Errorf("step %q depends on unknown step %q", step.StepRef, dep)
		}
	}
	return nil
}

func (s *StepCallback) resolveDynamicWorkflowStepRequests(
	ctx context.Context,
	projectID string,
	requests []dynamicWorkflowStepRequest,
) ([]dynamicWorkflowStepRequest, error) {
	resolved := make([]dynamicWorkflowStepRequest, 0, len(requests))
	for _, stepReq := range requests {
		resolvedStep := stepReq
		if resolvedStep.AgentID != "" {
			agent, err := s.store.GetAgent(ctx, resolvedStep.AgentID)
			if err != nil {
				return nil, fmt.Errorf("resolve dynamic step agent %q: %w", resolvedStep.AgentID, err)
			}
			if agent == nil || agent.ProjectID != projectID {
				return nil, fmt.Errorf("dynamic step %q references unknown agent_id %q", resolvedStep.StepRef, resolvedStep.AgentID)
			}
			if resolvedStep.JobID != "" && resolvedStep.JobID != agent.JobID {
				return nil, fmt.Errorf("dynamic step %q job_id %q does not match agent_id %q", resolvedStep.StepRef, resolvedStep.JobID, resolvedStep.AgentID)
			}
			resolvedStep.JobID = agent.JobID
		}
		resolved = append(resolved, resolvedStep)
	}
	return resolved, nil
}

func dynamicWorkflowStepsFromRequests(requests []dynamicWorkflowStepRequest) []domain.WorkflowStep {
	steps := make([]domain.WorkflowStep, 0, len(requests))
	for _, req := range requests {
		stepType := req.StepType
		if stepType == "" {
			stepType = domain.WorkflowStepTypeJob
		}
		steps = append(steps, domain.WorkflowStep{
			JobID:                   req.JobID,
			StepRef:                 req.StepRef,
			DependsOn:               slices.Clone(req.DependsOn),
			Condition:               req.Condition,
			OnFailure:               req.OnFailure,
			Payload:                 req.Payload,
			StepType:                stepType,
			ApprovalTimeoutSecs:     req.ApprovalTimeoutSecs,
			ApprovalApprovers:       slices.Clone(req.ApprovalApprovers),
			RetryMaxAttempts:        req.RetryMaxAttempts,
			RetryBackoff:            req.RetryBackoff,
			RetryInitialDelaySecs:   req.RetryInitialDelaySecs,
			RetryMaxDelaySecs:       req.RetryMaxDelaySecs,
			TimeoutSecsOverride:     req.TimeoutSecsOverride,
			OutputTransform:         req.OutputTransform,
			SubWorkflowID:           req.SubWorkflowID,
			MaxNestingDepth:         req.MaxNestingDepth,
			EventKey:                req.EventKey,
			EventTimeoutSecs:        req.EventTimeoutSecs,
			EventNotifyURL:          req.EventNotifyURL,
			SleepDurationSecs:       req.SleepDurationSecs,
			EventEmitKey:            req.EventEmitKey,
			ConcurrencyKey:          req.ConcurrencyKey,
			ResourceClass:           req.ResourceClass,
			ExpectedDurationSecs:    req.ExpectedDurationSecs,
			StageNotifications:      req.StageNotifications,
			CompensationJobID:       req.CompensationJobID,
			CompensationTimeoutSecs: req.CompensationTimeoutSecs,
		})
	}
	return steps
}

func advancedDependencyCount(
	step domain.WorkflowStep,
	currentStepRef string,
	wc *wfCtx,
	statuses map[string]domain.StepRunStatus,
) int {
	count := 0
	for _, dep := range step.DependsOn {
		if dep == currentStepRef {
			continue
		}
		if dependencyAdvanced(dep, wc, statuses) {
			count++
		}
	}
	return count
}

func dependencyAdvanced(dep string, wc *wfCtx, statuses map[string]domain.StepRunStatus) bool {
	status, ok := statuses[dep]
	if !ok {
		return false
	}
	if status == domain.StepCompleted {
		return true
	}
	if status == domain.StepFailed {
		step, exists := wc.stepByRef[dep]
		if exists && step.OnFailure == domain.Continue {
			return true
		}
	}
	return false
}
