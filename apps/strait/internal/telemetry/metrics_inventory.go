package telemetry

import (
	"fmt"
	"regexp"
	"strings"
)

type MetricInventoryEntry struct {
	Name              string
	Type              string
	Labels            string
	AllowedValues     string
	CardinalityBudget string
	Subsystem         string
}

var (
	metricNameRE = regexp.MustCompile(`^strait_[a-z0-9_]+$`)
	tableRowRE   = regexp.MustCompile(`^\|\s*` + "`" + `([^` + "`" + `]+)` + "`" + `\s*\|`)
)

func ParseMetricInventory(markdown string) (map[string]MetricInventoryEntry, error) {
	entries := map[string]MetricInventoryEntry{}
	for lineNo, raw := range strings.Split(markdown, "\n") {
		line := strings.TrimSpace(raw)
		matches := tableRowRE.FindStringSubmatch(line)
		if len(matches) != 2 {
			continue
		}

		cols := splitMarkdownTableRow(line)
		if len(cols) != 6 {
			return nil, fmt.Errorf("metric inventory line %d: expected 6 columns, got %d", lineNo+1, len(cols))
		}
		name := strings.Trim(cols[0], "` ")
		if !metricNameRE.MatchString(name) {
			return nil, fmt.Errorf("metric inventory line %d: invalid metric name %q", lineNo+1, name)
		}
		entries[name] = MetricInventoryEntry{
			Name:              name,
			Type:              strings.TrimSpace(cols[1]),
			Labels:            strings.TrimSpace(cols[2]),
			AllowedValues:     strings.TrimSpace(cols[3]),
			CardinalityBudget: strings.TrimSpace(cols[4]),
			Subsystem:         strings.TrimSpace(cols[5]),
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("metric inventory contains no metric rows")
	}
	return entries, nil
}

func splitMarkdownTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

func NormalizePrometheusMetricName(name string) string {
	return strings.ReplaceAll(name, ".", "_")
}
