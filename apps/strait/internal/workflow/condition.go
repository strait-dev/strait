package workflow

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"strait/internal/domain"

	"github.com/tidwall/gjson"
)

const (
	// maxRegexPatternLen limits user-supplied regex patterns to prevent
	// catastrophic backtracking with complex patterns.
	maxRegexPatternLen = 1000

	// maxRegexInputLen limits the input string matched against a regex
	// to prevent excessive CPU usage on pathological inputs.
	maxRegexInputLen = 10000

	maxConditionRegexCacheEntries = 128
)

var conditionRegexCache = struct {
	sync.RWMutex
	compiled map[string]*regexp.Regexp
}{
	compiled: make(map[string]*regexp.Regexp),
}

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
func EvaluateCondition(cond json.RawMessage, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	if len(cond) == 0 {
		return true, nil
	}
	condType, err := conditionType(cond)
	if err != nil {
		return false, err
	}

	switch condType {
	case "step_status":
		return evaluateStepStatusCondition(cond, stepStatuses)
	case "step_status_in":
		return evaluateStepStatusInCondition(cond, stepStatuses)
	case "not":
		return evaluateNotCondition(cond, stepStatuses)
	case "all_of":
		return evaluateAllOfCondition(cond, stepStatuses)
	case "any_of":
		return evaluateAnyOfCondition(cond, stepStatuses)
	case "eq", "ne", "gt", "gte", "lt", "lte", "contains", "in", "regex":
		return evaluateBinaryCondition(condType, cond, stepStatuses)
	case "exists":
		return evaluateExistsCondition(cond, stepStatuses)
	default:
		return false, fmt.Errorf("unknown condition type: %q", condType)
	}
}

func conditionType(cond json.RawMessage) (string, error) {
	if !gjson.ValidBytes(cond) {
		var envelope conditionEnvelope
		if err := json.Unmarshal(cond, &envelope); err != nil {
			return "", fmt.Errorf("unmarshal condition envelope: %w", err)
		}
	}

	condType := gjson.GetBytes(cond, "type").String()
	if condType == "" {
		return "", fmt.Errorf("unknown condition type: %q", "")
	}
	return condType, nil
}

func evaluateStepStatusCondition(cond json.RawMessage, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	stepRefValue := gjson.GetBytes(cond, "step_ref")
	statusValue := gjson.GetBytes(cond, "status")
	invalidShape := stepRefValue.Exists() && stepRefValue.Type != gjson.String
	invalidShape = invalidShape || (statusValue.Exists() && statusValue.Type != gjson.String)
	if invalidShape {
		var c stepStatusCondition
		if err := json.Unmarshal(cond, &c); err != nil {
			return false, fmt.Errorf("unmarshal step_status condition: %w", err)
		}
	}

	stepRef := stepRefValue.Str
	if stepRef == "" {
		return false, fmt.Errorf("step_ref is required for step_status condition")
	}
	actualStatus, found := stepStatuses[stepRef]
	if !found {
		return false, fmt.Errorf("step %q not found in statuses", stepRef)
	}
	return actualStatus == domain.StepRunStatus(statusValue.Str), nil
}

func evaluateStepStatusConditionResult(cond gjson.Result, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	stepRefValue := cond.Get("step_ref")
	statusValue := cond.Get("status")
	invalidShape := stepRefValue.Exists() && stepRefValue.Type != gjson.String
	invalidShape = invalidShape || (statusValue.Exists() && statusValue.Type != gjson.String)
	if invalidShape {
		var c stepStatusCondition
		if err := json.Unmarshal([]byte(cond.Raw), &c); err != nil {
			return false, fmt.Errorf("unmarshal step_status condition: %w", err)
		}
	}

	stepRef := stepRefValue.Str
	if stepRef == "" {
		return false, fmt.Errorf("step_ref is required for step_status condition")
	}
	actualStatus, found := stepStatuses[stepRef]
	if !found {
		return false, fmt.Errorf("step %q not found in statuses", stepRef)
	}
	return actualStatus == domain.StepRunStatus(statusValue.Str), nil
}

func evaluateStepStatusInCondition(cond json.RawMessage, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	stepRefValue := gjson.GetBytes(cond, "step_ref")
	allowedStatuses := gjson.GetBytes(cond, "statuses")
	invalidShape := stepRefValue.Exists() && stepRefValue.Type != gjson.String
	invalidShape = invalidShape || (allowedStatuses.Exists() && !allowedStatuses.IsArray())
	if invalidShape {
		var c stepStatusInCondition
		if err := json.Unmarshal(cond, &c); err != nil {
			return false, fmt.Errorf("unmarshal step_status_in condition: %w", err)
		}
	}

	stepRef := stepRefValue.Str
	if stepRef == "" {
		return false, fmt.Errorf("step_ref is required for step_status_in condition")
	}
	actualStatus, found := stepStatuses[stepRef]
	if !found {
		return false, fmt.Errorf("step %q not found in statuses", stepRef)
	}

	var invalidAllowedStatus bool
	var matched bool
	allowedStatuses.ForEach(func(_, status gjson.Result) bool {
		if status.Type != gjson.String {
			invalidAllowedStatus = true
			return false
		}
		if actualStatus == domain.StepRunStatus(status.Str) {
			matched = true
			return false
		}
		return true
	})
	if invalidAllowedStatus {
		var c stepStatusInCondition
		if err := json.Unmarshal(cond, &c); err != nil {
			return false, fmt.Errorf("unmarshal step_status_in condition: %w", err)
		}
	}
	return matched, nil
}

func evaluateStepStatusInConditionResult(cond gjson.Result, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	stepRefValue := cond.Get("step_ref")
	allowedStatuses := cond.Get("statuses")
	invalidShape := stepRefValue.Exists() && stepRefValue.Type != gjson.String
	invalidShape = invalidShape || (allowedStatuses.Exists() && !allowedStatuses.IsArray())
	if invalidShape {
		var c stepStatusInCondition
		if err := json.Unmarshal([]byte(cond.Raw), &c); err != nil {
			return false, fmt.Errorf("unmarshal step_status_in condition: %w", err)
		}
	}

	stepRef := stepRefValue.Str
	if stepRef == "" {
		return false, fmt.Errorf("step_ref is required for step_status_in condition")
	}
	actualStatus, found := stepStatuses[stepRef]
	if !found {
		return false, fmt.Errorf("step %q not found in statuses", stepRef)
	}

	var invalidAllowedStatus bool
	var matched bool
	allowedStatuses.ForEach(func(_, status gjson.Result) bool {
		if status.Type != gjson.String {
			invalidAllowedStatus = true
			return false
		}
		if actualStatus == domain.StepRunStatus(status.Str) {
			matched = true
			return false
		}
		return true
	})
	if invalidAllowedStatus {
		var c stepStatusInCondition
		if err := json.Unmarshal([]byte(cond.Raw), &c); err != nil {
			return false, fmt.Errorf("unmarshal step_status_in condition: %w", err)
		}
	}
	return matched, nil
}

func evaluateNotCondition(cond json.RawMessage, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	nested := gjson.GetBytes(cond, "condition")
	if !nested.Exists() {
		return false, fmt.Errorf("condition is required for not condition")
	}
	ok, err := evaluateConditionResult(nested, stepStatuses)
	if err != nil {
		return false, err
	}
	return !ok, nil
}

func evaluateAllOfCondition(cond json.RawMessage, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	conditions, err := compositeConditions(cond, "all_of")
	if err != nil {
		return false, err
	}

	var evalErr error
	allMet := true
	conditions.ForEach(func(_, subCondition gjson.Result) bool {
		var ok bool
		ok, evalErr = EvaluateCondition(json.RawMessage(subCondition.Raw), stepStatuses)
		if evalErr != nil {
			return false
		}
		if !ok {
			allMet = false
			return false
		}
		return true
	})
	if evalErr != nil {
		return false, evalErr
	}
	return allMet, nil
}

func evaluateAnyOfCondition(cond json.RawMessage, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	conditions, err := compositeConditions(cond, "any_of")
	if err != nil {
		return false, err
	}

	var evalErr error
	var anyMet bool
	conditions.ForEach(func(_, subCondition gjson.Result) bool {
		var ok bool
		ok, evalErr = EvaluateCondition(json.RawMessage(subCondition.Raw), stepStatuses)
		if evalErr != nil {
			return false
		}
		if ok {
			anyMet = true
			return false
		}
		return true
	})
	if evalErr != nil {
		return false, evalErr
	}
	return anyMet, nil
}

func compositeConditions(cond json.RawMessage, condType string) (gjson.Result, error) {
	conditions := gjson.GetBytes(cond, "conditions")
	if conditions.Exists() && !conditions.IsArray() {
		var c compositeCondition
		if err := json.Unmarshal(cond, &c); err != nil {
			return gjson.Result{}, fmt.Errorf("unmarshal %s condition: %w", condType, err)
		}
	}
	return conditions, nil
}

func evaluateConditionResult(cond gjson.Result, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	condType := cond.Get("type").String()
	if condType == "" {
		return false, fmt.Errorf("unknown condition type: %q", "")
	}

	switch condType {
	case "step_status":
		return evaluateStepStatusConditionResult(cond, stepStatuses)
	case "step_status_in":
		return evaluateStepStatusInConditionResult(cond, stepStatuses)
	case "not":
		return evaluateNotConditionResult(cond, stepStatuses)
	case "all_of":
		return evaluateAllOfConditionResult(cond, stepStatuses)
	case "any_of":
		return evaluateAnyOfConditionResult(cond, stepStatuses)
	case "eq", "ne", "gt", "gte", "lt", "lte", "contains", "in", "regex":
		return evaluateBinaryConditionResult(condType, cond, stepStatuses)
	case "exists":
		return evaluateExistsConditionResult(cond, stepStatuses)
	default:
		return false, fmt.Errorf("unknown condition type: %q", condType)
	}
}

func evaluateNotConditionResult(cond gjson.Result, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	nested := cond.Get("condition")
	if !nested.Exists() {
		return false, fmt.Errorf("condition is required for not condition")
	}
	ok, err := evaluateConditionResult(nested, stepStatuses)
	if err != nil {
		return false, err
	}
	return !ok, nil
}

func evaluateAllOfConditionResult(cond gjson.Result, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	conditions, err := compositeConditionsResult(cond, "all_of")
	if err != nil {
		return false, err
	}

	var evalErr error
	allMet := true
	conditions.ForEach(func(_, subCondition gjson.Result) bool {
		var ok bool
		ok, evalErr = EvaluateCondition(json.RawMessage(subCondition.Raw), stepStatuses)
		if evalErr != nil {
			return false
		}
		if !ok {
			allMet = false
			return false
		}
		return true
	})
	if evalErr != nil {
		return false, evalErr
	}
	return allMet, nil
}

func evaluateAnyOfConditionResult(cond gjson.Result, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	conditions, err := compositeConditionsResult(cond, "any_of")
	if err != nil {
		return false, err
	}

	var evalErr error
	var anyMet bool
	conditions.ForEach(func(_, subCondition gjson.Result) bool {
		var ok bool
		ok, evalErr = EvaluateCondition(json.RawMessage(subCondition.Raw), stepStatuses)
		if evalErr != nil {
			return false
		}
		if ok {
			anyMet = true
			return false
		}
		return true
	})
	if evalErr != nil {
		return false, evalErr
	}
	return anyMet, nil
}

func compositeConditionsResult(cond gjson.Result, condType string) (gjson.Result, error) {
	conditions := cond.Get("conditions")
	if conditions.Exists() && !conditions.IsArray() {
		var c compositeCondition
		if err := json.Unmarshal([]byte(cond.Raw), &c); err != nil {
			return gjson.Result{}, fmt.Errorf("unmarshal %s condition: %w", condType, err)
		}
	}
	return conditions, nil
}

func evaluateBinaryCondition(
	condType string,
	cond json.RawMessage,
	stepStatuses map[string]domain.StepRunStatus,
) (bool, error) {
	if handled, ok, err := evaluateFastStringBinaryCondition(condType, cond, stepStatuses); ok || err != nil {
		return handled, err
	}

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
		return evaluateNumericCondition(condType, left, right)
	case "contains":
		return strings.Contains(fmt.Sprint(left), fmt.Sprint(right)), nil
	case "in":
		return evaluateInCondition(left, right)
	case "regex":
		return evaluateRegexCondition(fmt.Sprint(left), fmt.Sprint(right))
	default:
		return false, nil
	}
}

func evaluateBinaryConditionResult(
	condType string,
	cond gjson.Result,
	stepStatuses map[string]domain.StepRunStatus,
) (bool, error) {
	if handled, ok, err := evaluateFastStringBinaryConditionResult(condType, cond, stepStatuses); ok || err != nil {
		return handled, err
	}

	var c binaryCondition
	if err := json.Unmarshal([]byte(cond.Raw), &c); err != nil {
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
		return evaluateNumericCondition(condType, left, right)
	case "contains":
		return strings.Contains(fmt.Sprint(left), fmt.Sprint(right)), nil
	case "in":
		return evaluateInCondition(left, right)
	case "regex":
		return evaluateRegexCondition(fmt.Sprint(left), fmt.Sprint(right))
	default:
		return false, nil
	}
}

func evaluateNumericCondition(condType string, left, right any) (bool, error) {
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
}

func evaluateInCondition(left, right any) (bool, error) {
	items, ok := right.([]any)
	if !ok {
		return false, fmt.Errorf("in requires right operand array")
	}
	needle := fmt.Sprint(left)
	for _, item := range items {
		if fmt.Sprint(item) == needle {
			return true, nil
		}
	}
	return false, nil
}

func evaluateRegexCondition(input, pattern string) (bool, error) {
	if len(pattern) > maxRegexPatternLen {
		return false, fmt.Errorf("regex pattern exceeds maximum length of %d characters", maxRegexPatternLen)
	}
	if len(input) > maxRegexInputLen {
		return false, fmt.Errorf("regex input exceeds maximum length of %d characters", maxRegexInputLen)
	}
	re, err := cachedConditionRegex(pattern)
	if err != nil {
		return false, fmt.Errorf("invalid regex: %w", err)
	}
	return re.MatchString(input), nil
}

func evaluateExistsCondition(cond json.RawMessage, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	var c unaryCondition
	if err := json.Unmarshal(cond, &c); err != nil {
		return false, fmt.Errorf("unmarshal exists condition: %w", err)
	}
	v := resolveOperand(c.Operand, stepStatuses)
	return fmt.Sprint(v) != "", nil
}

func evaluateExistsConditionResult(cond gjson.Result, stepStatuses map[string]domain.StepRunStatus) (bool, error) {
	var c unaryCondition
	if err := json.Unmarshal([]byte(cond.Raw), &c); err != nil {
		return false, fmt.Errorf("unmarshal exists condition: %w", err)
	}
	v := resolveOperand(c.Operand, stepStatuses)
	return fmt.Sprint(v) != "", nil
}

func evaluateFastStringBinaryCondition(
	condType string,
	cond json.RawMessage,
	stepStatuses map[string]domain.StepRunStatus,
) (bool, bool, error) {
	switch condType {
	case "eq", "ne", "contains", "regex":
	default:
		return false, false, nil
	}

	left, ok := conditionOperandString(gjson.GetBytes(cond, "left"), stepStatuses)
	if !ok {
		return false, false, nil
	}
	right, ok := conditionOperandString(gjson.GetBytes(cond, "right"), stepStatuses)
	if !ok {
		return false, false, nil
	}

	switch condType {
	case "eq":
		return left == right, true, nil
	case "ne":
		return left != right, true, nil
	case "contains":
		return strings.Contains(left, right), true, nil
	case "regex":
		if len(right) > maxRegexPatternLen {
			return false, true, fmt.Errorf("regex pattern exceeds maximum length of %d characters", maxRegexPatternLen)
		}
		if len(left) > maxRegexInputLen {
			return false, true, fmt.Errorf("regex input exceeds maximum length of %d characters", maxRegexInputLen)
		}
		re, err := cachedConditionRegex(right)
		if err != nil {
			return false, true, fmt.Errorf("invalid regex: %w", err)
		}
		return re.MatchString(left), true, nil
	default:
		return false, false, nil
	}
}

func evaluateFastStringBinaryConditionResult(
	condType string,
	cond gjson.Result,
	stepStatuses map[string]domain.StepRunStatus,
) (bool, bool, error) {
	switch condType {
	case "eq", "ne", "contains", "regex":
	default:
		return false, false, nil
	}

	left, ok := conditionOperandString(cond.Get("left"), stepStatuses)
	if !ok {
		return false, false, nil
	}
	right, ok := conditionOperandString(cond.Get("right"), stepStatuses)
	if !ok {
		return false, false, nil
	}

	switch condType {
	case "eq":
		return left == right, true, nil
	case "ne":
		return left != right, true, nil
	case "contains":
		return strings.Contains(left, right), true, nil
	case "regex":
		if len(right) > maxRegexPatternLen {
			return false, true, fmt.Errorf("regex pattern exceeds maximum length of %d characters", maxRegexPatternLen)
		}
		if len(left) > maxRegexInputLen {
			return false, true, fmt.Errorf("regex input exceeds maximum length of %d characters", maxRegexInputLen)
		}
		re, err := cachedConditionRegex(right)
		if err != nil {
			return false, true, fmt.Errorf("invalid regex: %w", err)
		}
		return re.MatchString(left), true, nil
	default:
		return false, false, nil
	}
}

func conditionOperandString(result gjson.Result, stepStatuses map[string]domain.StepRunStatus) (string, bool) {
	if !result.Exists() {
		return "", false
	}
	switch result.Type {
	case gjson.Null:
		return "", true
	case gjson.String:
		return result.Str, true
	case gjson.Number, gjson.True, gjson.False:
		return result.Raw, true
	case gjson.JSON:
		if !result.IsObject() {
			return "", false
		}
		if stepRef := result.Get("step_ref"); stepRef.Type == gjson.String {
			status, found := stepStatuses[stepRef.Str]
			if !found {
				return "", true
			}
			return string(status), true
		}
		if value := result.Get("value"); value.Exists() {
			switch value.Type {
			case gjson.Null:
				return "", true
			case gjson.String:
				return value.Str, true
			case gjson.Number, gjson.True, gjson.False:
				return value.Raw, true
			case gjson.JSON:
				return "", false
			}
		}
	}
	return "", false
}

func cachedConditionRegex(pattern string) (*regexp.Regexp, error) {
	conditionRegexCache.RLock()
	re := conditionRegexCache.compiled[pattern]
	conditionRegexCache.RUnlock()
	if re != nil {
		return re, nil
	}

	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	conditionRegexCache.Lock()
	if existing := conditionRegexCache.compiled[pattern]; existing != nil {
		conditionRegexCache.Unlock()
		return existing, nil
	}
	if len(conditionRegexCache.compiled) >= maxConditionRegexCacheEntries {
		conditionRegexCache.compiled = make(map[string]*regexp.Regexp)
	}
	conditionRegexCache.compiled[pattern] = compiled
	conditionRegexCache.Unlock()

	return compiled, nil
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
