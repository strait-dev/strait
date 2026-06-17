package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func parseInputFile(path string) (map[string]*testResult, map[string]string, error) {
	cleanPath := filepath.Clean(path)
	f, err := os.Open(cleanPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open input: %w", err)
	}
	defer f.Close()

	results := map[string]*testResult{}
	packageStatus := map[string]string{}

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var ev testEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}

		if ev.Test == "" {
			if ev.Action == "pass" || ev.Action == "fail" || ev.Action == "skip" {
				packageStatus[ev.Package] = ev.Action
			}
			continue
		}

		key := ev.Package + "/" + ev.Test
		if _, ok := results[key]; !ok {
			results[key] = &testResult{Name: ev.Test}
		}
		tr := results[key]

		switch ev.Action {
		case "pass", "fail", "skip":
			tr.Status = ev.Action
			tr.Elapsed = ev.Elapsed
		case "output":
			if ev.Output != "" {
				tr.Output = append(tr.Output, strings.TrimRight(ev.Output, "\n"))
			}
		}
	}

	scanErr := scanner.Err()
	if scanErr != nil {
		return nil, nil, fmt.Errorf("scan: %w", scanErr)
	}

	return results, packageStatus, nil
}

type testEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test"`
	Elapsed float64   `json:"Elapsed"`
	Output  string    `json:"Output"`
}

type testResult struct {
	Name    string
	Status  string
	Elapsed float64
	Output  []string
}

func main() {
	os.Exit(exitCode(os.Args[1:], os.Stdout, os.Stderr, os.WriteFile))
}

type writeFileFunc func(string, []byte, os.FileMode) error

func exitCode(args []string, stdout io.Writer, stderr io.Writer, writeFile writeFileFunc) int {
	flags := flag.NewFlagSet("summary", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inputFile := flags.String("input", "", "path to go test -json output (jsonl)")
	outputFile := flags.String("output", "", "path to write markdown summary")
	suite := flags.String("suite", "load", "suite name for the summary header")
	if err := flags.Parse(args); err != nil {
		return 1
	}

	if *inputFile == "" {
		fmt.Fprintln(stderr, "required: -input <path>")
		return 1
	}
	if *outputFile == "" {
		fmt.Fprintln(stderr, "required: -output <path>")
		return 1
	}

	results, packageStatus, err := parseInputFile(*inputFile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	body := renderSummary(*suite, results, packageStatus)
	if err := writeFile(*outputFile, []byte(body), 0o600); err != nil {
		fmt.Fprintf(stderr, "write output: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Summary written to %s (%d tests)\n", *outputFile, len(results))
	return 0
}

func renderSummary(suite string, results map[string]*testResult, packageStatus map[string]string) string {
	packageResults := make([]string, 0, len(packageStatus))

	var passed, failed, skipped int
	sorted := make([]*testResult, 0, len(results))
	for _, tr := range results {
		sorted = append(sorted, tr)
		switch tr.Status {
		case "pass":
			passed++
		case "fail":
			failed++
		case "skip":
			skipped++
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	for pkg, status := range packageStatus {
		packageResults = append(packageResults, fmt.Sprintf("%s: %s", pkg, status))
	}
	sort.Strings(packageResults)

	var b strings.Builder

	title := suiteTitle(suite)
	fmt.Fprintf(&b, "## %s Load Test Results\n\n", title)

	statusIcon := "pass"
	if failed > 0 {
		statusIcon = "FAIL"
	}
	fmt.Fprintf(&b, "**Overall: %s** | %d passed | %d failed | %d skipped | %d total\n\n",
		statusIcon, passed, failed, skipped, len(sorted))

	if len(packageResults) > 0 {
		fmt.Fprintf(&b, "### Packages\n\n")
		for _, pr := range packageResults {
			fmt.Fprintf(&b, "- %s\n", pr)
		}
		fmt.Fprintf(&b, "\n")
	}

	if failed > 0 {
		fmt.Fprintf(&b, "### Failed Tests\n\n")
		for _, tr := range sorted {
			if tr.Status != "fail" {
				continue
			}
			fmt.Fprintf(&b, "<details>\n<summary>%s (%.2fs)</summary>\n\n```\n", tr.Name, tr.Elapsed)
			for _, line := range tr.Output {
				fmt.Fprintf(&b, "%s\n", line)
			}
			fmt.Fprintf(&b, "```\n\n</details>\n\n")
		}
	}

	fmt.Fprintf(&b, "### All Tests\n\n")
	fmt.Fprintf(&b, "| Test | Status | Duration |\n")
	fmt.Fprintf(&b, "| --- | --- | ---: |\n")
	for _, tr := range sorted {
		icon := "?"
		switch tr.Status {
		case "pass":
			icon = "PASS"
		case "fail":
			icon = "FAIL"
		case "skip":
			icon = "SKIP"
		}
		fmt.Fprintf(&b, "| %s | %s | %.2fs |\n", tr.Name, icon, tr.Elapsed)
	}

	throughputLines := extractThroughputLines(sorted)
	if len(throughputLines) > 0 {
		fmt.Fprintf(&b, "\n### Throughput Metrics\n\n")
		fmt.Fprintf(&b, "| Test | Metric |\n")
		fmt.Fprintf(&b, "| --- | --- |\n")
		for _, tl := range throughputLines {
			fmt.Fprintf(&b, "| %s | %s |\n", tl.test, tl.line)
		}
	}

	return b.String()
}

func suiteTitle(suite string) string {
	if suite == "" {
		return "Load-test"
	}
	return strings.ToUpper(string(suite[0])) + suite[1:]
}

type throughputLine struct {
	test string
	line string
}

func extractThroughputLines(tests []*testResult) []throughputLine {
	var out []throughputLine
	for _, tr := range tests {
		for _, line := range tr.Output {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "/sec)") || strings.Contains(trimmed, "/sec,") {
				clean := strings.TrimPrefix(trimmed, "--- ")
				clean = strings.TrimPrefix(clean, "    ")
				out = append(out, throughputLine{test: tr.Name, line: clean})
			}
		}
	}
	return out
}
