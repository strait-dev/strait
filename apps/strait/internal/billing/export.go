package billing

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"time"
	"unicode"

	"github.com/go-pdf/fpdf"
)

const (
	maxUsageExportPeriod       = 370 * 24 * time.Hour
	maxUsageExportRows         = 10_000
	maxUsageExportQueryTimeout = 15 * time.Second
)

var (
	ErrUsageExportTooLarge          = errors.New("usage export exceeds maximum row limit")
	ErrUsageExportRequiresBoundedDB = errors.New("usage export requires bounded usage query support")
)

type boundedOrgUsagePeriodStore interface {
	GetOrgUsageForPeriodLimited(ctx context.Context, orgID string, from, to time.Time, limit int) ([]UsageRecord, error)
}

// ExportPeriod defines the time range for a usage export.
type ExportPeriod struct {
	From time.Time
	To   time.Time
}

// ExportCSV generates a CSV export of usage data for an org over the given period.
// The CSV columns are: date, project, runs, orchestration_cost_usd, total_usd.
func ExportCSV(ctx context.Context, store Store, orgID string, period ExportPeriod) ([]byte, error) {
	if err := validateExportPeriod(period); err != nil {
		return nil, err
	}
	records, err := getUsageRecordsForExport(ctx, store, orgID, period)
	if err != nil {
		return nil, fmt.Errorf("getting usage records for export: %w", err)
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Write header.
	header := []string{"date", "project", "runs", "orchestration_cost_usd", "total_usd"}
	if err := w.Write(header); err != nil {
		return nil, fmt.Errorf("writing CSV header: %w", err)
	}

	for _, r := range records {
		row := []string{
			r.PeriodDate.Format("2006-01-02"),
			escapeCSVFormulaCell(r.ProjectID),
			fmt.Sprintf("%d", r.RunsCount),
			microToUSDString(r.ComputeCostMicro),
			microToUSDString(r.ComputeCostMicro),
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

// ExportPDF generates a PDF export of usage data for an org over the given period.
func ExportPDF(ctx context.Context, store Store, orgID string, period ExportPeriod) ([]byte, error) {
	if err := validateExportPeriod(period); err != nil {
		return nil, err
	}
	records, err := getUsageRecordsForExport(ctx, store, orgID, period)
	if err != nil {
		return nil, fmt.Errorf("getting usage records for PDF export: %w", err)
	}

	// Fetch subscription to determine plan tier.
	planTier := "free"
	sub, err := store.GetOrgSubscription(ctx, orgID)
	if err == nil && sub != nil {
		planTier = sub.PlanTier
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	// Header.
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 10, "Strait Usage Report", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("Organization: %s", orgID), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("Plan: %s", planTier), "", 1, "L", false, 0, "")

	// Period line.
	pdf.Ln(4)
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("Period: %s to %s",
		period.From.Format("2006-01-02"), period.To.Format("2006-01-02")), "", 1, "L", false, 0, "")

	// Summary box.
	var totalRuns int64
	var totalComputeMicro int64
	for _, r := range records {
		totalRuns += r.RunsCount
		totalComputeMicro += r.ComputeCostMicro
	}

	pdf.Ln(6)
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(0, 8, "Summary", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("Total Runs: %d", totalRuns), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("Orchestration Cost: $%s", microToUSDString(totalComputeMicro)), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("Total: $%s", microToUSDString(totalComputeMicro)), "", 1, "L", false, 0, "")

	// Detail table.
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(0, 8, "Detail", "", 1, "L", false, 0, "")

	// Table header.
	colWidths := []float64{28, 48, 22, 38, 38}
	headers := []string{"Date", "Project", "Runs", "Orchestration ($)", "Total ($)"}
	pdf.SetFont("Helvetica", "B", 9)
	for i, h := range headers {
		pdf.CellFormat(colWidths[i], 7, h, "1", 0, "C", false, 0, "")
	}
	pdf.Ln(-1)

	// Table rows.
	pdf.SetFont("Helvetica", "", 8)
	for _, r := range records {
		pdf.CellFormat(colWidths[0], 6, r.PeriodDate.Format("2006-01-02"), "1", 0, "C", false, 0, "")
		pdf.CellFormat(colWidths[1], 6, r.ProjectID, "1", 0, "L", false, 0, "")
		pdf.CellFormat(colWidths[2], 6, fmt.Sprintf("%d", r.RunsCount), "1", 0, "R", false, 0, "")
		pdf.CellFormat(colWidths[3], 6, microToUSDString(r.ComputeCostMicro), "1", 0, "R", false, 0, "")
		pdf.CellFormat(colWidths[4], 6, microToUSDString(r.ComputeCostMicro), "1", 0, "R", false, 0, "")
		pdf.Ln(-1)
	}

	// Footer with generation timestamp.
	pdf.Ln(10)
	pdf.SetFont("Helvetica", "I", 8)
	pdf.CellFormat(0, 5, fmt.Sprintf("Generated at %s", time.Now().UTC().Format(time.RFC3339)), "", 1, "L", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("generating PDF output: %w", err)
	}

	return buf.Bytes(), nil
}

func validateExportPeriod(period ExportPeriod) error {
	if period.From.IsZero() || period.To.IsZero() {
		return errors.New("usage export period requires from and to")
	}
	if period.To.Before(period.From) {
		return errors.New("usage export period to must be on or after from")
	}
	if period.To.Sub(period.From) > maxUsageExportPeriod {
		return fmt.Errorf("usage export period cannot exceed %d days", int(maxUsageExportPeriod.Hours()/24))
	}
	return nil
}

func getUsageRecordsForExport(ctx context.Context, store Store, orgID string, period ExportPeriod) ([]UsageRecord, error) {
	boundedStore, ok := store.(boundedOrgUsagePeriodStore)
	if !ok {
		return nil, ErrUsageExportRequiresBoundedDB
	}

	queryCtx, cancel := context.WithTimeout(ctx, maxUsageExportQueryTimeout)
	defer cancel()

	records, err := boundedStore.GetOrgUsageForPeriodLimited(queryCtx, orgID, period.From, period.To, maxUsageExportRows+1)
	if err != nil {
		return nil, err
	}
	if len(records) > maxUsageExportRows {
		return nil, fmt.Errorf("%w: max %d rows", ErrUsageExportTooLarge, maxUsageExportRows)
	}
	return records, nil
}

func escapeCSVFormulaCell(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '\t', '\r', '\n', '\x00':
		return "'" + value
	}
	for _, r := range value {
		if unicode.Is(unicode.Cf, r) || unicode.IsMark(r) ||
			unicode.IsSpace(r) || unicode.IsControl(r) {
			continue
		}
		switch r {
		case '=', '+', '-', '@':
			return "'" + value
		}
		return value
	}
	return value
}

func microToUSDString(micro int64) string {
	return fmt.Sprintf("%.6f", float64(micro)/1_000_000)
}
