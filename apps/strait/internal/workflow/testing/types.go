// Package testing provides workflow test suite definitions, parsing, and execution.
package testing

import (
	"encoding/json"
	"fmt"
)

// TestSuite represents a collection of workflow tests defined in a strait.test.yaml file.
type TestSuite struct {
	Tests []TestCase `json:"tests" yaml:"tests"`
}

// TestCase represents a single test within a test suite.
type TestCase struct {
	Name       string                  `json:"name" yaml:"name"`
	Workflow   string                  `json:"workflow" yaml:"workflow"`
	Payload    json.RawMessage         `json:"payload,omitempty" yaml:"payload,omitempty"`
	Mocks      map[string]MockEndpoint `json:"mocks,omitempty" yaml:"mocks,omitempty"`
	Assertions []Assertion             `json:"assertions" yaml:"assertions"`
}

// MockEndpoint defines a mock response for an external service.
type MockEndpoint struct {
	Response   json.RawMessage   `json:"response" yaml:"response"`
	StatusCode int               `json:"status_code,omitempty" yaml:"status_code,omitempty"`
	Headers    map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	LatencyMS  int               `json:"latency_ms,omitempty" yaml:"latency_ms,omitempty"`
}

// Assertion defines what to verify in a test case.
type Assertion struct {
	Step           string `json:"step,omitempty" yaml:"step,omitempty"`
	Status         string `json:"status,omitempty" yaml:"status,omitempty"`
	OutputContains string `json:"output_contains,omitempty" yaml:"output_contains,omitempty"`
	WorkflowStatus string `json:"workflow_status,omitempty" yaml:"workflow_status,omitempty"`
	DurationUnder  string `json:"duration_under,omitempty" yaml:"duration_under,omitempty"`
}

// TestResult represents the outcome of running a test case.
type TestResult struct {
	Name       string            `json:"name"`
	Passed     bool              `json:"passed"`
	Assertions []AssertionResult `json:"assertions"`
	Error      string            `json:"error,omitempty"`
	DurationMS int64             `json:"duration_ms"`
}

// AssertionResult represents the outcome of a single assertion.
type AssertionResult struct {
	Description string `json:"description"`
	Passed      bool   `json:"passed"`
	Expected    string `json:"expected,omitempty"`
	Actual      string `json:"actual,omitempty"`
}

// Validate checks that a test suite definition is well-formed.
func (s *TestSuite) Validate() error {
	if len(s.Tests) == 0 {
		return fmt.Errorf("test suite has no tests")
	}

	for i, tc := range s.Tests {
		if tc.Name == "" {
			return fmt.Errorf("test %d has no name", i)
		}
		if tc.Workflow == "" {
			return fmt.Errorf("test %q has no workflow", tc.Name)
		}
		if len(tc.Assertions) == 0 {
			return fmt.Errorf("test %q has no assertions", tc.Name)
		}
		for j, a := range tc.Assertions {
			if a.Step == "" && a.WorkflowStatus == "" {
				return fmt.Errorf("test %q assertion %d must specify step or workflow_status", tc.Name, j)
			}
		}
	}

	return nil
}
