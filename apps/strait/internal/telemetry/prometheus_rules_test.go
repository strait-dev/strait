package telemetry

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type prometheusRuleDoc struct {
	Groups []struct {
		Name  string `yaml:"name"`
		Rules []struct {
			Alert       string            `yaml:"alert"`
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
	rulesPath := locateRulesFile(t)
	rulesBytes, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("read rules file %s: %v", rulesPath, err)
	}

	var doc prometheusRuleDoc
	if err := yaml.Unmarshal(rulesBytes, &doc); err != nil {
		t.Fatalf("parse rules yaml: %v", err)
	}

	registered := registeredMetricNames(t)

	// Extract metric identifiers from each expression. We treat any
	// token starting with "strait_" and composed of [a-z0-9_] as a
	// metric name, then strip histogram suffixes.
	tokenRe := regexp.MustCompile(`strait_[a-z0-9_]+`)
	suffixes := []string{"_bucket", "_count", "_sum"}

	var alerts, checked int
	for _, g := range doc.Groups {
		for _, r := range g.Rules {
			if r.Alert == "" {
				continue
			}
			alerts++
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
				if !found {
					t.Errorf("alert %s references unknown metric %q (expr=%q)", r.Alert, tok, r.Expr)
				}
				checked++
			}
		}
	}

	if alerts < 10 {
		t.Errorf("expected at least 10 alert rules, got %d", alerts)
	}
	if checked == 0 {
		t.Fatalf("did not extract any metric tokens; regex or rule structure is wrong")
	}
}

func TestPrometheusRules_Shape(t *testing.T) {
	doc := loadPrometheusRuleDoc(t)
	alerts := map[string]struct{}{}

	for _, group := range doc.Groups {
		if group.Name == "" {
			t.Error("rule group is missing name")
		}
		for _, rule := range group.Rules {
			if rule.Alert == "" {
				t.Errorf("group %q has rule with empty alert", group.Name)
				continue
			}
			if _, exists := alerts[rule.Alert]; exists {
				t.Errorf("duplicate alert name %q", rule.Alert)
			}
			alerts[rule.Alert] = struct{}{}
			if strings.TrimSpace(rule.Expr) == "" {
				t.Errorf("alert %s has empty expression", rule.Alert)
			} else if err := validatePromQLShape(rule.Expr); err != nil {
				t.Errorf("alert %s has invalid expression shape: %v", rule.Alert, err)
			}
			if rule.For == "" {
				t.Errorf("alert %s is missing for duration", rule.Alert)
			}
			if rule.Labels["app"] != "strait" {
				t.Errorf("alert %s must set app=strait", rule.Alert)
			}
			switch rule.Labels["severity"] {
			case "warning", "page":
			default:
				t.Errorf("alert %s has unsupported severity %q", rule.Alert, rule.Labels["severity"])
			}
			if strings.TrimSpace(rule.Annotations["summary"]) == "" {
				t.Errorf("alert %s is missing summary annotation", rule.Alert)
			}
			if strings.TrimSpace(rule.Annotations["description"]) == "" {
				t.Errorf("alert %s is missing description annotation", rule.Alert)
			}
		}
	}

	if len(alerts) < 20 {
		t.Errorf("expected at least 20 alert rules, got %d", len(alerts))
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
	if err != nil {
		t.Fatalf("tmp file: %v", err)
	}
	raw, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("read rules: %v", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		t.Fatalf("write tmp rules: %v", err)
	}
	_ = tmp.Close()
	cmd := exec.Command(bin, "check", "rules", tmp.Name())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("promtool check rules failed: %v\n%s", err, out)
	}
}

func locateRulesFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(moduleRoot(t), "monitoring", "prometheus-rules.yaml")
}

func loadPrometheusRuleDoc(t *testing.T) prometheusRuleDoc {
	t.Helper()
	raw, err := os.ReadFile(locateRulesFile(t))
	if err != nil {
		t.Fatalf("read rules file: %v", err)
	}
	var doc prometheusRuleDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse rules yaml: %v", err)
	}
	return doc
}

// moduleRoot walks up from the test binary's working directory until it
// finds the apps/strait go.mod, returning that directory.
func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
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
	t.Fatalf("could not locate apps/strait module root from %s", wd)
	return ""
}
