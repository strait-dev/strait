package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// auditMetricRegex matches any prometheus-style audit metric name
// referenced in a Grafana target expression. Captures the full metric
// name so we can assert it is registered by InitMetrics.
var auditMetricRegex = regexp.MustCompile(`\bstrait_audit_[a-z_]+\b`)

// metricsGoAuditNames returns the set of strait.audit.* instrument
// names declared in metrics.go. The OTel exporter translates dots to
// underscores for Prometheus, so "strait_audit_events_emitted_total"
// in Go is "strait_audit_events_emitted_total" on the scrape side.
// Observable gauges in Go are queried by their bare Prometheus name
// (no _total suffix) so we keep them in the set as-is.
func metricsGoAuditNames(t *testing.T) map[string]struct{} {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	metricsPath := filepath.Join(filepath.Dir(thisFile), "metrics.go")
	raw, err := os.ReadFile(metricsPath)
	require.NoError(t, err)

	// Match the OTel dotted names in strings to avoid scraping
	// comments that happen to mention a metric.
	otelPattern := regexp.MustCompile(`"strait\.audit\.[a-z_]+"`)

	set := map[string]struct{}{}
	for _, match := range otelPattern.FindAllString(string(raw), -1) {
		dotted := strings.Trim(match, `"`)
		prom := strings.ReplaceAll(dotted, ".", "_")
		set[prom] = struct{}{}
	}

	// The SIEM batch size histogram is registered under an
	// already-underscored name. Accept either the dotted or bare form.
	underscorePattern := regexp.MustCompile(`"strait_audit_[a-z_]+"`)
	for _, match := range underscorePattern.FindAllString(string(raw), -1) {
		set[strings.Trim(match, `"`)] = struct{}{}
	}
	return set
}

// dashboardAuditMetricRefs returns the set of strait_audit_* metric
// names referenced anywhere in the Grafana dashboard JSON. Panel
// expressions include both the base metric (strait_audit_X_total) and,
// for histograms, the bucket derivative (..._bucket). Both are valid
// scrapes; we normalize histogram bucket references back to the base
// name so registrations in metrics.go can cover them.
func dashboardAuditMetricRefs(t *testing.T) map[string]struct{} {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	dashPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "monitoring", "grafana", "audit-events.json")
	raw, err := os.ReadFile(dashPath)
	require.NoError(t, err)

	// Validate JSON so a drift between dashboard and test is not
	// caused by malformed JSON we are scanning with a regex.
	var decoded any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	matches := auditMetricRegex.FindAllString(string(raw), -1)
	set := map[string]struct{}{}
	for _, m := range matches {
		// Strip _bucket suffix on histogram references so we can
		// cross-check against the base histogram registration.
		base := strings.TrimSuffix(m, "_bucket")
		set[base] = struct{}{}
	}
	return set
}

// TestAuditDashboardDrift asserts every strait_audit_* metric referenced
// by the Grafana dashboard is registered in metrics.go. A dashboard
// panel that references a non-existent metric silently shows as "no
// data" in production — this test catches the drift at code review.
func TestAuditDashboardDrift(t *testing.T) {
	t.Parallel()

	refs := dashboardAuditMetricRefs(t)
	registered := metricsGoAuditNames(t)
	require.NotEmpty(t, refs)
	require.NotEmpty(t, registered)

	var missing []string
	for ref := range refs {
		if _, ok := registered[ref]; ok {
			continue
		}
		missing = append(missing, ref)
	}
	require.Empty(t, missing)
}
