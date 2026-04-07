//go:build loadtest

package loadtest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReportGenerator_EmptyInput(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	rg := NewReportGenerator(inputDir, outputDir, "report.html", "report.json")
	if err := rg.Generate(); err != nil {
		t.Fatalf("Generate() with empty input failed: %v", err)
	}

	// Verify HTML was created and is valid
	htmlPath := filepath.Join(outputDir, "report.html")
	htmlData, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("reading HTML report: %v", err)
	}
	if !strings.Contains(string(htmlData), "<!DOCTYPE html>") {
		t.Error("HTML report missing DOCTYPE")
	}
	if !strings.Contains(string(htmlData), "Strait Capacity Report") {
		t.Error("HTML report missing title")
	}

	// Verify JSON was created and is valid
	jsonPath := filepath.Join(outputDir, "report.json")
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("reading JSON report: %v", err)
	}
	var report Report
	if err := json.Unmarshal(jsonData, &report); err != nil {
		t.Fatalf("JSON report is not valid JSON: %v", err)
	}

	// Empty input should produce PASS verdict (no failures)
	if report.Summary.OverallVerdict != "PASS" {
		t.Errorf("expected PASS verdict for empty input, got %s", report.Summary.OverallVerdict)
	}
	if report.Summary.MaxThroughput != 0 {
		t.Errorf("expected 0 max throughput for empty input, got %d", report.Summary.MaxThroughput)
	}
}

func TestReportGenerator_WithThroughputData(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	// Write a throughput_ceiling.json file
	throughput := RampResult{
		Mode:            RampThroughput,
		MaxRate:         500,
		BreakingRate:    600,
		Bottleneck:      "latency_p99_10s",
		Duration:        30 * time.Minute,
		TotalOperations: 15000,
		TotalErrors:     50,
		Steps: []RampStepResult{
			{Rate: 100, Operations: 3000, Errors: 0, ErrorRate: 0, LatencyP50: 10 * time.Millisecond, LatencyP95: 50 * time.Millisecond, LatencyP99: 100 * time.Millisecond},
			{Rate: 200, Operations: 6000, Errors: 10, ErrorRate: 0.0017, LatencyP50: 15 * time.Millisecond, LatencyP95: 80 * time.Millisecond, LatencyP99: 200 * time.Millisecond},
			{Rate: 500, Operations: 6000, Errors: 40, ErrorRate: 0.0066, LatencyP50: 50 * time.Millisecond, LatencyP95: 500 * time.Millisecond, LatencyP99: 1 * time.Second},
		},
	}

	data, err := json.MarshalIndent(throughput, "", "  ")
	if err != nil {
		t.Fatalf("marshaling throughput data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "throughput_ceiling.json"), data, 0o644); err != nil {
		t.Fatalf("writing throughput file: %v", err)
	}

	rg := NewReportGenerator(inputDir, outputDir, "report.html", "report.json")
	if err := rg.Generate(); err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// Read and verify JSON report
	jsonData, err := os.ReadFile(filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatalf("reading JSON report: %v", err)
	}
	var report Report
	if err := json.Unmarshal(jsonData, &report); err != nil {
		t.Fatalf("invalid JSON report: %v", err)
	}

	if report.Summary.MaxThroughput != 500 {
		t.Errorf("expected MaxThroughput=500, got %d", report.Summary.MaxThroughput)
	}
	if report.Throughput == nil {
		t.Fatal("expected Throughput to be populated")
	}
	if report.Throughput.BreakingRate != 600 {
		t.Errorf("expected BreakingRate=600, got %d", report.Throughput.BreakingRate)
	}
	if len(report.Throughput.Steps) != 3 {
		t.Errorf("expected 3 steps, got %d", len(report.Throughput.Steps))
	}
}

func TestReportGenerator_BuildSummary(t *testing.T) {
	rg := &ReportGenerator{}

	tests := []struct {
		name            string
		report          *Report
		wantVerdict     string
		wantThroughput  int
		wantChaosPassed int
	}{
		{
			name:        "empty report",
			report:      &Report{},
			wantVerdict: "PASS",
		},
		{
			name: "with throughput only",
			report: &Report{
				Throughput: &RampResult{MaxRate: 1000},
			},
			wantVerdict:    "PASS",
			wantThroughput: 1000,
		},
		{
			name: "chaos all passed",
			report: &Report{
				Chaos: []ChaosResult{
					{Scenario: "test1", Verdict: "PASS"},
					{Scenario: "test2", Verdict: "PASS"},
				},
			},
			wantVerdict:     "PASS",
			wantChaosPassed: 2,
		},
		{
			name: "chaos some failed",
			report: &Report{
				Chaos: []ChaosResult{
					{Scenario: "test1", Verdict: "PASS"},
					{Scenario: "test2", Verdict: "FAIL"},
				},
			},
			wantVerdict:     "FAIL",
			wantChaosPassed: 1,
		},
		{
			name: "errors some failed",
			report: &Report{
				Errors: []ErrorScenarioResult{
					{Scenario: "e1", Passed: true},
					{Scenario: "e2", Passed: false},
				},
			},
			wantVerdict: "FAIL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := rg.buildSummary(tt.report)

			if summary.OverallVerdict != tt.wantVerdict {
				t.Errorf("OverallVerdict = %s, want %s", summary.OverallVerdict, tt.wantVerdict)
			}
			if tt.wantThroughput > 0 && summary.MaxThroughput != tt.wantThroughput {
				t.Errorf("MaxThroughput = %d, want %d", summary.MaxThroughput, tt.wantThroughput)
			}
			if tt.wantChaosPassed > 0 && summary.ChaosPassed != tt.wantChaosPassed {
				t.Errorf("ChaosPassed = %d, want %d", summary.ChaosPassed, tt.wantChaosPassed)
			}
		})
	}
}

func TestReportGenerator_DiffReport(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	outputDir := t.TempDir()

	// Write throughput results in both dirs
	throughputA := RampResult{MaxRate: 500, BreakingRate: 600, Bottleneck: "latency"}
	throughputB := RampResult{MaxRate: 400, BreakingRate: 500, Bottleneck: "latency"}

	writeJSON := func(dir, filename string, v any) {
		data, _ := json.MarshalIndent(v, "", "  ")
		os.WriteFile(filepath.Join(dir, filename), data, 0o644)
	}

	writeJSON(dirA, "throughput_ceiling.json", throughputA)
	writeJSON(dirB, "throughput_ceiling.json", throughputB)

	rg := NewReportGenerator(dirA, outputDir, "report.html", "report.json")
	rg.DiffDir = dirB

	if err := rg.Generate(); err != nil {
		t.Fatalf("Generate() with diff failed: %v", err)
	}

	// Check diff.json exists
	diffData, err := os.ReadFile(filepath.Join(outputDir, "diff.json"))
	if err != nil {
		t.Fatalf("reading diff.json: %v", err)
	}
	var diff ReportDiff
	if err := json.Unmarshal(diffData, &diff); err != nil {
		t.Fatalf("invalid diff JSON: %v", err)
	}

	if len(diff.Changes) == 0 {
		t.Fatal("expected at least one change in diff")
	}

	foundThroughput := false
	for _, change := range diff.Changes {
		if change.Metric == "max_throughput" {
			foundThroughput = true
			if change.Change != "improved" {
				t.Errorf("expected throughput change to be 'improved' (500 vs 400), got %s", change.Change)
			}
		}
	}
	if !foundThroughput {
		t.Error("diff missing max_throughput comparison")
	}

	// Check diff.html exists
	diffHTML, err := os.ReadFile(filepath.Join(outputDir, "diff.html"))
	if err != nil {
		t.Fatalf("reading diff.html: %v", err)
	}
	if !strings.Contains(string(diffHTML), "Load Test Comparison") {
		t.Error("diff HTML missing expected title")
	}
}

func TestMetricsCollector_StartStop(t *testing.T) {
	outputDir := t.TempDir()

	mc, err := NewMetricsCollector(MetricsCollectorConfig{
		OutputDir: outputDir,
		Interval:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewMetricsCollector: %v", err)
	}

	if err := mc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := mc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestMetricsCollector_CollectsGoMetrics(t *testing.T) {
	outputDir := t.TempDir()

	mc, err := NewMetricsCollector(MetricsCollectorConfig{
		OutputDir: outputDir,
		Interval:  50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewMetricsCollector: %v", err)
	}

	if err := mc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for at least a couple collection cycles
	time.Sleep(200 * time.Millisecond)

	if err := mc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	snapshots := mc.Snapshots()
	if len(snapshots) < 2 {
		t.Fatalf("expected at least 2 snapshots, got %d", len(snapshots))
	}

	// Verify Go metrics are populated
	for i, snap := range snapshots {
		if snap.Go.Goroutines == 0 {
			t.Errorf("snapshot %d: Goroutines should be > 0", i)
		}
		if snap.Go.HeapAlloc == 0 {
			t.Errorf("snapshot %d: HeapAlloc should be > 0", i)
		}
		if snap.Timestamp.IsZero() {
			t.Errorf("snapshot %d: Timestamp should not be zero", i)
		}
	}
}

func TestMetricsCollector_FileRotation(t *testing.T) {
	outputDir := t.TempDir()

	mc, err := NewMetricsCollector(MetricsCollectorConfig{
		OutputDir: outputDir,
		Interval:  10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewMetricsCollector: %v", err)
	}

	// Set a very small max file size to trigger rotation
	mc.maxFileSize = 500 // 500 bytes

	if err := mc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for enough collections to trigger rotation
	time.Sleep(500 * time.Millisecond)

	if err := mc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Check that multiple files were created
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("reading output dir: %v", err)
	}

	jsonlCount := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl") {
			jsonlCount++
		}
	}

	if jsonlCount < 2 {
		t.Errorf("expected at least 2 JSONL files from rotation, got %d", jsonlCount)
	}
}

func TestMetricsCollector_Snapshots(t *testing.T) {
	outputDir := t.TempDir()

	mc, err := NewMetricsCollector(MetricsCollectorConfig{
		OutputDir: outputDir,
		Interval:  50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewMetricsCollector: %v", err)
	}

	if err := mc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	// Get snapshots while collector is still running
	snaps1 := mc.Snapshots()
	if len(snaps1) == 0 {
		t.Error("expected snapshots to be available while running")
	}

	time.Sleep(100 * time.Millisecond)

	// Get more snapshots - should have grown
	snaps2 := mc.Snapshots()
	if len(snaps2) <= len(snaps1) {
		t.Errorf("expected more snapshots over time: first=%d, second=%d", len(snaps1), len(snaps2))
	}

	if err := mc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Verify snapshots are a copy (modifying returned slice does not affect collector)
	finalSnaps := mc.Snapshots()
	originalLen := len(finalSnaps)
	_ = finalSnaps[:0] // Reslice to verify the original is not affected
	afterClear := mc.Snapshots()
	if len(afterClear) != originalLen {
		t.Errorf("Snapshots() should return a copy; expected %d, got %d", originalLen, len(afterClear))
	}
}

func TestLatencyTracker_ReservoirSampling(t *testing.T) {
	lt := newLatencyTracker()

	// Record more than reservoirSize samples
	total := reservoirSize * 3
	for i := range total {
		lt.record(time.Duration(i) * time.Microsecond)
	}

	// Verify count is tracked correctly
	lt.mu.Lock()
	count := lt.count
	sampleLen := len(lt.samples)
	lt.mu.Unlock()

	if count != int64(total) {
		t.Errorf("expected count=%d, got %d", total, count)
	}
	if sampleLen != reservoirSize {
		t.Errorf("expected samples to be capped at %d, got %d", reservoirSize, sampleLen)
	}

	// Verify percentiles are reasonable
	p50 := lt.percentile(50)
	p99 := lt.percentile(99)

	// With uniform distribution from 0 to total*us, p50 should be roughly
	// around total/2 microseconds. Allow wide tolerance since reservoir sampling
	// is approximate.
	expectedMedian := time.Duration(total/2) * time.Microsecond
	if p50 < expectedMedian/4 || p50 > expectedMedian*4 {
		t.Errorf("p50=%v seems unreasonable for uniform distribution (expected near %v)", p50, expectedMedian)
	}

	if p99 <= p50 {
		t.Errorf("p99 (%v) should be greater than p50 (%v)", p99, p50)
	}
}

func TestLatencyTracker_Empty(t *testing.T) {
	lt := newLatencyTracker()

	if got := lt.percentile(50); got != 0 {
		t.Errorf("percentile(50) on empty tracker = %v, want 0", got)
	}
	if got := lt.percentile(99); got != 0 {
		t.Errorf("percentile(99) on empty tracker = %v, want 0", got)
	}
}

func TestLatencyTracker_SingleSample(t *testing.T) {
	lt := newLatencyTracker()
	lt.record(42 * time.Millisecond)

	if got := lt.percentile(0); got != 42*time.Millisecond {
		t.Errorf("percentile(0) = %v, want 42ms", got)
	}
	if got := lt.percentile(50); got != 42*time.Millisecond {
		t.Errorf("percentile(50) = %v, want 42ms", got)
	}
	if got := lt.percentile(99); got != 42*time.Millisecond {
		t.Errorf("percentile(99) = %v, want 42ms", got)
	}
	if got := lt.percentile(100); got != 42*time.Millisecond {
		t.Errorf("percentile(100) = %v, want 42ms", got)
	}
}
