package domain

// CloneWorkflowSteps copies mutable fields before workflow steps cross cache boundaries.
func CloneWorkflowSteps(steps []WorkflowStep) []WorkflowStep {
	if steps == nil {
		return nil
	}
	out := make([]WorkflowStep, len(steps))
	for i := range steps {
		out[i] = steps[i]
		out[i].DependsOn = append([]string(nil), steps[i].DependsOn...)
		out[i].Condition = append([]byte(nil), steps[i].Condition...)
		out[i].Payload = append([]byte(nil), steps[i].Payload...)
		out[i].ApprovalApprovers = append([]string(nil), steps[i].ApprovalApprovers...)
		out[i].StageNotifications = append([]byte(nil), steps[i].StageNotifications...)
	}
	return out
}
