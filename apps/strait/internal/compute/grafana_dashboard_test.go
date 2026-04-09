package compute

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestGrafanaDashboard_ValidJSON(t *testing.T) {
	data, err := os.ReadFile("../../k8s/grafana-dashboard.json")
	if err != nil {
		t.Skipf("dashboard file not found: %v", err)
	}

	var dashboard map[string]any
	if err := json.Unmarshal(data, &dashboard); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	db, ok := dashboard["dashboard"].(map[string]any)
	if !ok {
		t.Fatal("missing 'dashboard' key")
	}

	panels, ok := db["panels"].([]any)
	if !ok {
		t.Fatal("missing 'panels' array")
	}

	if len(panels) < 5 {
		t.Errorf("only %d panels, want at least 5", len(panels))
	}
}

func TestGrafanaDashboard_AllPanelsHaveQueries(t *testing.T) {
	data, err := os.ReadFile("../../k8s/grafana-dashboard.json")
	if err != nil {
		t.Skipf("dashboard file not found: %v", err)
	}

	var dashboard map[string]any
	_ = json.Unmarshal(data, &dashboard)

	db := dashboard["dashboard"].(map[string]any)
	panels := db["panels"].([]any)

	for i, p := range panels {
		panel := p.(map[string]any)
		title := panel["title"].(string)
		if panel["type"] == "row" {
			continue // row panels are section dividers with no queries
		}
		targets, ok := panel["targets"].([]any)
		if !ok || len(targets) == 0 {
			t.Errorf("panel %d (%q) has no targets", i, title)
			continue
		}
		for j, tgt := range targets {
			target := tgt.(map[string]any)
			expr, ok := target["expr"].(string)
			if !ok || expr == "" {
				t.Errorf("panel %d (%q) target %d has empty expr", i, title, j)
			}
		}
	}
}

func TestGrafanaDashboard_MetricNamesMatchCode(t *testing.T) {
	data, err := os.ReadFile("../../k8s/grafana-dashboard.json")
	if err != nil {
		t.Skipf("dashboard file not found: %v", err)
	}

	raw := string(data)

	// These metrics are defined in telemetry/metrics.go.
	expectedMetrics := []string{
		"strait_k8s_job_create_total",
		"strait_k8s_job_create_duration_seconds",
		"strait_k8s_job_wait_duration_seconds",
		"strait_k8s_pod_scheduling_duration_seconds",
		"strait_k8s_jobs_active",
		"strait_compute_fallback_total",
	}

	for _, m := range expectedMetrics {
		if !strings.Contains(raw, m) {
			t.Errorf("metric %q not referenced in dashboard", m)
		}
	}
}
