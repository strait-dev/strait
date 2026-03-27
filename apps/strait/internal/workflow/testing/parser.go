package testing

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseTestSuiteJSON parses a test suite from JSON bytes.
func ParseTestSuiteJSON(data []byte) (*TestSuite, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty test suite data")
	}

	var suite TestSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("parse test suite JSON: %w", err)
	}

	if err := suite.Validate(); err != nil {
		return nil, fmt.Errorf("invalid test suite: %w", err)
	}

	return &suite, nil
}

// ParseTestSuiteYAML parses a test suite from YAML-like JSON (simplified).
// For full YAML support, use a proper YAML parser -- this handles the
// JSON-compatible subset used in most test definitions.
func ParseTestSuiteYAML(data []byte) (*TestSuite, error) {
	// Try JSON first since YAML is a superset of JSON.
	suite, err := ParseTestSuiteJSON(data)
	if err == nil {
		return suite, nil
	}

	return nil, fmt.Errorf("parse test suite: %w (only JSON format is currently supported)", err)
}

// FilterTests returns only tests whose name contains the given pattern.
func FilterTests(suite *TestSuite, pattern string) []TestCase {
	if pattern == "" {
		return suite.Tests
	}

	var filtered []TestCase
	for _, tc := range suite.Tests {
		if strings.Contains(tc.Name, pattern) {
			filtered = append(filtered, tc)
		}
	}
	return filtered
}
