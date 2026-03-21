//go:build loadtest

package loadtest

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Report is the top-level structure for a load test report.
type Report struct {
	Generated   time.Time              `json:"generated"`
	Environment ReportEnvironment      `json:"environment"`
	Summary     ReportSummary          `json:"summary"`
	Throughput  *RampResult            `json:"throughput,omitempty"`
	Concurrency *RampResult            `json:"concurrency,omitempty"`
	MultiTenant *TenantSimulatorResult `json:"multi_tenant,omitempty"`
	Chaos       []ChaosResult          `json:"chaos,omitempty"`
	Errors      []ErrorScenarioResult  `json:"errors,omitempty"`
	Metrics     []MetricsSnapshot      `json:"metrics,omitempty"`
}

// ReportEnvironment describes the test environment.
type ReportEnvironment struct {
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	CPUs      int    `json:"cpus"`
	MemoryGB  int    `json:"memory_gb"`
	GoVersion string `json:"go_version"`
}

// ReportSummary provides the executive summary.
type ReportSummary struct {
	MaxThroughput    int    `json:"max_throughput"`
	MaxConcurrency   int    `json:"max_concurrent"`
	MultiTenantCount int    `json:"multi_tenant_count"`
	EnduranceHours   int    `json:"endurance_hours"`
	ChaosTotal       int    `json:"chaos_total"`
	ChaosPassed      int    `json:"chaos_passed"`
	ErrorsTotal      int    `json:"errors_total"`
	ErrorsPassed     int    `json:"errors_passed"`
	MemoryLeak       bool   `json:"memory_leak"`
	GoroutineLeak    bool   `json:"goroutine_leak"`
	OverallVerdict   string `json:"overall_verdict"`
}

// ErrorScenarioResult captures the result of a single error scenario test.
type ErrorScenarioResult struct {
	Scenario     string `json:"scenario"`
	ExpectedClass string `json:"expected_class"`
	ActualClass  string `json:"actual_class"`
	Passed       bool   `json:"passed"`
	Error        string `json:"error,omitempty"`
}

// ReportGenerator builds reports from load test results.
type ReportGenerator struct {
	InputDir     string
	OutputDir    string
	HTMLFilename string
	JSONFilename string
	PDFFilename  string   // Optional: generates PDF via chromedp
	DiffDir      string   // Optional: second results dir for comparison
}

// LogCapture captures external service logs during load tests.
type LogCapture struct {
	OutputDir string
}

// NewLogCapture creates a log capture instance.
func NewLogCapture(outputDir string) *LogCapture {
	return &LogCapture{OutputDir: filepath.Join(outputDir, "logs")}
}

// CaptureAll captures logs from Postgres, Redis, and Docker.
func (lc *LogCapture) CaptureAll() error {
	if err := os.MkdirAll(lc.OutputDir, 0o755); err != nil {
		return fmt.Errorf("creating logs dir: %w", err)
	}

	// Capture Postgres slow query log (auto-detect container)
	if pgContainer, err := findContainer("postgres"); err == nil {
		lc.captureDockerLog(pgContainer, "postgres.log")
	}

	// Capture Redis log (auto-detect container)
	if redisContainer, err := findContainer("redis"); err == nil {
		lc.captureDockerLog(redisContainer, "redis.log")
	}

	return nil
}

func (lc *LogCapture) captureDockerLog(container, filename string) {
	if container == "" {
		return
	}
	path := filepath.Join(lc.OutputDir, filename)
	// Best-effort log capture
	data, err := execCommand("docker", "logs", "--tail", "10000", container)
	if err != nil {
		return
	}
	os.WriteFile(path, data, 0o644)
}

// ReportDiff compares two test runs and generates a diff report.
type ReportDiff struct {
	RunA     *Report     `json:"run_a"`
	RunB     *Report     `json:"run_b"`
	Changes  []DiffEntry `json:"changes"`
}

// DiffEntry represents a single difference between two runs.
type DiffEntry struct {
	Metric string `json:"metric"`
	ValueA string `json:"value_a"`
	ValueB string `json:"value_b"`
	Change string `json:"change"` // "improved", "degraded", "unchanged"
}

// NewReportGenerator creates a report generator.
func NewReportGenerator(inputDir, outputDir, htmlFile, jsonFile string) *ReportGenerator {
	if htmlFile == "" {
		htmlFile = "report.html"
	}
	if jsonFile == "" {
		jsonFile = "report.json"
	}
	return &ReportGenerator{
		InputDir:     inputDir,
		OutputDir:    outputDir,
		HTMLFilename: htmlFile,
		JSONFilename: jsonFile,
	}
}

// Generate reads results from InputDir and produces HTML, JSON, and optionally PDF reports.
func (rg *ReportGenerator) Generate() error {
	if err := os.MkdirAll(rg.OutputDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	report := &Report{
		Generated: time.Now(),
	}

	// Populate environment info
	hostname, _ := os.Hostname()
	report.Environment = ReportEnvironment{
		Hostname:  hostname,
		CPUs:      numCPUs(),
		GoVersion: goVersion(),
	}

	// Load individual results if they exist
	rg.loadJSON("throughput_ceiling.json", &report.Throughput)
	rg.loadJSON("concurrency_ceiling_http.json", &report.Concurrency)
	rg.loadJSON("production_simulation.json", &report.MultiTenant)
	rg.loadJSON("chaos_all.json", &report.Chaos)
	rg.loadJSON("error_scenarios.json", &report.Errors)

	// Load metrics snapshots
	rg.loadJSONL("raw/metrics_*.jsonl", &report.Metrics)

	// Build summary
	report.Summary = rg.buildSummary(report)

	// Capture service logs
	logCapture := NewLogCapture(rg.OutputDir)
	logCapture.CaptureAll()

	// Write JSON report
	if err := rg.writeJSON(rg.JSONFilename, report); err != nil {
		return fmt.Errorf("writing JSON report: %w", err)
	}

	// Write HTML report
	if err := rg.writeHTML(rg.HTMLFilename, report); err != nil {
		return fmt.Errorf("writing HTML report: %w", err)
	}

	// Generate diff report if comparison dir provided
	if rg.DiffDir != "" {
		if err := rg.generateDiff(report); err != nil {
			return fmt.Errorf("generating diff: %w", err)
		}
	}

	// Generate PDF if requested (requires chromedp/Chrome)
	if rg.PDFFilename != "" {
		if err := rg.generatePDF(); err != nil {
			// PDF is best-effort; don't fail the whole report
			fmt.Fprintf(os.Stderr, "PDF generation failed (chromedp/Chrome required): %v\n", err)
		}
	}

	return nil
}

func (rg *ReportGenerator) generateDiff(current *Report) error {
	other := &Report{}
	otherGen := &ReportGenerator{InputDir: rg.DiffDir, OutputDir: rg.OutputDir}
	otherGen.loadJSON("throughput_ceiling.json", &other.Throughput)
	otherGen.loadJSON("concurrency_ceiling_http.json", &other.Concurrency)
	otherGen.loadJSON("chaos_all.json", &other.Chaos)

	diff := ReportDiff{RunA: current, RunB: other}

	// Compare throughput
	if current.Throughput != nil && other.Throughput != nil {
		change := "unchanged"
		if current.Throughput.MaxRate > other.Throughput.MaxRate {
			change = "improved"
		} else if current.Throughput.MaxRate < other.Throughput.MaxRate {
			change = "degraded"
		}
		diff.Changes = append(diff.Changes, DiffEntry{
			Metric: "max_throughput",
			ValueA: fmt.Sprintf("%d jobs/sec", current.Throughput.MaxRate),
			ValueB: fmt.Sprintf("%d jobs/sec", other.Throughput.MaxRate),
			Change: change,
		})
	}

	// Compare concurrency
	if current.Concurrency != nil && other.Concurrency != nil {
		change := "unchanged"
		if current.Concurrency.MaxRate > other.Concurrency.MaxRate {
			change = "improved"
		} else if current.Concurrency.MaxRate < other.Concurrency.MaxRate {
			change = "degraded"
		}
		diff.Changes = append(diff.Changes, DiffEntry{
			Metric: "max_concurrency",
			ValueA: fmt.Sprintf("%d", current.Concurrency.MaxRate),
			ValueB: fmt.Sprintf("%d", other.Concurrency.MaxRate),
			Change: change,
		})
	}

	if err := rg.writeJSON("diff.json", diff); err != nil {
		return err
	}

	// Also generate HTML diff
	return rg.writeDiffHTML("diff.html", diff)
}

func (rg *ReportGenerator) writeDiffHTML(filename string, diff ReportDiff) error {
	path := filepath.Join(rg.OutputDir, filename)

	funcMap := template.FuncMap{
		"changeColor": func(change string) string {
			switch change {
			case "improved":
				return "#22c55e"
			case "degraded":
				return "#ef4444"
			default:
				return "#888"
			}
		},
	}

	tmpl, err := template.New("diff").Funcs(funcMap).Parse(diffHTMLTemplate)
	if err != nil {
		return fmt.Errorf("parsing diff template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating diff file: %w", err)
	}
	defer f.Close()

	return tmpl.Execute(f, diff)
}

var diffHTMLTemplate = "<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"UTF-8\">\n<title>Strait Load Test Diff</title>\n<style>\n  body { font-family: -apple-system, sans-serif; background: #0a0a0a; color: #e0e0e0; padding: 2rem; }\n  h1 { color: #fff; margin-bottom: 1rem; }\n  table { width: 100%%; border-collapse: collapse; margin: 1rem 0; }\n  th, td { padding: 0.8rem 1rem; text-align: left; border-bottom: 1px solid #1a1a1a; }\n  th { color: #888; font-size: 0.8rem; text-transform: uppercase; }\n  .improved { color: #22c55e; }\n  .degraded { color: #ef4444; }\n  .unchanged { color: #888; }\n</style>\n</head>\n<body>\n<h1>Load Test Comparison</h1>\n<table>\n  <thead><tr><th>Metric</th><th>Run A</th><th>Run B</th><th>Change</th></tr></thead>\n  <tbody>\n  {{range .Changes}}\n  <tr>\n    <td>{{.Metric}}</td>\n    <td>{{.ValueA}}</td>\n    <td>{{.ValueB}}</td>\n    <td class=\"{{.Change}}\">{{.Change}}</td>\n  </tr>\n  {{end}}\n  </tbody>\n</table>\n</body>\n</html>"

func (rg *ReportGenerator) generatePDF() error {
	// PDF generation uses chromedp to render the HTML report to PDF.
	// This requires Chrome/Chromium to be installed.
	// If not available, fall back to a simple text notice.
	htmlPath := filepath.Join(rg.OutputDir, rg.HTMLFilename)
	if _, err := os.Stat(htmlPath); err != nil {
		return fmt.Errorf("HTML report not found at %s: %w", htmlPath, err)
	}

	pdfPath := filepath.Join(rg.OutputDir, rg.PDFFilename)
	absHTML, _ := filepath.Abs(htmlPath)

	// Try chromedp via Chrome CLI (doesn't require Go chromedp dependency)
	data, err := execCommand("google-chrome", "--headless", "--disable-gpu",
		"--print-to-pdf="+pdfPath, "file://"+absHTML)
	if err != nil {
		// Try chromium
		data, err = execCommand("chromium", "--headless", "--disable-gpu",
			"--print-to-pdf="+pdfPath, "file://"+absHTML)
	}
	if err != nil {
		// Try macOS Chrome
		data, err = execCommand("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"--headless", "--disable-gpu",
			"--print-to-pdf="+pdfPath, "file://"+absHTML)
	}
	_ = data
	return err
}

func (rg *ReportGenerator) loadJSON(filename string, target any) {
	path := filepath.Join(rg.InputDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	json.Unmarshal(data, target)
}

func (rg *ReportGenerator) writeJSON(filename string, data any) error {
	path := filepath.Join(rg.OutputDir, filename)
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func (rg *ReportGenerator) buildSummary(report *Report) ReportSummary {
	s := ReportSummary{
		OverallVerdict: "PASS",
	}

	if report.Throughput != nil {
		s.MaxThroughput = report.Throughput.MaxRate
	}
	if report.Concurrency != nil {
		s.MaxConcurrency = report.Concurrency.MaxRate
	}
	if report.MultiTenant != nil {
		s.MultiTenantCount = len(report.MultiTenant.PerTenant)
	}

	s.ChaosTotal = len(report.Chaos)
	for _, c := range report.Chaos {
		if c.Verdict == "PASS" {
			s.ChaosPassed++
		}
	}
	if s.ChaosPassed < s.ChaosTotal {
		s.OverallVerdict = "FAIL"
	}

	s.ErrorsTotal = len(report.Errors)
	for _, e := range report.Errors {
		if e.Passed {
			s.ErrorsPassed++
		}
	}
	if s.ErrorsPassed < s.ErrorsTotal {
		s.OverallVerdict = "FAIL"
	}

	return s
}

func (rg *ReportGenerator) writeHTML(filename string, report *Report) error {
	path := filepath.Join(rg.OutputDir, filename)

	funcMap := template.FuncMap{
		"mul": func(a, b float64) float64 { return a * b },
	}
	tmpl, err := template.New("report").Funcs(funcMap).Parse(reportHTMLTemplate)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	return tmpl.Execute(f, report)
}

func (rg *ReportGenerator) loadJSONL(globPattern string, target *[]MetricsSnapshot) {
	pattern := filepath.Join(rg.InputDir, globPattern)
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return
	}
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range splitLines(string(data)) {
			if len(line) == 0 {
				continue
			}
			var snap MetricsSnapshot
			if json.Unmarshal([]byte(line), &snap) == nil {
				*target = append(*target, snap)
			}
		}
	}
}

func execCommand(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func numCPUs() int {
	return runtime.NumCPU()
}

func goVersion() string {
	return runtime.Version()
}

const reportHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Strait Capacity Report</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: #0a0a0a; color: #e0e0e0; padding: 2rem; }
  h1 { font-size: 1.8rem; margin-bottom: 0.5rem; color: #fff; }
  h2 { font-size: 1.3rem; margin: 2rem 0 1rem; color: #a0a0a0; text-transform: uppercase; letter-spacing: 0.1em; border-bottom: 1px solid #222; padding-bottom: 0.5rem; }
  h3 { font-size: 1.1rem; margin: 1.5rem 0 0.5rem; color: #ccc; }
  .header { margin-bottom: 2rem; }
  .header .subtitle { color: #666; font-size: 0.9rem; }
  .summary { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 1rem; margin: 1rem 0; }
  .card { background: #141414; border: 1px solid #222; border-radius: 8px; padding: 1.2rem; }
  .card .value { font-size: 2rem; font-weight: 700; color: #fff; }
  .card .label { font-size: 0.8rem; color: #666; text-transform: uppercase; letter-spacing: 0.05em; margin-top: 0.3rem; }
  .verdict-pass { color: #22c55e; }
  .verdict-fail { color: #ef4444; }
  table { width: 100%; border-collapse: collapse; margin: 1rem 0; }
  th, td { padding: 0.6rem 1rem; text-align: left; border-bottom: 1px solid #1a1a1a; }
  th { color: #888; font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.05em; }
  td { font-size: 0.9rem; }
  .pass { color: #22c55e; }
  .fail { color: #ef4444; }
  .section { margin: 2rem 0; }
  .steps-table td:nth-child(n+2) { font-variant-numeric: tabular-nums; text-align: right; }
  .steps-table th:nth-child(n+2) { text-align: right; }
  details { margin: 0.5rem 0; }
  summary { cursor: pointer; color: #a0a0a0; }
  pre { background: #111; padding: 1rem; border-radius: 4px; overflow-x: auto; font-size: 0.8rem; margin: 0.5rem 0; }
  .footer { margin-top: 3rem; padding-top: 1rem; border-top: 1px solid #222; color: #444; font-size: 0.8rem; }
</style>
</head>
<body>
<div class="header">
  <h1>Strait Capacity Report</h1>
  <div class="subtitle">Generated: {{.Generated.Format "2006-01-02 15:04:05 UTC"}}</div>
</div>

<h2>Executive Summary</h2>
<div class="summary">
  <div class="card">
    <div class="value">{{.Summary.MaxThroughput}}</div>
    <div class="label">Max Throughput (jobs/sec)</div>
  </div>
  <div class="card">
    <div class="value">{{.Summary.MaxConcurrency}}</div>
    <div class="label">Max Concurrent</div>
  </div>
  <div class="card">
    <div class="value">{{.Summary.MultiTenantCount}}</div>
    <div class="label">Tenants Tested</div>
  </div>
  <div class="card">
    <div class="value">{{.Summary.ChaosPassed}}/{{.Summary.ChaosTotal}}</div>
    <div class="label">Chaos Tests Passed</div>
  </div>
  <div class="card">
    <div class="value">{{.Summary.ErrorsPassed}}/{{.Summary.ErrorsTotal}}</div>
    <div class="label">Error Scenarios Passed</div>
  </div>
  <div class="card">
    <div class="value verdict-{{if eq .Summary.OverallVerdict "PASS"}}pass{{else}}fail{{end}}">{{.Summary.OverallVerdict}}</div>
    <div class="label">Overall Verdict</div>
  </div>
</div>

{{if .Throughput}}
<div class="section">
  <h2>Tier 1: Throughput Ceiling</h2>
  <p>Max sustained: <strong>{{.Throughput.MaxRate}} jobs/sec</strong>. Breaks at: <strong>{{.Throughput.BreakingRate}} jobs/sec</strong>. Bottleneck: <strong>{{.Throughput.Bottleneck}}</strong></p>
  {{if .Throughput.Steps}}
  <table class="steps-table">
    <thead><tr><th>Rate</th><th>Ops</th><th>Errors</th><th>Error %</th><th>P50</th><th>P95</th><th>P99</th><th>Queue</th></tr></thead>
    <tbody>
    {{range .Throughput.Steps}}
    <tr>
      <td>{{.Rate}}/s</td><td>{{.Operations}}</td><td>{{.Errors}}</td>
      <td>{{printf "%.2f" (mul .ErrorRate 100)}}%</td>
      <td>{{.LatencyP50}}</td><td>{{.LatencyP95}}</td><td>{{.LatencyP99}}</td>
      <td>{{.QueueDepth}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
  {{end}}
</div>
{{end}}

{{if .Concurrency}}
<div class="section">
  <h2>Tier 2: Concurrency Ceiling</h2>
  <p>Max sustained concurrent: <strong>{{.Concurrency.MaxRate}}</strong>. Breaks at: <strong>{{.Concurrency.BreakingRate}}</strong>. Bottleneck: <strong>{{.Concurrency.Bottleneck}}</strong></p>
</div>
{{end}}

{{if .MultiTenant}}
<div class="section">
  <h2>Tier 3: Multi-Tenant Simulation</h2>
  <p>Total runs: <strong>{{.MultiTenant.TotalRuns}}</strong> | Errors: <strong>{{.MultiTenant.TotalErrors}}</strong> | Rate: <strong>{{printf "%.1f" .MultiTenant.RunsPerSecond}} runs/sec</strong></p>
</div>
{{end}}

{{if .Chaos}}
<div class="section">
  <h2>Tier 5: Chaos Engineering</h2>
  <table>
    <thead><tr><th>Scenario</th><th>Lost</th><th>Recovered</th><th>Recovery Time</th><th>Verdict</th></tr></thead>
    <tbody>
    {{range .Chaos}}
    <tr>
      <td>{{.Scenario}}</td><td>{{.Lost}}</td><td>{{.Recovered}}</td>
      <td>{{.RecoveryTime}}</td>
      <td class="{{if eq .Verdict "PASS"}}pass{{else}}fail{{end}}">{{.Verdict}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
</div>
{{end}}

{{if .Errors}}
<div class="section">
  <h2>Error Scenarios</h2>
  <table>
    <thead><tr><th>Scenario</th><th>Expected</th><th>Actual</th><th>Result</th></tr></thead>
    <tbody>
    {{range .Errors}}
    <tr>
      <td>{{.Scenario}}</td><td>{{.ExpectedClass}}</td><td>{{.ActualClass}}</td>
      <td class="{{if .Passed}}pass{{else}}fail{{end}}">{{if .Passed}}PASS{{else}}FAIL{{end}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
</div>
{{end}}

<div class="footer">
  Strait Load Test Report
</div>
</body>
</html>`
