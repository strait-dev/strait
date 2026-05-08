package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

func TestGrafanaDashboards_PromQLShape(t *testing.T) {
	dashboardPaths, err := filepath.Glob(filepath.Join(moduleRoot(t), "monitoring", "grafana", "*.json"))
	if err != nil {
		t.Fatalf("glob dashboards: %v", err)
	}
	if len(dashboardPaths) == 0 {
		t.Fatal("no dashboard JSON files found")
	}

	var checked int
	for _, path := range dashboardPaths {
		exprs := dashboardExpressions(t, path)
		for _, expr := range exprs {
			checked++
			if strings.TrimSpace(expr) == "" {
				t.Errorf("%s has an empty PromQL expression", filepath.Base(path))
				continue
			}
			if err := validatePromQLShape(expr); err != nil {
				t.Errorf("%s invalid PromQL shape for %q: %v", filepath.Base(path), expr, err)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no dashboard PromQL expressions checked")
	}
}

func TestGrafanaDashboards_DatasourceAndIntervalVariables(t *testing.T) {
	dashboardPaths, err := filepath.Glob(filepath.Join(moduleRoot(t), "monitoring", "grafana", "*.json"))
	if err != nil {
		t.Fatalf("glob dashboards: %v", err)
	}
	for _, path := range dashboardPaths {
		doc := loadGrafanaDashboard(t, path)
		variables := map[string]string{}
		for _, variable := range doc.Dashboard.Templating.List {
			variables[variable.Name] = variable.Type
		}
		if variables["datasource"] != "datasource" {
			t.Errorf("%s missing datasource variable", filepath.Base(path))
		}
		if variables["interval"] != "interval" {
			t.Errorf("%s missing interval variable", filepath.Base(path))
		}

		for _, panel := range doc.Dashboard.Panels {
			if len(panel.Targets) == 0 {
				continue
			}
			if panel.Datasource.UID != "${datasource}" {
				t.Errorf("%s panel %q must use datasource variable, got %q", filepath.Base(path), panel.Title, panel.Datasource.UID)
			}
			for _, target := range panel.Targets {
				if target.Expr == "" {
					continue
				}
				if target.Datasource.UID != "${datasource}" {
					t.Errorf("%s panel %q target %q must use datasource variable, got %q", filepath.Base(path), panel.Title, target.RefID, target.Datasource.UID)
				}
				if strings.Contains(target.Expr, "[5m]") || strings.Contains(target.Expr, "[1h]") {
					t.Errorf("%s panel %q target %q has hard-coded short range selector: %s", filepath.Base(path), panel.Title, target.RefID, target.Expr)
				}
			}
		}
	}
}

func TestGrafanaDashboards_MetricRefsRegistered(t *testing.T) {
	dashboardPaths, err := filepath.Glob(filepath.Join(moduleRoot(t), "monitoring", "grafana", "*.json"))
	if err != nil {
		t.Fatalf("glob dashboards: %v", err)
	}
	registered := registeredMetricNames(t)
	metricTokenRE := regexp.MustCompile(`strait_[a-z0-9_]+`)

	for _, path := range dashboardPaths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, token := range metricTokenRE.FindAllString(string(raw), -1) {
			base := normalizePrometheusMetricToken(token)
			candidates := []string{token}
			if base != token {
				candidates = append(candidates, base)
			}
			if trimmed, ok := strings.CutSuffix(base, "_total"); ok {
				candidates = append(candidates, trimmed)
			}
			found := false
			for _, candidate := range candidates {
				if _, ok := registered[candidate]; ok {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s references metric %q that is not registered in source", filepath.Base(path), token)
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

func TestGrafanaSmokeScriptSyntax(t *testing.T) {
	script := filepath.Join(moduleRoot(t), "monitoring", "grafana", "smoke.sh")
	if info, err := os.Stat(script); err != nil {
		t.Fatalf("stat smoke script: %v", err)
	} else if info.Mode()&0o111 == 0 {
		t.Fatalf("smoke script is not executable: mode %s", info.Mode())
	}

	cmd := exec.Command("bash", "-n", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash -n smoke.sh failed: %v\n%s", err, out)
	}
}

func dashboardExpressions(t *testing.T, path string) []string {
	t.Helper()
	doc := loadGrafanaDashboard(t, path)
	var exprs []string
	for _, panel := range doc.Dashboard.Panels {
		for _, target := range panel.Targets {
			if target.Expr != "" {
				exprs = append(exprs, target.Expr)
			}
		}
	}
	return exprs
}

type grafanaDashboardDoc struct {
	Dashboard struct {
		Templating struct {
			List []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"list"`
		} `json:"templating"`
		Panels []struct {
			Title      string            `json:"title"`
			Datasource grafanaDatasource `json:"datasource"`
			Targets    []struct {
				RefID      string            `json:"refId"`
				Expr       string            `json:"expr"`
				Datasource grafanaDatasource `json:"datasource"`
			} `json:"targets"`
		} `json:"panels"`
	} `json:"dashboard"`
}

type grafanaDatasource struct {
	Type string `json:"type"`
	UID  string `json:"uid"`
}

func loadGrafanaDashboard(t *testing.T, path string) grafanaDashboardDoc {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc grafanaDashboardDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s does not parse as dashboard JSON: %v", filepath.Base(path), err)
	}
	return doc
}

func validatePromQLShape(expr string) error {
	if regexp.MustCompile(`\b(rate|increase)\s*\([^)]*\)\s+by\s*\(`).MatchString(expr) {
		return fmt.Errorf("aggregation must wrap range functions, e.g. sum by (...) (rate(metric[window]))")
	}
	if !balancedPromQLDelimiters(expr, '(', ')') {
		return fmt.Errorf("unbalanced parentheses")
	}
	if !balancedPromQLDelimiters(expr, '[', ']') {
		return fmt.Errorf("unbalanced range selector brackets")
	}
	if strings.Count(expr, "\"")%2 != 0 {
		return fmt.Errorf("unbalanced quotes")
	}
	return nil
}

func balancedPromQLDelimiters(expr string, open, close rune) bool {
	depth := 0
	inString := false
	escaped := false
	for _, c := range expr {
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch c {
		case open:
			depth++
		case close:
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
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
	return normalizePrometheusMetricToken(name)
}

func normalizePrometheusMetricToken(name string) string {
	for _, suffix := range []string{"_bucket", "_count", "_sum"} {
		if trimmed, ok := strings.CutSuffix(name, suffix); ok {
			return trimmed
		}
	}
	return name
}
