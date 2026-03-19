//go:build loadtest

package loadtest

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
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
	InputDir  string
	OutputDir string
}

// NewReportGenerator creates a report generator.
func NewReportGenerator(inputDir, outputDir string) *ReportGenerator {
	return &ReportGenerator{
		InputDir:  inputDir,
		OutputDir: outputDir,
	}
}

// Generate reads results from InputDir and produces HTML and JSON reports.
func (rg *ReportGenerator) Generate() error {
	if err := os.MkdirAll(rg.OutputDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	report := &Report{
		Generated: time.Now(),
	}

	// Load individual results if they exist
	rg.loadJSON("throughput_ceiling.json", &report.Throughput)
	rg.loadJSON("concurrency_ceiling.json", &report.Concurrency)
	rg.loadJSON("production_simulation.json", &report.MultiTenant)
	rg.loadJSON("chaos_all.json", &report.Chaos)
	rg.loadJSON("error_scenarios.json", &report.Errors)

	// Build summary
	report.Summary = rg.buildSummary(report)

	// Write JSON report
	if err := rg.writeJSON("report.json", report); err != nil {
		return fmt.Errorf("writing JSON report: %w", err)
	}

	// Write HTML report
	if err := rg.writeHTML("report.html", report); err != nil {
		return fmt.Errorf("writing HTML report: %w", err)
	}

	return nil
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
