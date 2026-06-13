package telemetry

import (
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

type prometheusRuleDoc struct {
	Groups []struct {
		Name  string `yaml:"name"`
		Rules []struct {
			Alert       string            `yaml:"alert"`
			Record      string            `yaml:"record"`
			Expr        string            `yaml:"expr"`
			For         string            `yaml:"for"`
			Labels      map[string]string `yaml:"labels"`
			Annotations map[string]string `yaml:"annotations"`
		} `yaml:"rules"`
	} `yaml:"groups"`
}

// TestPrometheusRules_MetricsExist guards against alert-rule / metric
// drift: every metric name referenced in monitoring/prometheus-rules.yaml must
// appear somewhere in the telemetry source tree. If a metric is renamed
// or removed without updating the rules file, this test fails.
func TestPrometheusRules_MetricsExist(t *testing.T) {
	doc := loadPrometheusRuleDoc(t)
	registered := registeredMetricNames(t)

	// Extract metric identifiers from each expression. We treat any
	// token starting with "strait_" and composed of [a-z0-9_] as a
	// metric name, then strip histogram suffixes.
	tokenRe := regexp.MustCompile(`strait_[a-z0-9_]+`)
	suffixes := []string{"_bucket", "_count", "_sum"}

	var rules, checked int
	for _, g := range doc.Groups {
		for _, r := range g.Rules {
			if r.Alert == "" && r.Record == "" {
				continue
			}
			rules++
			for _, tok := range tokenRe.FindAllString(r.Expr, -1) {
				base := tok
				for _, s := range suffixes {
					if trimmed, ok := strings.CutSuffix(base, s); ok {
						base = trimmed
						break
					}
				}
				// _total counters are defined verbatim in source (e.g.
				// strait_webhook_deliveries_total) or as OTel dotted
				// counters where the exporter appends _total. Try both.
				candidates := []string{base}
				if trimmed, ok := strings.CutSuffix(base, "_total"); ok {
					candidates = append(candidates, trimmed)
				}
				found := false
				for _, c := range candidates {
					if _, ok := registered[c]; ok {
						found = true
						break
					}
				}
				assert.True(t,
					found)

				checked++
			}
		}
	}
	assert.GreaterOrEqual(t,
		rules, 10)
	require.NotEqual(t, 0, checked)
}

func TestPrometheusRules_Shape(t *testing.T) {
	doc := loadPrometheusRuleDoc(t)
	alerts := map[string]struct{}{}
	records := map[string]struct{}{}

	for _, group := range doc.Groups {
		assert.NotEmpty(t, group.
			Name)

		for _, rule := range group.Rules {
			switch {
			case rule.Alert != "":
				if _, exists := alerts[rule.Alert]; exists {
					assert.Failf(t, "duplicate alert name", "%q", rule.Alert)
				}
				alerts[rule.Alert] = struct{}{}
				if strings.TrimSpace(rule.Expr) == "" {
					assert.Failf(t, "alert has empty expression", "%s", rule.Alert)
				} else if err := validatePromQLShape(rule.Expr); err != nil {
					assert.NoErrorf(t, err, "alert %s has invalid expression shape", rule.Alert)
				}
				if rule.For == "" {
					assert.Failf(t, "alert is missing for duration", "%s", rule.Alert)
				}
				assert.Equal(t, "strait", rule.Labels["app"])
				assert.Equal(t, "platform", rule.Labels["owner"])
				switch rule.Labels["severity"] {
				case "warning", "page":
				default:
					assert.Failf(t, "alert has unsupported severity", "%s %q", rule.Alert, rule.Labels["severity"])
				}
				if strings.TrimSpace(rule.Annotations["summary"]) == "" {
					assert.Failf(t, "alert is missing summary annotation", "%s", rule.Alert)
				}
				if strings.TrimSpace(rule.Annotations["description"]) == "" {
					assert.Failf(t, "alert is missing description annotation", "%s", rule.Alert)
				}
				if _, ok := rule.Annotations["runbook_url"]; ok {
					assert.Failf(t, "alert has runbook_url annotation; runbooks are intentionally deferred", "%s", rule.Alert)
				}
			case rule.Record != "":
				if _, exists := records[rule.Record]; exists {
					assert.Failf(t, "duplicate recording rule name", "%q", rule.Record)
				}
				records[rule.Record] = struct{}{}
				if strings.TrimSpace(rule.Expr) == "" {
					assert.Failf(t, "recording rule has empty expression", "%s", rule.Record)
				} else if err := validatePromQLShape(rule.Expr); err != nil {
					assert.NoErrorf(t, err, "recording rule %s has invalid expression shape", rule.Record)
				}
			default:
				assert.Failf(t, "group has rule with neither alert nor record", "%q", group.Name)
			}
		}
	}
	assert.GreaterOrEqual(t,
		len(alerts), 20)
	assert.GreaterOrEqual(t,
		len(records), 10)
}

func TestPrometheusRules_RecordingRulesPresent(t *testing.T) {
	doc := loadPrometheusRuleDoc(t)
	want := map[string]struct{}{
		"strait:http_request_rate5m":                  {},
		"strait:http_request_p95_seconds5m":           {},
		"strait:http_request_p99_seconds5m":           {},
		"strait:http_error_rate5m":                    {},
		"strait:queue_depth":                          {},
		"strait:pgque_consumer_lag_ticks":             {},
		"strait:queue_oldest_queued_p95_seconds5m":    {},
		"strait:queue_claim_p95_seconds5m":            {},
		"strait:worker_dispatch_p95_seconds5m":        {},
		"strait:worker_retry_rate5m":                  {},
		"strait:workflow_step_p95_seconds5m":          {},
		"strait:workflow_compensation_failure_rate5m": {},
		"strait:scheduler_loop_p95_seconds5m":         {},
		"strait:scheduler_skew_seconds":               {},
		"strait:auth_failure_rate5m":                  {},
		"strait:redis_command_p95_seconds5m":          {},
		"strait:cdc_shared_dedupe_fallback_rate5m":    {},
		"strait:billing_quota_block_rate5m":           {},
		"strait:trigger_admission_direct_ratio5m":     {},
		"strait:trigger_admission_fallback_rate5m":    {},
		"strait:trigger_dependency_evaluated_ratio5m": {},
	}

	got := map[string]string{}
	for _, group := range doc.Groups {
		for _, rule := range group.Rules {
			if rule.Record != "" {
				got[rule.Record] = rule.Expr
			}
		}
	}
	for record := range want {
		expr, ok := got[record]
		if !ok {
			assert.Failf(t, "recording rule missing", "%s", record)
			continue
		}
		assert.NotEmpty(t, expr)
	}
}

// TestPrometheusRules_Promtool runs `promtool check rules` when the
// binary is available. Skips otherwise — CI is not required to install
// it.
func TestPrometheusRules_Promtool(t *testing.T) {
	bin, err := exec.LookPath("promtool")
	if err != nil {
		t.Skip("promtool not installed; skipping syntactic rule check")
	}
	rulesPath := locateRulesFile(t)
	tmp, err := os.CreateTemp(t.TempDir(), "strait-rules-*.yaml")
	require.NoError(t, err)

	raw, err := os.ReadFile(rulesPath)
	require.NoError(t, err)

	_, err = tmp.Write(raw)
	require.NoError(t, err)
	_ = tmp.Close()
	cmd := exec.Command(bin, "check", "rules", tmp.Name())
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "%s", out)
}

func locateRulesFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(moduleRoot(t), "monitoring", "prometheus-rules.yaml")
}

func loadPrometheusRuleDoc(t *testing.T) prometheusRuleDoc {
	t.Helper()
	raw, err := os.ReadFile(locateRulesFile(t))
	require.NoError(t, err)

	var doc prometheusRuleDoc
	require.NoError(t, yaml.Unmarshal(raw, &doc))

	return doc
}

// moduleRoot walks up from the test binary's working directory until it
// finds the apps/strait go.mod, returning that directory.
func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)

	dir := wd
	for range 10 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	require.Failf(t, "could not locate apps/strait module root", "%s", wd)
	return ""
}
