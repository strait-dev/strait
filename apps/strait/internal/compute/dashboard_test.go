package compute

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type grafanaDashboard struct {
	Dashboard struct {
		Title  string          `json:"title"`
		UID    string          `json:"uid"`
		Panels []grafanaPanel  `json:"panels"`
	} `json:"dashboard"`
}

type grafanaPanel struct {
	Title   string          `json:"title"`
	Type    string          `json:"type"`
	Targets []grafanaTarget `json:"targets"`
}

type grafanaTarget struct {
	Expr         string `json:"expr"`
	LegendFormat string `json:"legendFormat"`
}

func loadDashboard(t *testing.T) grafanaDashboard {
	t.Helper()
	data, err := os.ReadFile("../../k8s/grafana-dashboard.json")
	if err != nil {
		t.Fatalf("failed to read dashboard: %v", err)
	}
	var d grafanaDashboard
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	return d
}

func TestDashboard_JSONValid(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)
	if d.Dashboard.Title == "" {
		t.Error("dashboard title is empty")
	}
	if d.Dashboard.UID == "" {
		t.Error("dashboard UID is empty")
	}
	if len(d.Dashboard.Panels) == 0 {
		t.Error("dashboard has no panels")
	}
}

func TestDashboard_AllPanelsHaveTargets(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)
	for _, p := range d.Dashboard.Panels {
		if len(p.Targets) == 0 {
			t.Errorf("panel %q has no targets", p.Title)
		}
		for i, tgt := range p.Targets {
			if tgt.Expr == "" {
				t.Errorf("panel %q target[%d] has empty expr", p.Title, i)
			}
		}
	}
}

func TestDashboard_NorthStarMetricExists(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	found := false
	for _, p := range d.Dashboard.Panels {
		if strings.Contains(strings.ToLower(p.Title), "north star") || strings.Contains(strings.ToLower(p.Title), "success rate") {
			found = true
			if len(p.Targets) == 0 {
				t.Error("north star panel has no targets")
				break
			}
			expr := p.Targets[0].Expr
			if !strings.Contains(expr, "strait_run_transitions") {
				t.Errorf("north star metric should use strait_run_transitions, got: %s", expr)
			}
			if !strings.Contains(expr, "completed") {
				t.Errorf("north star metric should track completed runs, got: %s", expr)
			}
			break
		}
	}
	if !found {
		t.Error("dashboard missing north star / success rate panel")
	}
}

func TestDashboard_HasMinimumPanelCount(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)
	if len(d.Dashboard.Panels) < 20 {
		t.Errorf("expected at least 20 panels for comprehensive coverage, got %d", len(d.Dashboard.Panels))
	}
}

func TestDashboard_CoversAllCategories(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	categories := map[string]bool{
		"success rate":  false,
		"queue":         false,
		"webhook":       false,
		"workflow":      false,
		"http":          false,
		"worker pool":   false,
		"db":            false,
		"node":          false,
		"scheduling":    false,
	}

	for _, p := range d.Dashboard.Panels {
		title := strings.ToLower(p.Title)
		for cat := range categories {
			if strings.Contains(title, cat) {
				categories[cat] = true
			}
		}
		// Also check targets for metric names.
		for _, tgt := range p.Targets {
			expr := strings.ToLower(tgt.Expr)
			if strings.Contains(expr, "webhook") { categories["webhook"] = true }
			if strings.Contains(expr, "workflow") { categories["workflow"] = true }
			if strings.Contains(expr, "queue") { categories["queue"] = true }
			if strings.Contains(expr, "http") { categories["http"] = true }
			if strings.Contains(expr, "pool_running") { categories["worker pool"] = true }
			if strings.Contains(expr, "db_pool") { categories["db"] = true }
			if strings.Contains(expr, "node_cpu") || strings.Contains(expr, "node_memory") { categories["node"] = true }
			if strings.Contains(expr, "scheduling") { categories["scheduling"] = true }
		}
	}

	for cat, found := range categories {
		if !found {
			t.Errorf("dashboard missing category: %s", cat)
		}
	}
}
