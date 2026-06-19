package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseInputFileCollectsResultsAndPackageStatuses(t *testing.T) {
	input := writeSummaryInput(t, `
{"Action":"output","Package":"strait/test/load","Test":"TestThroughput","Output":"--- PASS: TestThroughput\n"}
{"Action":"output","Package":"strait/test/load","Test":"TestThroughput","Output":"    123/sec, p95=9ms\n"}
{"Action":"pass","Package":"strait/test/load","Test":"TestThroughput","Elapsed":1.25}
{"Action":"output","Package":"strait/test/load","Test":"TestFailure","Output":"failure line\n"}
{"Action":"fail","Package":"strait/test/load","Test":"TestFailure","Elapsed":2.5}
{"Action":"skip","Package":"strait/test/load","Test":"TestSkipped","Elapsed":0.01}
{"Action":"pass","Package":"strait/test/load"}
{"Action":"fail","Package":"strait/test/other"}
not-json

`)

	results, packageStatus, err := parseInputFile(input)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"strait/test/load":  "pass",
		"strait/test/other": "fail",
	}, packageStatus)

	throughput := results["strait/test/load/TestThroughput"]
	require.NotNil(t, throughput)
	require.Equal(t, "pass", throughput.Status)
	require.InDelta(t, 1.25, throughput.Elapsed, 0)
	require.Equal(t, []string{"--- PASS: TestThroughput", "    123/sec, p95=9ms"}, throughput.Output)

	failed := results["strait/test/load/TestFailure"]
	require.NotNil(t, failed)
	require.Equal(t, "fail", failed.Status)
	require.Equal(t, []string{"failure line"}, failed.Output)

	skipped := results["strait/test/load/TestSkipped"]
	require.NotNil(t, skipped)
	require.Equal(t, "skip", skipped.Status)
}

func TestParseInputFileReturnsOpenAndScanErrors(t *testing.T) {
	_, _, err := parseInputFile(filepath.Join(t.TempDir(), "missing.jsonl"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "open input:")

	path := filepath.Join(t.TempDir(), "too-long.jsonl")
	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte("x"), 11*1024*1024), 0o600))
	_, _, err = parseInputFile(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "scan:")
}

func TestRenderSummaryIncludesFailuresPackagesAndThroughput(t *testing.T) {
	results := map[string]*testResult{
		"pkg/TestPass": {
			Name:    "TestPass",
			Status:  "pass",
			Elapsed: 0.5,
			Output:  []string{"--- PASS: TestPass", "    99/sec)"},
		},
		"pkg/TestFail": {
			Name:    "TestFail",
			Status:  "fail",
			Elapsed: 1.25,
			Output:  []string{"first failure", "second failure"},
		},
		"pkg/TestSkip": {
			Name:    "TestSkip",
			Status:  "skip",
			Elapsed: 0.01,
		},
		"pkg/TestUnknown": {
			Name:    "TestUnknown",
			Status:  "pause",
			Elapsed: 0,
		},
	}

	got := renderSummary("chaos", results, map[string]string{
		"pkg-b": "fail",
		"pkg-a": "pass",
	})

	require.Contains(t, got, "## Chaos Load Test Results")
	require.Contains(t, got, "**Overall: FAIL** | 1 passed | 1 failed | 1 skipped | 4 total")
	require.Contains(t, got, "- pkg-a: pass\n- pkg-b: fail")
	require.Contains(t, got, "### Failed Tests")
	require.Contains(t, got, "<summary>TestFail (1.25s)</summary>")
	require.Contains(t, got, "first failure\nsecond failure")
	require.Contains(t, got, "| TestFail | FAIL | 1.25s |")
	require.Contains(t, got, "| TestPass | PASS | 0.50s |")
	require.Contains(t, got, "| TestSkip | SKIP | 0.01s |")
	require.Contains(t, got, "| TestUnknown | ? | 0.00s |")
	require.Contains(t, got, "### Throughput Metrics")
	require.Contains(t, got, "| TestPass | 99/sec) |")
}

func TestRenderSummaryHandlesPassingEmptySuiteWithoutOptionalSections(t *testing.T) {
	got := renderSummary("", map[string]*testResult{
		"pkg/TestPass": {Name: "TestPass", Status: "pass"},
	}, nil)

	require.Contains(t, got, "## Load-test Load Test Results")
	require.Contains(t, got, "**Overall: pass** | 1 passed | 0 failed | 0 skipped | 1 total")
	require.NotContains(t, got, "### Packages")
	require.NotContains(t, got, "### Failed Tests")
	require.NotContains(t, got, "### Throughput Metrics")
}

func TestExitCodeValidatesFlagsAndWritesSummary(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := exitCode(nil, &stdout, &stderr, os.WriteFile)
	require.Equal(t, 1, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "required: -input <path>")

	stdout.Reset()
	stderr.Reset()
	code = exitCode([]string{"-input", "in.jsonl"}, &stdout, &stderr, os.WriteFile)
	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "required: -output <path>")

	input := writeSummaryInput(t, `{"Action":"pass","Package":"pkg","Test":"TestPass","Elapsed":0.1}`)
	output := filepath.Join(t.TempDir(), "summary.md")
	stdout.Reset()
	stderr.Reset()

	code = exitCode([]string{"-input", input, "-output", output, "-suite", "soak"}, &stdout, &stderr, os.WriteFile)
	require.Zero(t, code)
	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "Summary written to "+output+" (1 tests)")
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(t, string(data), "## Soak Load Test Results")
}

func TestExitCodeReportsParseFlagAndWriteErrors(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := exitCode([]string{"-bad"}, &stdout, &stderr, os.WriteFile)
	require.Equal(t, 1, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "flag provided but not defined")

	stdout.Reset()
	stderr.Reset()
	code = exitCode([]string{"-input", "missing.jsonl", "-output", "out.md"}, &stdout, &stderr, os.WriteFile)
	require.Equal(t, 1, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "open input:")

	input := writeSummaryInput(t, `{"Action":"pass","Package":"pkg","Test":"TestPass","Elapsed":0.1}`)
	stdout.Reset()
	stderr.Reset()
	code = exitCode(
		[]string{"-input", input, "-output", "out.md"},
		&stdout,
		&stderr,
		func(string, []byte, os.FileMode) error {
			return errors.New("disk full")
		},
	)
	require.Equal(t, 1, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "write output: disk full")
}

func TestExtractThroughputLinesCleansKnownPrefixes(t *testing.T) {
	got := extractThroughputLines([]*testResult{
		{
			Name: "TestA",
			Output: []string{
				"--- 111/sec)",
				"    222/sec, p95=2ms",
				"no throughput",
			},
		},
	})

	require.Equal(t, []throughputLine{
		{test: "TestA", line: "111/sec)"},
		{test: "TestA", line: "222/sec, p95=2ms"},
	}, got)
}

func writeSummaryInput(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "events.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
