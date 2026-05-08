package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGrafanaDashboards_JSONValidAndInventoried(t *testing.T) {
	dashboardPaths, err := filepath.Glob(filepath.Join(moduleRoot(t), "monitoring", "grafana", "*.json"))
	if err != nil {
		t.Fatalf("glob dashboards: %v", err)
	}
	if len(dashboardPaths) != 9 {
		t.Fatalf("dashboard count = %d, want 9: %v", len(dashboardPaths), dashboardPaths)
	}

	inventory := loadMetricInventory(t)
	metricTokenRE := regexp.MustCompile(`strait_[a-z0-9_]+`)
	seenUIDs := map[string]string{}
	seenTitles := map[string]string{}

	for _, path := range dashboardPaths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		var doc struct {
			Dashboard struct {
				UID    string `json:"uid"`
				Title  string `json:"title"`
				Panels []struct {
					Title   string `json:"title"`
					Type    string `json:"type"`
					Targets []struct {
						Expr string `json:"expr"`
					} `json:"targets"`
				} `json:"panels"`
			} `json:"dashboard"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s does not parse as dashboard JSON: %v", filepath.Base(path), err)
		}
		if doc.Dashboard.UID == "" {
			t.Errorf("%s missing dashboard.uid", filepath.Base(path))
		}
		if doc.Dashboard.Title == "" {
			t.Errorf("%s missing dashboard.title", filepath.Base(path))
		}
		if previous := seenUIDs[doc.Dashboard.UID]; previous != "" {
			t.Errorf("%s duplicates uid %q from %s", filepath.Base(path), doc.Dashboard.UID, previous)
		}
		if previous := seenTitles[doc.Dashboard.Title]; previous != "" {
			t.Errorf("%s duplicates title %q from %s", filepath.Base(path), doc.Dashboard.Title, previous)
		}
		seenUIDs[doc.Dashboard.UID] = filepath.Base(path)
		seenTitles[doc.Dashboard.Title] = filepath.Base(path)
		if len(doc.Dashboard.Panels) < 4 {
			t.Errorf("%s has %d panels, want at least 4", filepath.Base(path), len(doc.Dashboard.Panels))
		}

		metricRefs := map[string]struct{}{}
		for _, token := range metricTokenRE.FindAllString(string(raw), -1) {
			metricRefs[normalizeDashboardMetricRef(token, inventory)] = struct{}{}
		}
		if len(metricRefs) == 0 {
			t.Errorf("%s references no Strait metrics", filepath.Base(path))
		}
		for metric := range metricRefs {
			if _, ok := inventory[metric]; !ok {
				t.Errorf("%s references metric %q that is not in metrics inventory", filepath.Base(path), metric)
			}
		}
	}
}

func TestGrafanaProvisioningFiles(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "monitoring", "grafana", "provisioning")
	files := []string{
		filepath.Join(root, "dashboards", "strait.yaml"),
		filepath.Join(root, "datasources", "prometheus.yaml"),
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var parsed map[string]any
		if err := yaml.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("%s is invalid YAML: %v", filepath.Base(path), err)
		}
		if parsed["apiVersion"] == nil {
			t.Errorf("%s missing apiVersion", filepath.Base(path))
		}
	}
}

func loadMetricInventory(t *testing.T) map[string]MetricInventoryEntry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(moduleRoot(t), "..", "docs", "operations", "metrics-inventory.mdx"))
	if err != nil {
		t.Fatalf("read metrics inventory: %v", err)
	}
	inventory, err := ParseMetricInventory(string(raw))
	if err != nil {
		t.Fatalf("parse metrics inventory: %v", err)
	}
	return inventory
}

func normalizeDashboardMetricRef(name string, inventory map[string]MetricInventoryEntry) string {
	if _, ok := inventory[name]; ok {
		return name
	}
	for _, suffix := range []string{"_bucket", "_count", "_sum"} {
		if trimmed, ok := strings.CutSuffix(name, suffix); ok {
			return trimmed
		}
	}
	return name
}
