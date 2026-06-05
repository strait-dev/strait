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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestGrafanaDashboards_JSONValidAndRegisteredMetrics(t *testing.T) {
	dashboardPaths, err := filepath.Glob(filepath.Join(moduleRoot(t), "monitoring", "grafana", "*.json"))
	require.NoError(t, err)
	require.Len(t, dashboardPaths,
		10,
	)

	registeredMetrics := registeredMetricNames(t)
	metricTokenRE := regexp.MustCompile(`strait_[a-z0-9_]+`)
	seenUIDs := map[string]string{}
	seenTitles := map[string]string{}

	for _, path := range dashboardPaths {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)

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
		require.NoError(t, json.Unmarshal(raw,
			&doc))
		assert.NotEmpty(t, doc.
			Dashboard.
			UID,
		)
		assert.NotEmpty(t, doc.
			Dashboard.
			Title,
		)
		assert.Empty(t, seenUIDs[doc.
			Dashboard.
			UID])
		assert.Empty(t, seenTitles[doc.
			Dashboard.
			Title])

		seenUIDs[doc.Dashboard.UID] = filepath.Base(path)
		seenTitles[doc.Dashboard.Title] = filepath.Base(path)
		assert.GreaterOrEqual(t,
			len(doc.
				Dashboard.
				Panels,
			), 4)

		metricRefs := map[string]struct{}{}
		for _, token := range metricTokenRE.FindAllString(string(raw), -1) {
			metricRefs[normalizeDashboardMetricRef(token, registeredMetrics)] = struct{}{}
		}
		assert.NotEmpty(t, metricRefs)

		for metric := range metricRefs {
			if _, ok := registeredMetrics[metric]; !ok {
				assert.Failf(t, "dashboard references unregistered metric", "%s %q", filepath.Base(path), metric)
			}
		}
	}
}

func TestGrafanaDashboards_PromQLShape(t *testing.T) {
	dashboardPaths, err := filepath.Glob(filepath.Join(moduleRoot(t), "monitoring", "grafana", "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, dashboardPaths)

	var checked int
	for _, path := range dashboardPaths {
		exprs := dashboardExpressions(t, path)
		for _, expr := range exprs {
			checked++
			if strings.TrimSpace(expr) == "" {
				assert.Failf(t, "dashboard has an empty PromQL expression", "%s", filepath.Base(path))
				continue
			}
			require.NoError(t, validatePromQLShape(
				expr))
		}
	}
	require.NotEqual(t, 0, checked)
}

func TestGrafanaDashboards_DatasourceAndIntervalVariables(t *testing.T) {
	dashboardPaths, err := filepath.Glob(filepath.Join(moduleRoot(t), "monitoring", "grafana", "*.json"))
	require.NoError(t, err)

	for _, path := range dashboardPaths {
		doc := loadGrafanaDashboard(t, path)
		variables := map[string]string{}
		for _, variable := range doc.Dashboard.Templating.List {
			variables[variable.Name] = variable.Type
		}
		assert.Equal(t, "datasource",
			variables["datasource"])
		assert.Equal(t, "interval",
			variables["interval"],
		)

		for _, panel := range doc.Dashboard.Panels {
			if len(panel.Targets) == 0 {
				continue
			}
			assert.Equal(t, "${datasource}",

				panel.
					Datasource.
					UID)

			for _, target := range panel.Targets {
				if target.Expr == "" {
					continue
				}
				assert.Equal(t, "${datasource}",

					target.
						Datasource.
						UID)
				assert.False(t, strings.Contains(target.
					Expr, "[5m]",
				) || strings.Contains(target.
					Expr,
					"[1h]"))
			}
		}
	}
}

func TestGrafanaDashboards_MetricRefsRegistered(t *testing.T) {
	dashboardPaths, err := filepath.Glob(filepath.Join(moduleRoot(t), "monitoring", "grafana", "*.json"))
	require.NoError(t, err)

	registered := registeredMetricNames(t)
	metricTokenRE := regexp.MustCompile(`strait_[a-z0-9_]+`)

	for _, path := range dashboardPaths {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)

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
			assert.True(t, found)
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
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, yaml.Unmarshal(raw,
			&parsed))
		assert.NotNil(t, parsed["apiVersion"])
	}
}

func TestGrafanaSmokeScriptSyntax(t *testing.T) {
	scripts := []string{
		filepath.Join(moduleRoot(t), "monitoring", "grafana", "smoke.sh"),
		filepath.Join(moduleRoot(t), "monitoring", "check-scrape-coverage.sh"),
	}
	for _, script := range scripts {
		if info, err := os.Stat(script); err != nil {
			require.NoErrorf(t, err, "stat script %s", script)
		} else if info.Mode()&0o111 == 0 {
			require.Failf(t, "script is not executable", "%s mode %s", filepath.Base(script), info.Mode())
		}

		cmd := exec.Command("bash", "-n", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			require.NoErrorf(t, err, "bash -n %s failed: %s", filepath.Base(script), out)
		}
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
	require.NoError(t, err)

	var doc grafanaDashboardDoc
	require.NoError(t, json.Unmarshal(raw,
		&doc))

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

func normalizeDashboardMetricRef(name string, registeredMetrics map[string]struct{}) string {
	if _, ok := registeredMetrics[name]; ok {
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
