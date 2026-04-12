//go:build loadtest

package loadtest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MarkdownReportsDir is the default on-disk location for scenario
// Markdown summaries. The directory is gitignored; it exists only on
// the machine that ran the scenario.
const MarkdownReportsDir = "internal/loadtest/reports"

// MarkdownSummary captures the minimal set of fields needed to produce
// a reviewer-friendly scenario summary.
type MarkdownSummary struct {
	Scenario    string
	StartedAt   time.Time
	FinishedAt  time.Time
	Tier        int
	Description string

	// Headline metrics. Zero values render as "n/a".
	MaxThroughput  int
	P50LatencyMS   float64
	P95LatencyMS   float64
	P99LatencyMS   float64
	ErrorRate      float64
	RejectionRate  float64
	QueueDepthPeak int
	Notes          []string
}

// WriteMarkdownReport writes summary to
// <dir>/<scenario>-<ISO8601>.md, creating the directory if needed.
// Returns the path written.
func WriteMarkdownReport(dir string, summary MarkdownSummary) (string, error) {
	if dir == "" {
		dir = MarkdownReportsDir
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("mkdir reports dir: %w", err)
	}
	stamp := summary.FinishedAt.UTC().Format("20060102T150405Z")
	if summary.FinishedAt.IsZero() {
		stamp = time.Now().UTC().Format("20060102T150405Z")
	}
	name := fmt.Sprintf("%s-%s.md", safeFilename(summary.Scenario), stamp)
	path := filepath.Join(dir, name)
	body := renderMarkdown(summary)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return "", fmt.Errorf("write markdown report: %w", err)
	}
	return path, nil
}

func renderMarkdown(s MarkdownSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Load test: %s\n\n", s.Scenario)
	if s.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", s.Description)
	}
	fmt.Fprintf(&b, "- Tier: %d\n", s.Tier)
	if !s.StartedAt.IsZero() {
		fmt.Fprintf(&b, "- Started: %s\n", s.StartedAt.UTC().Format(time.RFC3339))
	}
	if !s.FinishedAt.IsZero() {
		fmt.Fprintf(&b, "- Finished: %s\n", s.FinishedAt.UTC().Format(time.RFC3339))
		if !s.StartedAt.IsZero() {
			fmt.Fprintf(&b, "- Duration: %s\n", s.FinishedAt.Sub(s.StartedAt).Round(time.Second))
		}
	}
	b.WriteString("\n## Headline metrics\n\n")
	b.WriteString("| Metric | Value |\n")
	b.WriteString("|---|---|\n")
	fmt.Fprintf(&b, "| Max throughput (jobs/sec) | %s |\n", intOrNA(s.MaxThroughput))
	fmt.Fprintf(&b, "| P50 latency (ms) | %s |\n", floatOrNA(s.P50LatencyMS))
	fmt.Fprintf(&b, "| P95 latency (ms) | %s |\n", floatOrNA(s.P95LatencyMS))
	fmt.Fprintf(&b, "| P99 latency (ms) | %s |\n", floatOrNA(s.P99LatencyMS))
	fmt.Fprintf(&b, "| Error rate | %s |\n", percentOrNA(s.ErrorRate))
	fmt.Fprintf(&b, "| Rejection rate | %s |\n", percentOrNA(s.RejectionRate))
	fmt.Fprintf(&b, "| Queue depth peak | %s |\n", intOrNA(s.QueueDepthPeak))
	if len(s.Notes) > 0 {
		b.WriteString("\n## Notes\n\n")
		for _, n := range s.Notes {
			fmt.Fprintf(&b, "- %s\n", n)
		}
	}
	return b.String()
}

func safeFilename(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "scenario"
	}
	return string(out)
}

func intOrNA(n int) string {
	if n == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d", n)
}

func floatOrNA(f float64) string {
	if f == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", f)
}

func percentOrNA(f float64) string {
	if f == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2f%%", f*100)
}
