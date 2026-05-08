package telemetry

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestParseMetricInventory(t *testing.T) {
	t.Parallel()

	doc := "| `strait_queue_depth` | gauge | `queue` | bounded | <=20 | queue |\n"
	entries, err := ParseMetricInventory(doc)
	if err != nil {
		t.Fatalf("ParseMetricInventory: %v", err)
	}
	entry, ok := entries["strait_queue_depth"]
	if !ok {
		t.Fatal("missing strait_queue_depth")
	}
	if entry.Type != "gauge" || entry.Subsystem != "queue" {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestMetricsInventory_AllRegisteredMetricsAreDocumented(t *testing.T) {
	inventoryPath := filepath.Join(repoRoot(t), "apps", "docs", "operations", "metrics-inventory.mdx")
	raw, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	inventory, err := ParseMetricInventory(string(raw))
	if err != nil {
		t.Fatalf("parse inventory: %v", err)
	}

	registered := registeredMetricNames(t)
	for name := range registered {
		if _, ok := inventory[name]; !ok {
			t.Errorf("registered metric %q is missing from metrics-inventory.mdx", name)
		}
	}
}

func TestMetricsPolicy_NamingConvention(t *testing.T) {
	registered := registeredMetricNames(t)
	for name := range registered {
		if !metricNameRE.MatchString(name) {
			t.Errorf("metric %q does not match strait_<subsystem>_<measurement> convention", name)
		}
	}
}

func TestMetricsPolicy_HistogramSuffixes(t *testing.T) {
	registered := registeredMetricTypes(t)
	for name, typ := range registered {
		if typ != "histogram" {
			continue
		}
		if legacyHistogramName(name) {
			continue
		}
		if strings.HasSuffix(name, "_seconds") || strings.HasSuffix(name, "_bytes") || strings.HasSuffix(name, "_rows") || strings.HasSuffix(name, "_ratio") {
			continue
		}
		t.Errorf("histogram metric %q must include an explicit unit suffix", name)
	}
}

func legacyHistogramName(name string) bool {
	// Existing names that will be normalized in the follow-up metric
	// migration phase. Keeping the exception here lets the inventory land
	// before the runtime rename.
	switch name {
	case "strait_dequeue_duration",
		"strait_dispatch_duration",
		"strait_event_triggers_wait_duration",
		"strait_execution_trace_dispatch_duration",
		"strait_execution_trace_queue_wait_duration",
		"strait_http_request_duration",
		"strait_job_duration",
		"strait_queue_lag",
		"strait_run_duration",
		"strait_scheduler_cron_drift",
		"strait_worker_retry_attempts",
		"strait_workflow_step_wait_duration":
		return true
	default:
		return false
	}
}

func registeredMetricNames(t *testing.T) map[string]struct{} {
	t.Helper()
	types := registeredMetricTypes(t)
	names := make(map[string]struct{}, len(types))
	for name := range types {
		names[name] = struct{}{}
	}
	return names
}

func registeredMetricTypes(t *testing.T) map[string]string {
	t.Helper()

	root := filepath.Join(repoRoot(t), "apps", "strait", "internal")
	constructorRE := regexp.MustCompile(`(?s)(Int64Counter|Float64Histogram|Int64Gauge|Float64Gauge|Int64ObservableGauge|Int64ObservableCounter|Int64UpDownCounter)\(\s*"([^"]+)"`)
	types := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "proto" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range constructorRE.FindAllStringSubmatch(string(raw), -1) {
			name := NormalizePrometheusMetricName(match[2])
			if !strings.HasPrefix(name, "strait_") {
				continue
			}
			types[name] = metricKindFromConstructor(match[1])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan metric registrations: %v", err)
	}
	if len(types) == 0 {
		t.Fatal("no metric registrations found")
	}
	return types
}

func metricKindFromConstructor(constructor string) string {
	switch {
	case strings.Contains(constructor, "Histogram"):
		return "histogram"
	case strings.Contains(constructor, "Gauge"):
		return "gauge"
	case strings.Contains(constructor, "UpDownCounter"):
		return "updown_counter"
	default:
		return "counter"
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for range 10 {
		if _, err := os.Stat(filepath.Join(dir, "apps", "strait", "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root from %s", wd)
	return ""
}
