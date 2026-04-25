package telemetry_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// artifactPath resolves the repository-relative path to a grafana artifact
// from this test file's working directory (apps/strait/internal/telemetry).
func artifactPath(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join("..", "..", "k8s", "grafana", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("artifact %s not found: %v", p, err)
	}
	return p
}

func TestAuditDashboard_JSONValid(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(artifactPath(t, "audit-events.json"))
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("dashboard JSON does not parse: %v", err)
	}
	if _, ok := parsed["dashboard"]; !ok {
		t.Fatalf("dashboard JSON missing top-level 'dashboard' key")
	}

	// Each metric the dashboard documents must appear textually at least once.
	want := []string{
		"strait_audit_events_emitted_total",
		"strait_audit_events_dropped_total",
		"strait_audit_events_truncated_total",
		"strait_audit_events_deadlettered_total",
		"strait_audit_reclaimer_success_total",
		"strait_audit_reclaimer_failed_total",
		"strait_audit_retention_deleted_total",
		"strait_audit_siem_batch_size_bucket",
		"strait_audit_siem_forwarded_total",
		"strait_audit_siem_failed_total",
		"strait_audit_siem_circuit_open_total",
		"strait_audit_drainer_queue_depth",
		"strait_audit_drainer_queue_capacity",
		"strait_audit_events_export_capped_total",
		"strait_audit_chain_verify_failed_total",
	}
	body := string(raw)
	for _, m := range want {
		if !strings.Contains(body, m) {
			t.Errorf("dashboard JSON missing expected metric reference: %s", m)
		}
	}
}

// alertFile models the minimal Prometheus rule file shape we need to inspect.
type alertFile struct {
	Groups []struct {
		Name  string `yaml:"name"`
		Rules []struct {
			Alert  string            `yaml:"alert"`
			Expr   string            `yaml:"expr"`
			For    string            `yaml:"for"`
			Labels map[string]string `yaml:"labels"`
		} `yaml:"rules"`
	} `yaml:"groups"`
}

func loadAlerts(t *testing.T) alertFile {
	t.Helper()
	raw, err := os.ReadFile(artifactPath(t, "audit-alerts.yaml"))
	if err != nil {
		t.Fatalf("read alerts: %v", err)
	}
	var af alertFile
	if err := yaml.Unmarshal(raw, &af); err != nil {
		t.Fatalf("alerts YAML does not parse: %v", err)
	}
	return af
}

// TestAuditAlerts_PromQLParses is a regex-based sanity check — we do not pull
// in github.com/prometheus/prometheus because it would bloat go.mod for a
// single static-validation test. The check ensures every alert expression is
// non-empty, references at least one metric identifier, and uses balanced
// parentheses. Real PromQL validity is enforced by Prometheus at load time.
func TestAuditAlerts_PromQLParses(t *testing.T) {
	t.Parallel()

	af := loadAlerts(t)
	if len(af.Groups) == 0 {
		t.Fatal("no alert groups parsed")
	}

	metricIdent := regexp.MustCompile(`[a-zA-Z_:][a-zA-Z0-9_:]*`)

	for _, g := range af.Groups {
		for _, r := range g.Rules {
			if r.Alert == "" {
				t.Errorf("rule in group %q missing alert name", g.Name)
				continue
			}
			if strings.TrimSpace(r.Expr) == "" {
				t.Errorf("alert %q has empty expr", r.Alert)
				continue
			}
			if !metricIdent.MatchString(r.Expr) {
				t.Errorf("alert %q expr has no metric-like identifier: %s", r.Alert, r.Expr)
			}
			// Balanced parentheses.
			depth := 0
			for _, c := range r.Expr {
				switch c {
				case '(':
					depth++
				case ')':
					depth--
				}
				if depth < 0 {
					break
				}
			}
			if depth != 0 {
				t.Errorf("alert %q expr has unbalanced parentheses: %s", r.Alert, r.Expr)
			}
		}
	}
}

func TestAuditAlerts_RequiredAlertsPresent(t *testing.T) {
	t.Parallel()

	af := loadAlerts(t)
	seen := map[string]bool{}
	for _, g := range af.Groups {
		for _, r := range g.Rules {
			seen[r.Alert] = true
		}
	}

	required := []string{
		"AuditDLQRising",
		"AuditDrainerSaturated",
		"AuditSIEMForwardFailing",
		"AuditChainVerificationFailed",
	}
	for _, name := range required {
		if !seen[name] {
			t.Errorf("required alert %q not present in audit-alerts.yaml", name)
		}
	}
}
