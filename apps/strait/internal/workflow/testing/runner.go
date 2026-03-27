package testing

import (
	"fmt"
	"strings"
	"time"
)

// RunTestCase executes a single test case and returns the result.
// In local simulation mode, this evaluates assertions against simulated
// step statuses without requiring a live Strait instance.
func RunTestCase(tc TestCase, stepStatuses map[string]string, workflowStatus string, stepOutputs map[string]string, totalDuration time.Duration) *TestResult {
	start := time.Now()
	result := &TestResult{
		Name:       tc.Name,
		Passed:     true,
		Assertions: make([]AssertionResult, 0, len(tc.Assertions)),
	}

	for _, a := range tc.Assertions {
		ar := evaluateAssertion(a, stepStatuses, workflowStatus, stepOutputs, totalDuration)
		result.Assertions = append(result.Assertions, ar)
		if !ar.Passed {
			result.Passed = false
		}
	}

	result.DurationMS = time.Since(start).Milliseconds()
	return result
}

func evaluateAssertion(a Assertion, stepStatuses map[string]string, workflowStatus string, stepOutputs map[string]string, totalDuration time.Duration) AssertionResult {
	// Workflow-level status assertion.
	if a.WorkflowStatus != "" {
		return AssertionResult{
			Description: fmt.Sprintf("workflow_status == %s", a.WorkflowStatus),
			Passed:      workflowStatus == a.WorkflowStatus,
			Expected:    a.WorkflowStatus,
			Actual:      workflowStatus,
		}
	}

	// Step-level assertions.
	if a.Step == "" {
		return AssertionResult{
			Description: "invalid assertion (no step or workflow_status)",
			Passed:      false,
		}
	}

	// Step status assertion.
	if a.Status != "" {
		actual := stepStatuses[a.Step]
		return AssertionResult{
			Description: fmt.Sprintf("step %q status == %s", a.Step, a.Status),
			Passed:      actual == a.Status,
			Expected:    a.Status,
			Actual:      actual,
		}
	}

	// Output contains assertion.
	if a.OutputContains != "" {
		actual := stepOutputs[a.Step]
		return AssertionResult{
			Description: fmt.Sprintf("step %q output contains %q", a.Step, a.OutputContains),
			Passed:      strings.Contains(actual, a.OutputContains),
			Expected:    a.OutputContains,
			Actual:      actual,
		}
	}

	// Duration under assertion.
	if a.DurationUnder != "" {
		dur, err := time.ParseDuration(a.DurationUnder)
		if err != nil {
			return AssertionResult{
				Description: fmt.Sprintf("duration_under %s (invalid duration)", a.DurationUnder),
				Passed:      false,
				Expected:    a.DurationUnder,
				Actual:      err.Error(),
			}
		}
		return AssertionResult{
			Description: fmt.Sprintf("duration < %s", a.DurationUnder),
			Passed:      totalDuration < dur,
			Expected:    a.DurationUnder,
			Actual:      totalDuration.String(),
		}
	}

	return AssertionResult{
		Description: "unknown assertion type",
		Passed:      false,
	}
}
