package billing

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"time"
)

// ExportPeriod defines the time range for a usage export.
type ExportPeriod struct {
	From time.Time
	To   time.Time
}

// ExportCSV generates a CSV export of usage data for an org over the given period.
// The CSV columns are: date, project, runs, compute_cost_usd, ai_tokens, ai_cost_usd, total_usd.
func ExportCSV(ctx context.Context, store Store, orgID string, period ExportPeriod) ([]byte, error) {
	records, err := store.GetOrgUsageForPeriod(ctx, orgID, period.From, period.To)
	if err != nil {
		return nil, fmt.Errorf("getting usage records for export: %w", err)
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Write header.
	header := []string{"date", "project", "runs", "compute_cost_usd", "ai_tokens", "ai_cost_usd", "total_usd"}
	if err := w.Write(header); err != nil {
		return nil, fmt.Errorf("writing CSV header: %w", err)
	}

	for _, r := range records {
		totalMicro := r.ComputeCostMicro + r.AICostMicro
		row := []string{
			r.PeriodDate.Format("2006-01-02"),
			r.ProjectID,
			fmt.Sprintf("%d", r.RunsCount),
			microToUSDString(r.ComputeCostMicro),
			fmt.Sprintf("%d", r.AITokensTotal),
			microToUSDString(r.AICostMicro),
			microToUSDString(totalMicro),
		}
		if err := w.Write(row); err != nil {
			return nil, fmt.Errorf("writing CSV row: %w", err)
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("flushing CSV writer: %w", err)
	}

	return buf.Bytes(), nil
}

func microToUSDString(micro int64) string {
	return fmt.Sprintf("%.6f", float64(micro)/1_000_000)
}
