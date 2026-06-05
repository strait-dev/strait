package telemetry_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// artifactPath resolves the repository-relative path to a grafana artifact
// from this test file's working directory (apps/strait/internal/telemetry).
func artifactPath(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join("..", "..", "monitoring", "grafana", name)
	if _, err := os.Stat(p); err != nil {
		require.NoErrorf(t, err, "artifact %s not found", p)
	}
	return p
}

func TestAuditDashboard_JSONValid(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(artifactPath(t, "audit-events.json"))
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw,
		&parsed))

	require.Contains(t, parsed, "dashboard")

	// Each metric the dashboard documents must appear textually at least once.
	want := []string{
		"strait_audit_events_emitted_total",
		"strait_audit_events_dropped_total",
		"strait_audit_events_truncated_total",
		"strait_audit_events_deadlettered_total",
		"strait_audit_reclaimer_success_total",
		"strait_audit_reclaimer_failed_total",
		"strait_audit_retention_deleted_total",
		"strait_audit_siem_batch_size_items_bucket",
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
		assert.True(t,
			strings.Contains(body,
				m))

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
	require.NoError(t, err)

	var af alertFile
	require.NoError(t, yaml.Unmarshal(raw,
		&af))

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
	require.NotEmpty(t, af.Groups)

	metricIdent := regexp.MustCompile(`[a-zA-Z_:][a-zA-Z0-9_:]*`)

	for _, g := range af.Groups {
		for _, r := range g.Rules {
			if r.Alert == "" {
				assert.Failf(t, "rule missing alert name", "group=%q", g.Name)
				continue
			}
			if strings.TrimSpace(r.Expr) == "" {
				assert.Failf(t, "alert has empty expr", "%q", r.Alert)
				continue
			}
			assert.True(t,
				metricIdent.MatchString(r.Expr))

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
			assert.Equal(
				t, 0, depth)

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
		assert.True(t,
			seen[name])

	}
}
