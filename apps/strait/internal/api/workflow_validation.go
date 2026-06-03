package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/samber/lo"

	"strait/internal/domain"
)

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

func (s *Server) validateWorkflowPolicy(ctx context.Context, projectID string, steps []domain.WorkflowStep) error {
	policy, err := s.store.GetWorkflowPolicyByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("load workflow policy: %w", err)
	}
	if policy == nil {
		return nil
	}

	if err := validateWorkflowPolicyFanOut(policy, steps); err != nil {
		return err
	}
	if err := validateWorkflowPolicyDepth(policy, steps); err != nil {
		return err
	}
	if err := validateWorkflowPolicyForbiddenTypes(policy, steps); err != nil {
		return err
	}
	if err := validateWorkflowPolicyDeployApproval(policy, steps); err != nil {
		return err
	}

	return nil
}

func validateWorkflowPolicyFanOut(policy *domain.WorkflowPolicy, steps []domain.WorkflowStep) error {
	if policy.MaxFanOut <= 0 {
		return nil
	}
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
	return nil
}

func validateWorkflowPolicyDepth(policy *domain.WorkflowPolicy, steps []domain.WorkflowStep) error {
	if policy.MaxDepth <= 0 {
		return nil
	}

	stepByRef := lo.KeyBy(steps, func(step domain.WorkflowStep) string { return step.StepRef })
	memo := make(map[string]int, len(steps))
	visiting := make(map[string]bool, len(steps))

	maxDepth := 0
	for _, step := range steps {
		depth, err := workflowStepDepth(step.StepRef, stepByRef, memo, visiting)
		if err != nil {
			return fmt.Errorf("workflow policy violation: %w", err)
		}
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	if maxDepth > policy.MaxDepth {
		return fmt.Errorf("workflow policy violation: workflow depth %d exceeds max_depth %d", maxDepth, policy.MaxDepth)
	}
	return nil
}

func workflowStepDepth(
	ref string,
	stepByRef map[string]domain.WorkflowStep,
	memo map[string]int,
	visiting map[string]bool,
) (int, error) {
	if depth, ok := memo[ref]; ok {
		return depth, nil
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
		depDepth, err := workflowStepDepth(dep, stepByRef, memo, visiting)
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

func validateWorkflowPolicyForbiddenTypes(policy *domain.WorkflowPolicy, steps []domain.WorkflowStep) error {
	if len(policy.ForbiddenStepTypes) == 0 {
		return nil
	}
	forbidden := make(map[string]struct{}, len(policy.ForbiddenStepTypes))
	for _, stepType := range policy.ForbiddenStepTypes {
		forbidden[strings.ToLower(strings.TrimSpace(stepType))] = struct{}{}
	}
	for _, step := range steps {
		stepType := normalizedWorkflowStepType(step.StepType)
		if _, blocked := forbidden[strings.ToLower(string(stepType))]; blocked {
			return fmt.Errorf("workflow policy violation: step %s uses forbidden step_type %s", step.StepRef, stepType)
		}
	}
	return nil
}

func validateWorkflowPolicyDeployApproval(policy *domain.WorkflowPolicy, steps []domain.WorkflowStep) error {
	if !policy.RequireApprovalForDeploy {
		return nil
	}
	hasApproval := lo.ContainsBy(steps, func(step domain.WorkflowStep) bool {
		return step.StepType == domain.WorkflowStepTypeApproval
	})
	hasDeployLikeStep := lo.ContainsBy(steps, func(step domain.WorkflowStep) bool {
		if normalizedWorkflowStepType(step.StepType) != domain.WorkflowStepTypeJob {
			return false
		}
		ref := strings.ToLower(step.StepRef)
		return strings.Contains(ref, "deploy") || strings.Contains(ref, "release")
	})
	if hasDeployLikeStep && !hasApproval {
		return fmt.Errorf("workflow policy violation: approval step is required for deploy-like workflows")
	}
	return nil
}

func normalizedWorkflowStepType(stepType domain.WorkflowStepType) domain.WorkflowStepType {
	if stepType == "" {
		return domain.WorkflowStepTypeJob
	}
	return stepType
}

const maxWorkflowSteps = 1000

func validateWorkflowSteps(steps []workflowStepRequest) error {
	if len(steps) > maxWorkflowSteps {
		return fmt.Errorf("workflow cannot have more than %d steps", maxWorkflowSteps)
	}

	knownRefs, err := workflowStepRefs(steps)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if err := validateWorkflowStepFields(step, knownRefs); err != nil {
			return err
		}
	}
	return validateWorkflowStepAcyclic(steps)
}

func workflowStepRefs(steps []workflowStepRequest) (map[string]bool, error) {
	knownRefs := make(map[string]bool, len(steps))
	for _, step := range steps {
		if step.StepRef == "" {
			return nil, errors.New("each step requires step_ref")
		}
		if knownRefs[step.StepRef] {
			return nil, fmt.Errorf("duplicate step_ref: %s", step.StepRef)
		}
		knownRefs[step.StepRef] = true
	}
	return knownRefs, nil
}

func validateWorkflowStepFields(step workflowStepRequest, knownRefs map[string]bool) error {
	stepType := normalizedWorkflowStepType(step.StepType)
	if !isValidWorkflowStepType(stepType) {
		return fmt.Errorf("step %s has invalid step_type %q", step.StepRef, stepType)
	}
	if err := validateWorkflowStepTypeFields(step, stepType); err != nil {
		return err
	}
	if step.EventNotifyURL != "" {
		if err := validateURL(step.EventNotifyURL); err != nil {
			return fmt.Errorf("step %s has invalid event_notify_url: %w", step.StepRef, err)
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
	return validateWorkflowStepDependencies(step, knownRefs)
}

func validateWorkflowStepTypeFields(step workflowStepRequest, stepType domain.WorkflowStepType) error {
	switch stepType {
	case domain.WorkflowStepTypeJob:
		if step.JobID == "" {
			return errors.New("job steps require job_id")
		}
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
		if step.SleepDurationSecs > domain.MaxSleepDurationSecs {
			return fmt.Errorf("sleep_duration_secs must be <= %d", domain.MaxSleepDurationSecs)
		}
	}
	return nil
}

func validateWorkflowStepDependencies(step workflowStepRequest, knownRefs map[string]bool) error {
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
	return nil
}

func validateWorkflowStepAcyclic(steps []workflowStepRequest) error {
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
