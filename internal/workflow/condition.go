package workflow

import (
	"encoding/json"
	"fmt"

	"strait/internal/domain"
)

type conditionEnvelope struct {
	Type string `json:"type"`
}

type stepStatusCondition struct {
	Type    string `json:"type"`
	StepRef string `json:"step_ref"`
	Status  string `json:"status"`
}

type compositeCondition struct {
	Type       string            `json:"type"`
	Conditions []json.RawMessage `json:"conditions"`
}

// EvaluateCondition evaluates a workflow step condition against current step statuses.
// Returns true if the condition is met and the step should run.
// Returns true if cond is nil or empty (unconditional step).
func EvaluateCondition(cond json.RawMessage, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	if len(cond) == 0 {
		return true, nil
	}

	var envelope conditionEnvelope
	if err := json.Unmarshal(cond, &envelope); err != nil {
		return false, fmt.Errorf("unmarshal condition envelope: %w", err)
	}

	condType := envelope.Type
	if condType == "" {
		return false, fmt.Errorf("unknown condition type: %q", "")
	}

	switch condType {
	case "step_status":
		var c stepStatusCondition
		if err := json.Unmarshal(cond, &c); err != nil {
			return false, fmt.Errorf("unmarshal step_status condition: %w", err)
		}
		if c.StepRef == "" {
			return false, fmt.Errorf("step_ref is required for step_status condition")
		}

		actualStatus, found := stepStatuses[c.StepRef]
		if !found {
			return false, fmt.Errorf("step %q not found in statuses", c.StepRef)
		}

		return actualStatus == domain.StepRunStatus(c.Status), nil

	case "all_of":
		var c compositeCondition
		if err := json.Unmarshal(cond, &c); err != nil {
			return false, fmt.Errorf("unmarshal all_of condition: %w", err)
		}

		for _, subCondition := range c.Conditions {
			ok, err := EvaluateCondition(subCondition, stepStatuses)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}

		return true, nil

	case "any_of":
		var c compositeCondition
		if err := json.Unmarshal(cond, &c); err != nil {
			return false, fmt.Errorf("unmarshal any_of condition: %w", err)
		}

		for _, subCondition := range c.Conditions {
			ok, err := EvaluateCondition(subCondition, stepStatuses)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}

		return false, nil

	default:
		return false, fmt.Errorf("unknown condition type: %q", condType)
	}
}
