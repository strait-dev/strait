package compute

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type grafanaDashboard struct {
	Dashboard struct {
		Title      string             `json:"title"`
		UID        string             `json:"uid"`
		Panels     []grafanaPanel     `json:"panels"`
		Templating grafanaTemplating  `json:"templating"`
	} `json:"dashboard"`
}

type grafanaTemplating struct {
	List []grafanaVariable `json:"list"`
}

type grafanaVariable struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type grafanaPanel struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Type        string          `json:"type"`
	Targets     []grafanaTarget `json:"targets"`
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

func TestDashboard_AllPanelsHaveDescriptions(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)
	for _, p := range d.Dashboard.Panels {
		if p.Description == "" {
			t.Errorf("panel %q is missing description", p.Title)
		}
	}
}

func TestDashboard_NorthStarMetricExists(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	found := false
	for _, p := range d.Dashboard.Panels {
		if strings.Contains(strings.ToLower(p.Title), "success rate") {
			found = true
			if len(p.Targets) == 0 {
				t.Error("north star panel has no targets")
				break
			}
			if !strings.Contains(p.Targets[0].Expr, "strait_run_transitions") {
				t.Errorf("north star should use strait_run_transitions")
			}
			break
		}
	}
	if !found {
		t.Error("dashboard missing success rate panel")
	}
}

func TestDashboard_HealthScoreExists(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	found := false
	for _, p := range d.Dashboard.Panels {
		if strings.Contains(strings.ToLower(p.Title), "health score") {
			found = true
			if p.Type != "gauge" {
				t.Errorf("Health Score should be a gauge, got %q", p.Type)
			}
			if len(p.Targets) == 0 {
				t.Error("Health Score panel has no targets")
			} else {
				expr := p.Targets[0].Expr
				// Must be a composite metric using multiple signals.
				if !strings.Contains(expr, "strait_run_transitions") {
					t.Error("Health Score should include success rate")
				}
				if !strings.Contains(expr, "strait_dispatch_errors") {
					t.Error("Health Score should include dispatch errors")
				}
			}
			break
		}
	}
	if !found {
		t.Error("dashboard missing Health Score (0-5) gauge")
	}
}

func TestDashboard_HasMinimumPanelCount(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)
	if len(d.Dashboard.Panels) < 40 {
		t.Errorf("expected at least 40 panels for comprehensive coverage, got %d", len(d.Dashboard.Panels))
	}
}

func TestDashboard_HasTemplateVariables(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	required := map[string]bool{"namespace": false, "interval": false, "pod": false}
	for _, v := range d.Dashboard.Templating.List {
		if _, ok := required[v.Name]; ok {
			required[v.Name] = true
		}
	}
	for name, found := range required {
		if !found {
			t.Errorf("dashboard missing template variable: %s", name)
		}
	}
}

func TestDashboard_HasHeatmaps(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)
	count := 0
	for _, p := range d.Dashboard.Panels {
		if p.Type == "heatmap" {
			count++
		}
	}
	if count < 2 {
		t.Errorf("expected at least 2 heatmap panels, got %d", count)
	}
}

func TestDashboard_HasK8sClusterPanels(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	k8sMetrics := map[string]bool{
		"kube_pod_status_phase":                       false,
		"kube_deployment_status_replicas":              false,
		"kube_pod_container_status_restarts_total":     false,
		"kube_node_status_condition":                   false,
		"node_disk":                                    false,
		"node_network":                                 false,
	}

	for _, p := range d.Dashboard.Panels {
		for _, tgt := range p.Targets {
			for metric := range k8sMetrics {
				if strings.Contains(tgt.Expr, metric) {
					k8sMetrics[metric] = true
				}
			}
		}
	}

	for metric, found := range k8sMetrics {
		if !found {
			t.Errorf("dashboard missing K8s metric: %s", metric)
		}
	}
}

func TestDashboard_CoversAllCategories(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	categories := map[string]bool{
		"success rate": false, "queue": false, "webhook": false,
		"workflow": false, "http": false, "worker pool": false,
		"db": false, "node": false, "scheduling": false,
		"disk": false, "network": false, "health score": false,
		"kyverno": false, "audit": false, "cost": false,
	}

	for _, p := range d.Dashboard.Panels {
		title := strings.ToLower(p.Title)
		for cat := range categories {
			if strings.Contains(title, cat) {
				categories[cat] = true
			}
		}
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
			if strings.Contains(expr, "node_disk") { categories["disk"] = true }
			if strings.Contains(expr, "node_network") { categories["network"] = true }
			if strings.Contains(expr, "kyverno") { categories["kyverno"] = true }
			if strings.Contains(expr, "audit") || strings.Contains(expr, "k3s-audit") { categories["audit"] = true }
			if strings.Contains(expr, "cost") || strings.Contains(expr, "currencyUSD") { categories["cost"] = true }
		}
	}

	for cat, found := range categories {
		if !found {
			t.Errorf("dashboard missing category: %s", cat)
		}
	}
}

func TestDashboard_HasSecurityPanels(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	securityMetrics := map[string]bool{
		"kyverno_policy_results_total":    false,
		"kyverno_admission_requests_total": false,
		"falco":                           false,
		"k3s-audit":                       false,
	}

	for _, p := range d.Dashboard.Panels {
		for _, tgt := range p.Targets {
			for metric := range securityMetrics {
				if strings.Contains(tgt.Expr, metric) {
					securityMetrics[metric] = true
				}
			}
		}
	}

	for metric, found := range securityMetrics {
		if !found {
			t.Errorf("dashboard missing security metric: %s", metric)
		}
	}
}

func TestDashboard_HasCostPanels(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	hasCost := false
	hasNodeCount := false
	for _, p := range d.Dashboard.Panels {
		title := strings.ToLower(p.Title)
		if strings.Contains(title, "cost") {
			hasCost = true
		}
		if strings.Contains(title, "node count") {
			hasNodeCount = true
		}
	}

	if !hasCost {
		t.Error("dashboard missing cost estimation panel")
	}
	if !hasNodeCount {
		t.Error("dashboard missing node count panel")
	}
}

func TestDashboard_HasLogPanels(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	logPanels := 0
	for _, p := range d.Dashboard.Panels {
		if p.Type == "logs" {
			logPanels++
		}
	}
	if logPanels < 2 {
		t.Errorf("expected at least 2 log panels (audit + falco), got %d", logPanels)
	}
}
