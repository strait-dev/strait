package workflow

import (
	"encoding/json"
	"fmt"

	"strait/internal/domain"
)

type stepStatusCondition struct {
	Type    string `json:"type"`
	StepRef string `json:"step_ref"`
	Status  string `json:"status"`
}

type compositeCondition struct {
	Type       string            `json:"type"`
	Conditions []json.RawMessage `json:"conditions"`
}

type stepStatusInCondition struct {
	Type     string   `json:"type"`
	StepRef  string   `json:"step_ref"`
	Statuses []string `json:"statuses"`
}

type notCondition struct {
	Type      string          `json:"type"`
	Condition json.RawMessage `json:"condition"`
}

// EvaluateCondition evaluates a workflow step condition against current step statuses.
// Returns true if the condition is met and the step should run.
// Returns true if cond is nil or empty (unconditional step).
func EvaluateCondition(cond json.RawMessage, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	if len(cond) == 0 {
		return true, nil
	}

	var envelopeData map[string]any
	if err := json.Unmarshal(cond, &envelopeData); err != nil {
		return false, fmt.Errorf("unmarshal condition envelope: %w", err)
	}

	condType, ok := envelopeData["type"].(string)
	if !ok {
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

	case "step_status_in":
		var c stepStatusInCondition
		if err := json.Unmarshal(cond, &c); err != nil {
			return false, fmt.Errorf("unmarshal step_status_in condition: %w", err)
		}
		if c.StepRef == "" {
			return false, fmt.Errorf("step_ref is required for step_status_in condition")
		}
		actualStatus, found := stepStatuses[c.StepRef]
		if !found {
			return false, fmt.Errorf("step %q not found in statuses", c.StepRef)
		}
		for _, allowed := range c.Statuses {
			if actualStatus == domain.StepRunStatus(allowed) {
				return true, nil
			}
		}
		return false, nil

	case "not":
		var c notCondition
		if err := json.Unmarshal(cond, &c); err != nil {
			return false, fmt.Errorf("unmarshal not condition: %w", err)
		}
		if len(c.Condition) == 0 {
			return false, fmt.Errorf("condition is required for not condition")
		}
		ok, err := EvaluateCondition(c.Condition, stepStatuses)
		if err != nil {
			return false, err
		}
		return !ok, nil

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
