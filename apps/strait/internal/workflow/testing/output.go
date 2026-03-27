package testing

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// FormatJUnitXML generates JUnit XML output from test results.
func FormatJUnitXML(suiteName string, results []*TestResult) ([]byte, error) {
	suite := junitTestSuite{
		Name:     suiteName,
		Tests:    len(results),
		Failures: 0,
	}

	for _, r := range results {
		tc := junitTestCase{
			Name:      r.Name,
			ClassName: suiteName,
			Time:      fmt.Sprintf("%.3f", float64(r.DurationMS)/1000),
		}

		if !r.Passed {
			suite.Failures++
			var messages []string
			for _, a := range r.Assertions {
				if !a.Passed {
					messages = append(messages, fmt.Sprintf("%s: expected=%s actual=%s", a.Description, a.Expected, a.Actual))
				}
			}
			tc.Failure = &junitFailure{
				Message: strings.Join(messages, "; "),
				Type:    "AssertionFailure",
			}
		}

		suite.TestCases = append(suite.TestCases, tc)
	}

	output, err := xml.MarshalIndent(suite, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal JUnit XML: %w", err)
	}

	return append([]byte(xml.Header), output...), nil
}

type junitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	TestCases []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
}
