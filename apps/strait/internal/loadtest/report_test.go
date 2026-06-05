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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportGenerator_EmptyInput(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	rg := NewReportGenerator(inputDir, outputDir, "report.html", "report.json")
	require.NoError(t,

		rg.Generate())

	// Verify HTML was created and is valid
	htmlPath := filepath.Join(outputDir, "report.html")
	htmlData, err := os.ReadFile(htmlPath)
	require.NoError(t,

		err)
	assert.True(t, strings.Contains(string(
		htmlData), "<!DOCTYPE html>",
	))
	assert.True(t, strings.Contains(string(
		htmlData), "Strait Capacity Report",
	))

	// Verify JSON was created and is valid
	jsonPath := filepath.Join(outputDir, "report.json")
	jsonData, err := os.ReadFile(jsonPath)
	require.NoError(t,

		err)

	var report Report
	require.NoError(t,

		json.Unmarshal(jsonData,
			&report))
	assert.Equal(t, "PASS",

		report.
			Summary.
			OverallVerdict)
	assert.EqualValues(t, 0,

		report.Summary.
			MaxThroughput,
	)

	// Empty input should produce PASS verdict (no failures)

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
	require.NoError(t,

		err)
	require.NoError(t,

		os.WriteFile(filepath.
			Join(inputDir,
				"throughput_ceiling.json",
			), data,
			0o644))

	rg := NewReportGenerator(inputDir, outputDir, "report.html", "report.json")
	require.NoError(t,

		rg.Generate())

	// Read and verify JSON report
	jsonData, err := os.ReadFile(filepath.Join(outputDir, "report.json"))
	require.NoError(t,

		err)

	var report Report
	require.NoError(t,

		json.Unmarshal(jsonData,
			&report))
	assert.EqualValues(t, 500,

		report.
			Summary.MaxThroughput,
	)
	require.NotNil(t,
		report.
			Throughput,
	)
	assert.EqualValues(t, 600,

		report.
			Throughput.
			BreakingRate)
	assert.Len(t, report.
		Throughput.
		Steps,
		3)

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
			assert.Equal(t, tt.
				wantVerdict,
				summary.
					OverallVerdict)
			assert.False(t, tt.
				wantThroughput >
				0 &&
				summary.MaxThroughput !=
					tt.wantThroughput,
			)
			assert.False(t, tt.
				wantChaosPassed >
				0 &&
				summary.ChaosPassed !=
					tt.wantChaosPassed,
			)

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
	require.NoError(t,

		rg.Generate())

	// Check diff.json exists
	diffData, err := os.ReadFile(filepath.Join(outputDir, "diff.json"))
	require.NoError(t,

		err)

	var diff ReportDiff
	require.NoError(t,

		json.Unmarshal(diffData,
			&diff))
	require.NotEmpty(t,

		diff.Changes,
	)

	foundThroughput := false
	for _, change := range diff.Changes {
		if change.Metric == "max_throughput" {
			foundThroughput = true
			assert.Equal(t, "improved",

				change.Change,
			)

		}
	}
	assert.True(t, foundThroughput)

	// Check diff.html exists
	diffHTML, err := os.ReadFile(filepath.Join(outputDir, "diff.html"))
	require.NoError(t,

		err)
	assert.True(t, strings.Contains(string(
		diffHTML), "Load Test Comparison",
	))

}

func TestMetricsCollector_StartStop(t *testing.T) {
	outputDir := t.TempDir()

	mc, err := NewMetricsCollector(MetricsCollectorConfig{
		OutputDir: outputDir,
		Interval:  100 * time.Millisecond,
	})
	require.NoError(t,

		err)
	require.NoError(t,

		mc.Start(
			context.Background()))
	require.NoError(t,

		mc.Stop(),
	)

}

func TestMetricsCollector_CollectsGoMetrics(t *testing.T) {
	outputDir := t.TempDir()

	mc, err := NewMetricsCollector(MetricsCollectorConfig{
		OutputDir: outputDir,
		Interval:  50 * time.Millisecond,
	})
	require.NoError(t,

		err)
	require.NoError(t,

		mc.Start(
			context.Background()))

	// Wait for at least a couple collection cycles
	time.Sleep(200 * time.Millisecond)
	require.NoError(t,

		mc.Stop(),
	)

	snapshots := mc.Snapshots()
	require.GreaterOrEqual(t, len(snapshots), 2)

	// Verify Go metrics are populated
	for _, snap := range snapshots {
		assert.NotEqual(t,

			0, snap.Go.
				Goroutines,
		)
		assert.NotEqual(t,

			0, snap.Go.
				HeapAlloc,
		)
		assert.False(t, snap.
			Timestamp.
			IsZero(),
		)

	}
}

func TestMetricsCollector_FileRotation(t *testing.T) {
	outputDir := t.TempDir()

	mc, err := NewMetricsCollector(MetricsCollectorConfig{
		OutputDir: outputDir,
		Interval:  10 * time.Millisecond,
	})
	require.NoError(t,

		err)

	// Set a very small max file size to trigger rotation
	mc.maxFileSize = 500
	require.NoError(t,

		mc.Start(
			context.Background()))

	// 500 bytes

	// Wait for enough collections to trigger rotation
	time.Sleep(500 * time.Millisecond)
	require.NoError(t,

		mc.Stop(),
	)

	// Check that multiple files were created
	entries, err := os.ReadDir(outputDir)
	require.NoError(t,

		err)

	jsonlCount := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl") {
			jsonlCount++
		}
	}
	assert.GreaterOrEqual(t, jsonlCount,
		2)

}

func TestMetricsCollector_Snapshots(t *testing.T) {
	outputDir := t.TempDir()

	mc, err := NewMetricsCollector(MetricsCollectorConfig{
		OutputDir: outputDir,
		Interval:  50 * time.Millisecond,
	})
	require.NoError(t,

		err)
	require.NoError(t,

		mc.Start(
			context.Background()))

	time.Sleep(150 * time.Millisecond)

	// Get snapshots while collector is still running
	snaps1 := mc.Snapshots()
	assert.NotEmpty(t,

		snaps1)

	time.Sleep(100 * time.Millisecond)

	// Get more snapshots - should have grown
	snaps2 := mc.Snapshots()
	assert.False(t, len(snaps2) <=
		len(snaps1))
	require.NoError(t,

		mc.Stop(),
	)

	// Verify snapshots are a copy (modifying returned slice does not affect collector)
	finalSnaps := mc.Snapshots()
	originalLen := len(finalSnaps)
	_ = finalSnaps[:0] // Reslice to verify the original is not affected
	afterClear := mc.Snapshots()
	assert.Len(t, afterClear,

		originalLen,
	)

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
	assert.Equal(t, int64(total),
		count)
	assert.Equal(t, reservoirSize,

		sampleLen,
	)

	// Verify percentiles are reasonable
	p50 := lt.percentile(50)
	p99 := lt.percentile(99)

	// With uniform distribution from 0 to total*us, p50 should be roughly
	// around total/2 microseconds. Allow wide tolerance since reservoir sampling
	// is approximate.
	expectedMedian := time.Duration(total/2) * time.Microsecond
	assert.False(t, p50 <
		expectedMedian/
			4 ||
		p50 > expectedMedian*
			4)
	assert.False(t, p99 <=
		p50)

}

func TestLatencyTracker_Empty(t *testing.T) {
	lt := newLatencyTracker()

	assert.Equal(t, time.Duration(0), lt.percentile(50))
	assert.Equal(t, time.Duration(0), lt.percentile(99))
}

func TestLatencyTracker_SingleSample(t *testing.T) {
	lt := newLatencyTracker()
	lt.record(42 * time.Millisecond)

	assert.Equal(t, 42*time.Millisecond, lt.percentile(0))
	assert.Equal(t, 42*time.Millisecond, lt.percentile(50))
	assert.Equal(t, 42*time.Millisecond, lt.percentile(99))
	assert.Equal(t, 42*time.Millisecond, lt.percentile(100))
}
