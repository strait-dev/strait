package telemetry

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var metricNameRE = regexp.MustCompile(`^strait_[a-z0-9_]+$`)

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
		if strings.HasSuffix(name, "_seconds") || strings.HasSuffix(name, "_bytes") || strings.HasSuffix(name, "_rows") || strings.HasSuffix(name, "_ratio") || strings.HasSuffix(name, "_number") || strings.HasSuffix(name, "_items") || strings.HasSuffix(name, "_microusd") {
			continue
		}
		t.Errorf("histogram metric %q must include an explicit unit suffix", name)
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
	constructorRE := regexp.MustCompile(`(?s)(Int64Counter|Int64Histogram|Float64Histogram|Int64Gauge|Float64Gauge|Int64ObservableGauge|Int64ObservableCounter|Int64UpDownCounter)\(\s*"([^"]+)"`)
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
			name := normalizePrometheusMetricName(match[2])
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

func normalizePrometheusMetricName(name string) string {
	return strings.ReplaceAll(name, ".", "_")
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
