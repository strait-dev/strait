package workflow

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

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

type stepStatusInCondition struct {
	Type     string   `json:"type"`
	StepRef  string   `json:"step_ref"`
	Statuses []string `json:"statuses"`
}

type notCondition struct {
	Type      string          `json:"type"`
	Condition json.RawMessage `json:"condition"`
}

type binaryCondition struct {
	Type  string `json:"type"`
	Left  any    `json:"left"`
	Right any    `json:"right"`
}

type unaryCondition struct {
	Type    string `json:"type"`
	Operand any    `json:"operand"`
}

// EvaluateCondition evaluates a workflow step condition against current step statuses.
// Returns true if the condition is met and the step should run.
// Returns true if cond is nil or empty (unconditional step).
//
//nolint:gocognit,gocyclo,cyclop,funlen
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

	case "eq", "ne", "gt", "gte", "lt", "lte", "contains", "in", "regex":
		var c binaryCondition
		if err := json.Unmarshal(cond, &c); err != nil {
			return false, fmt.Errorf("unmarshal %s condition: %w", condType, err)
		}
		left := resolveOperand(c.Left, stepStatuses)
		right := resolveOperand(c.Right, stepStatuses)
		switch condType {
		case "eq":
			return fmt.Sprint(left) == fmt.Sprint(right), nil
		case "ne":
			return fmt.Sprint(left) != fmt.Sprint(right), nil
		case "gt", "gte", "lt", "lte":
			lf, lok := asFloat(left)
			rf, rok := asFloat(right)
			if !lok || !rok {
				return false, fmt.Errorf("numeric comparison requires numeric left/right")
			}
			switch condType {
			case "gt":
				return lf > rf, nil
			case "gte":
				return lf >= rf, nil
			case "lt":
				return lf < rf, nil
			default:
				return lf <= rf, nil
			}
		case "contains":
			return strings.Contains(fmt.Sprint(left), fmt.Sprint(right)), nil
		case "in":
			s, ok := right.([]any)
			if !ok {
				return false, fmt.Errorf("in requires right operand array")
			}
			needle := fmt.Sprint(left)
			for _, item := range s {
				if fmt.Sprint(item) == needle {
					return true, nil
				}
			}
			return false, nil
		case "regex":
			re, err := regexp.Compile(fmt.Sprint(right))
			if err != nil {
				return false, fmt.Errorf("invalid regex: %w", err)
			}
			return re.MatchString(fmt.Sprint(left)), nil
		}
		return false, nil

	case "exists":
		var c unaryCondition
		if err := json.Unmarshal(cond, &c); err != nil {
			return false, fmt.Errorf("unmarshal exists condition: %w", err)
		}
		v := resolveOperand(c.Operand, stepStatuses)
		return fmt.Sprint(v) != "", nil

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

func resolveOperand(v any, stepStatuses map[string]domain.StepRunStatus) any {
	obj, ok := v.(map[string]any)
	if !ok {
		return v
	}
	if stepRef, ok := obj["step_ref"].(string); ok {
		if status, found := stepStatuses[stepRef]; found {
			return string(status)
		}
		return ""
	}
	if val, ok := obj["value"]; ok {
		return val
	}
	return v
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
