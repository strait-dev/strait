package testing

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Parser tests.

func TestParseTestSuite_ValidJSON(t *testing.T) {
	t.Parallel()
	data := []byte(`{
		"tests": [{
			"name": "happy path",
			"workflow": "order-processing",
			"payload": {"order_id": "123"},
			"assertions": [
				{"step": "validate", "status": "completed"},
				{"workflow_status": "completed"}
			]
		}]
	}`)

	suite, err := ParseTestSuiteJSON(data)
	require.NoError(t, err)
	require.Len(t, suite.Tests, 1)
	assert.Equal(t, "happy path", suite.Tests[0].Name)
}

func TestParseTestSuite_MissingWorkflow(t *testing.T) {
	t.Parallel()
	data := []byte(`{"tests": [{"name": "test", "assertions": [{"step": "a", "status": "ok"}]}]}`)
	_, err := ParseTestSuiteJSON(data)
	assert.Error(t, err)
}

func TestParseTestSuite_EmptyAssertions(t *testing.T) {
	t.Parallel()
	data := []byte(`{"tests": [{"name": "test", "workflow": "wf", "assertions": []}]}`)
	_, err := ParseTestSuiteJSON(data)
	assert.Error(t, err)
}

func TestParseTestSuite_MultipleTests(t *testing.T) {
	t.Parallel()
	data := []byte(`{
		"tests": [
			{"name": "test1", "workflow": "wf", "assertions": [{"step": "a", "status": "ok"}]},
			{"name": "test2", "workflow": "wf", "assertions": [{"workflow_status": "completed"}]}
		]
	}`)

	suite, err := ParseTestSuiteJSON(data)
	require.NoError(t, err)
	assert.Len(t, suite.Tests, 2)
}

func TestParseTestSuite_EmptyData(t *testing.T) {
	t.Parallel()
	_, err := ParseTestSuiteJSON(nil)
	assert.Error(t, err)
}

func TestParseTestSuite_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := ParseTestSuiteJSON([]byte(`not valid json`))
	assert.Error(t, err)
}

func TestParseTestSuite_InvalidMockConfig(t *testing.T) {
	t.Parallel()
	data := []byte(`{
		"tests": [{
			"name": "test",
			"workflow": "wf",
			"mocks": {"service": {"status_code": 200, "response": {"ok": true}}},
			"assertions": [{"step": "a", "status": "completed"}]
		}]
	}`)

	suite, err := ParseTestSuiteJSON(data)
	require.NoError(t, err)
	assert.EqualValues(t, 200, suite.Tests[0].Mocks["service"].StatusCode)
}

// FilterTests.

func TestFilterTests_EmptyPattern(t *testing.T) {
	t.Parallel()
	suite := &TestSuite{Tests: []TestCase{{Name: "a"}, {Name: "b"}}}
	filtered := FilterTests(suite, "")
	assert.Len(t, filtered, 2)
}

func TestFilterTests_MatchPattern(t *testing.T) {
	t.Parallel()
	suite := &TestSuite{Tests: []TestCase{
		{Name: "payment success"},
		{Name: "payment failure"},
		{Name: "order processing"},
	}}
	filtered := FilterTests(suite, "payment")
	assert.Len(t, filtered, 2)
}

// Runner tests.

func TestRunner_HappyPath(t *testing.T) {
	t.Parallel()
	tc := TestCase{
		Name:     "happy path",
		Workflow: "wf",
		Assertions: []Assertion{
			{Step: "validate", Status: "completed"},
			{Step: "charge", Status: "completed"},
			{WorkflowStatus: "completed"},
		},
	}
	statuses := map[string]string{"validate": "completed", "charge": "completed"}
	outputs := map[string]string{}

	result := RunTestCase(tc, statuses, "completed", outputs, 5*time.Second)
	assert.True(t, result.Passed)
	assert.Len(t, result.Assertions, 3)
}

func TestRunner_StepStatusAssertion_Fail(t *testing.T) {
	t.Parallel()
	tc := TestCase{
		Name:     "step fails",
		Workflow: "wf",
		Assertions: []Assertion{
			{Step: "charge", Status: "completed"},
		},
	}
	statuses := map[string]string{"charge": "failed"}

	result := RunTestCase(tc, statuses, "failed", nil, 0)
	assert.False(t, result.Passed)
	require.NotEmpty(t, result.Assertions)
	assert.Equal(t, "failed", result.Assertions[0].Actual)
}

func TestRunner_OutputContains_Pass(t *testing.T) {
	t.Parallel()
	tc := TestCase{
		Name:     "output check",
		Workflow: "wf",
		Assertions: []Assertion{
			{Step: "charge", OutputContains: "txn-456"},
		},
	}
	outputs := map[string]string{"charge": `{"transaction_id":"txn-456","amount":99}`}

	result := RunTestCase(tc, nil, "", outputs, 0)
	assert.True(t, result.Passed)
}

func TestRunner_OutputContains_Fail(t *testing.T) {
	t.Parallel()
	tc := TestCase{
		Name:     "output missing",
		Workflow: "wf",
		Assertions: []Assertion{
			{Step: "charge", OutputContains: "txn-999"},
		},
	}
	outputs := map[string]string{"charge": `{"transaction_id":"txn-456"}`}

	result := RunTestCase(tc, nil, "", outputs, 0)
	assert.False(t, result.Passed)
}

func TestRunner_DurationUnder_Pass(t *testing.T) {
	t.Parallel()
	tc := TestCase{
		Name:     "fast enough",
		Workflow: "wf",
		Assertions: []Assertion{
			{Step: "a", DurationUnder: "30s"},
		},
	}

	result := RunTestCase(tc, nil, "", nil, 10*time.Second)
	assert.True(t, result.Passed)
}

func TestRunner_DurationUnder_Fail(t *testing.T) {
	t.Parallel()
	tc := TestCase{
		Name:     "too slow",
		Workflow: "wf",
		Assertions: []Assertion{
			{Step: "a", DurationUnder: "5s"},
		},
	}

	result := RunTestCase(tc, nil, "", nil, 10*time.Second)
	assert.False(t, result.Passed)
}

func TestRunner_WorkflowStatus_Pass(t *testing.T) {
	t.Parallel()
	tc := TestCase{
		Name:       "workflow ok",
		Workflow:   "wf",
		Assertions: []Assertion{{WorkflowStatus: "completed"}},
	}

	result := RunTestCase(tc, nil, "completed", nil, 0)
	assert.True(t, result.Passed)
}

// JUnit output tests.

func TestOutput_JUnitXML_AllPass(t *testing.T) {
	t.Parallel()
	results := []*TestResult{
		{Name: "test1", Passed: true, DurationMS: 100, Assertions: []AssertionResult{{Passed: true}}},
		{Name: "test2", Passed: true, DurationMS: 200, Assertions: []AssertionResult{{Passed: true}}},
	}

	out, err := FormatJUnitXML("suite", results)
	require.NoError(t, err)
	assert.Contains(t, string(out), `failures="0"`)
	assert.Contains(t, string(out), `tests="2"`)
}

func TestOutput_JUnitXML_SomeFailures(t *testing.T) {
	t.Parallel()
	results := []*TestResult{
		{Name: "pass", Passed: true, DurationMS: 50, Assertions: []AssertionResult{{Passed: true}}},
		{Name: "fail", Passed: false, DurationMS: 100, Assertions: []AssertionResult{
			{Description: "step status", Passed: false, Expected: "completed", Actual: "failed"},
		}},
	}

	out, err := FormatJUnitXML("suite", results)
	require.NoError(t, err)
	assert.Contains(t, string(out), `failures="1"`)
	assert.Contains(t, string(out), "step status")
}

func TestOutput_JUnitXML_Escaping(t *testing.T) {
	t.Parallel()
	results := []*TestResult{
		{Name: "test with <special> & chars", Passed: false, DurationMS: 10, Assertions: []AssertionResult{
			{Description: "check \"quotes\" & <tags>", Passed: false, Expected: "<expected>", Actual: "&actual"},
		}},
	}

	out, err := FormatJUnitXML("suite", results)
	require.NoError(t, err)

	// Verify valid XML.
	var suite junitTestSuite
	require.NoError(t, xml.Unmarshal(out, &suite))
}

// Fuzz tests.

func FuzzParseTestSuite(f *testing.F) {
	f.Add([]byte(`{"tests":[{"name":"t","workflow":"w","assertions":[{"step":"a","status":"ok"}]}]}`))
	f.Add([]byte(`not json`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must never panic.
		_, _ = ParseTestSuiteJSON(data)
	})
}

// Adversarial tests.

func TestTestSuite_100TestCases(t *testing.T) {
	t.Parallel()
	tests := make([]TestCase, 100)
	for i := range tests {
		tests[i] = TestCase{
			Name:       strings.Repeat("test-", 1) + string(rune('a'+i%26)),
			Workflow:   "wf",
			Assertions: []Assertion{{Step: "a", Status: "ok"}},
		}
	}
	suite := &TestSuite{Tests: tests}
	require.NoError(t, suite.Validate())
}

func TestTestSuite_MalformedYAML(t *testing.T) {
	t.Parallel()
	_, err := ParseTestSuiteYAML([]byte(`{{{ invalid yaml`))
	assert.Error(t, err)
}

func TestTestSuite_LargeMockResponse(t *testing.T) {
	t.Parallel()
	largeResponse, _ := json.Marshal(map[string]string{"data": strings.Repeat("x", 1024*1024)})
	data, _ := json.Marshal(TestSuite{
		Tests: []TestCase{{
			Name:     "large mock",
			Workflow: "wf",
			Mocks: map[string]MockEndpoint{
				"service": {Response: largeResponse, StatusCode: 200},
			},
			Assertions: []Assertion{{Step: "a", Status: "ok"}},
		}},
	})

	suite, err := ParseTestSuiteJSON(data)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(suite.Tests[0].Mocks["service"].Response), 1024*1024)
}

func TestRunner_InvalidDurationFormat(t *testing.T) {
	t.Parallel()
	tc := TestCase{
		Name:     "bad duration",
		Workflow: "wf",
		Assertions: []Assertion{
			{Step: "a", DurationUnder: "not-a-duration"},
		},
	}

	result := RunTestCase(tc, nil, "", nil, 0)
	assert.False(t, result.Passed)
}

func TestValidate_NoName(t *testing.T) {
	t.Parallel()
	suite := &TestSuite{Tests: []TestCase{{
		Workflow:   "wf",
		Assertions: []Assertion{{Step: "a", Status: "ok"}},
	}}}
	err := suite.Validate()
	assert.Error(t, err)
}

func TestValidate_AssertionMissingStepAndWorkflowStatus(t *testing.T) {
	t.Parallel()
	suite := &TestSuite{Tests: []TestCase{{
		Name:       "test",
		Workflow:   "wf",
		Assertions: []Assertion{{Status: "completed"}}, // missing step and workflow_status
	}}}
	err := suite.Validate()
	assert.Error(t, err)
}
