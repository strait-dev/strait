package billing

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type mockExportStore struct {
	mockBillingStore
	usageRecords      []UsageRecord
	limitedQueryLimit int
}

func (m *mockExportStore) GetOrgUsageForPeriod(_ context.Context, _ string, _, _ time.Time) ([]UsageRecord, error) {
	return m.usageRecords, nil
}

func (m *mockExportStore) GetOrgUsageForPeriodLimited(_ context.Context, _ string, _, _ time.Time, limit int) ([]UsageRecord, error) {
	m.limitedQueryLimit = limit
	if len(m.usageRecords) > limit {
		return m.usageRecords[:limit], nil
	}
	return m.usageRecords, nil
}

func makeUsageExportRecords(count int) []UsageRecord {
	records := make([]UsageRecord, count)
	for i := range records {
		records[i] = UsageRecord{
			ProjectID:        fmt.Sprintf("proj-%05d", i),
			PeriodDate:       time.Date(2026, 1, 1+(i%31), 0, 0, 0, 0, time.UTC),
			RunsCount:        1,
			ComputeCostMicro: 1000,
		}
	}
	return records
}

func TestExportCSV_Empty(t *testing.T) {
	store := &mockExportStore{}
	period := ExportPeriod{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	data, err := ExportCSV(context.Background(), store, "org-1", period)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reader := csv.NewReader(strings.NewReader(string(data)))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}
	// Should have just the header row.
	if len(records) != 1 {
		t.Fatalf("expected 1 row (header), got %d", len(records))
	}
	if records[0][0] != "date" {
		t.Errorf("expected first header column to be 'date', got %s", records[0][0])
	}
}

func TestExportCSV_WithRecords(t *testing.T) {
	store := &mockExportStore{
		usageRecords: []UsageRecord{
			{
				ProjectID:        "proj-a",
				PeriodDate:       time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
				RunsCount:        42,
				ComputeCostMicro: 5000000,
			},
			{
				ProjectID:        "proj-b",
				PeriodDate:       time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC),
				RunsCount:        10,
				ComputeCostMicro: 1000000,
			},
		},
	}
	period := ExportPeriod{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	data, err := ExportCSV(context.Background(), store, "org-1", period)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reader := csv.NewReader(strings.NewReader(string(data)))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	// Header + 2 data rows.
	if len(records) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(records))
	}

	expectedHeader := []string{"date", "project", "runs", "orchestration_cost_usd", "total_usd"}
	for i, col := range expectedHeader {
		if records[0][i] != col {
			t.Errorf("header[%d]: expected %s, got %s", i, col, records[0][i])
		}
	}

	if records[1][0] != "2026-01-15" {
		t.Errorf("expected date 2026-01-15, got %s", records[1][0])
	}
	if records[1][1] != "proj-a" {
		t.Errorf("expected project proj-a, got %s", records[1][1])
	}
	if records[1][2] != "42" {
		t.Errorf("expected runs 42, got %s", records[1][2])
	}
	if records[1][3] != "5.000000" {
		t.Errorf("expected orchestration_cost_usd 5.000000, got %s", records[1][3])
	}
	if records[1][4] != "5.000000" {
		t.Errorf("expected total_usd 5.000000, got %s", records[1][4])
	}
}

func TestExportCSV_SingleDayPeriodAllowed(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	store := &mockExportStore{
		usageRecords: []UsageRecord{{
			ProjectID:        "proj-one-day",
			PeriodDate:       day,
			RunsCount:        1,
			ComputeCostMicro: 1_000_000,
		}},
	}

	data, err := ExportCSV(context.Background(), store, "org-1", ExportPeriod{From: day, To: day})
	if err != nil {
		t.Fatalf("ExportCSV single-day period: %v", err)
	}
	reader := csv.NewReader(strings.NewReader(string(data)))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("rows = %d, want header + one record", len(records))
	}
}

func TestExportPDF_SingleDayPeriodAllowed(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	store := &mockExportStore{
		usageRecords: []UsageRecord{{
			ProjectID:        "proj-one-day",
			PeriodDate:       day,
			RunsCount:        1,
			ComputeCostMicro: 1_000_000,
		}},
	}

	data, err := ExportPDF(context.Background(), store, "org-1", ExportPeriod{From: day, To: day})
	if err != nil {
		t.Fatalf("ExportPDF single-day period: %v", err)
	}
	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Fatalf("expected PDF output, got %q", string(data[:5]))
	}
}

func TestDeepSecExportCSV_EscapesFormulaProjectID(t *testing.T) {
	store := &mockExportStore{
		usageRecords: []UsageRecord{
			{
				ProjectID:        "=HYPERLINK(\"https://attacker.test\")",
				PeriodDate:       time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
				RunsCount:        1,
				ComputeCostMicro: 1000,
			},
		},
	}
	period := ExportPeriod{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	data, err := ExportCSV(context.Background(), store, "org-1", period)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reader := csv.NewReader(strings.NewReader(string(data)))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}
	if got := records[1][1]; !strings.HasPrefix(got, "'=") {
		t.Fatalf("project cell = %q, want formula escaped with apostrophe", got)
	}
}

func TestDeepSecExportCSV_EscapesHiddenFormulaProjectID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		projectID string
	}{
		{"bom_equals", "\uFEFF=HYPERLINK(\"https://attacker.test\")"},
		{"zwsp_plus", "\u200b+SUM(1,1)"},
		{"lrm_minus", "\u200e-1+1"},
		{"combining_mark_at", "\u0301@cmd"},
		{"space_then_equals", " =cmd"},
		{"tab_then_equals", "\t=cmd"},
		{"nul_then_equals", "\x00=cmd"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockExportStore{
				usageRecords: []UsageRecord{{
					ProjectID:        tc.projectID,
					PeriodDate:       time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
					RunsCount:        1,
					ComputeCostMicro: 1000,
				}},
			}
			period := ExportPeriod{
				From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
			}

			data, err := ExportCSV(context.Background(), store, "org-1", period)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			reader := csv.NewReader(strings.NewReader(string(data)))
			records, err := reader.ReadAll()
			if err != nil {
				t.Fatalf("failed to parse CSV: %v", err)
			}
			if got := records[1][1]; !strings.HasPrefix(got, "'") {
				t.Fatalf("project cell = %q, want formula escaped with apostrophe", got)
			}
		})
	}
}

func TestDeepSecExportCSV_PreservesBenignInvisibleProjectID(t *testing.T) {
	t.Parallel()

	projectID := "\uFEFFproject-benign"
	store := &mockExportStore{
		usageRecords: []UsageRecord{{
			ProjectID:        projectID,
			PeriodDate:       time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			RunsCount:        1,
			ComputeCostMicro: 1000,
		}},
	}
	period := ExportPeriod{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	data, err := ExportCSV(context.Background(), store, "org-1", period)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reader := csv.NewReader(strings.NewReader(string(data)))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}
	if got := records[1][1]; got != projectID {
		t.Fatalf("project cell = %q, want unchanged %q", got, projectID)
	}
}

func TestDeepSecExportCSV_RejectsOversizedPeriod(t *testing.T) {
	store := &mockExportStore{}
	period := ExportPeriod{
		From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	if _, err := ExportCSV(context.Background(), store, "org-1", period); err == nil {
		t.Fatal("expected oversized export period error")
	}
}

func TestDeepSecExportPDF_RejectsOversizedPeriod(t *testing.T) {
	store := &mockExportStore{}
	period := ExportPeriod{
		From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	if _, err := ExportPDF(context.Background(), store, "org-1", period); err == nil {
		t.Fatal("expected oversized export period error")
	}
}

func TestDeepSecExportCSV_RejectsRowOverflowWithBoundedQuery(t *testing.T) {
	t.Parallel()

	store := &mockExportStore{usageRecords: makeUsageExportRecords(maxUsageExportRows + 1)}
	period := ExportPeriod{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	_, err := ExportCSV(context.Background(), store, "org-1", period)
	if !errors.Is(err, ErrUsageExportTooLarge) {
		t.Fatalf("ExportCSV error = %v, want ErrUsageExportTooLarge", err)
	}
	if store.limitedQueryLimit != maxUsageExportRows+1 {
		t.Fatalf("limited query limit = %d, want %d", store.limitedQueryLimit, maxUsageExportRows+1)
	}
}

func TestDeepSecExportPDF_RejectsRowOverflowWithBoundedQuery(t *testing.T) {
	t.Parallel()

	store := &mockExportStore{usageRecords: makeUsageExportRecords(maxUsageExportRows + 1)}
	period := ExportPeriod{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	_, err := ExportPDF(context.Background(), store, "org-1", period)
	if !errors.Is(err, ErrUsageExportTooLarge) {
		t.Fatalf("ExportPDF error = %v, want ErrUsageExportTooLarge", err)
	}
	if store.limitedQueryLimit != maxUsageExportRows+1 {
		t.Fatalf("limited query limit = %d, want %d", store.limitedQueryLimit, maxUsageExportRows+1)
	}
}

func TestExportPDF_Empty(t *testing.T) {
	store := &mockExportStore{}
	period := ExportPeriod{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	data, err := ExportPDF(context.Background(), store, "org-1", period)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Errorf("expected PDF output to start with %%PDF-, got %q", string(data[:20]))
	}
	if len(data) < 100 {
		t.Errorf("expected non-trivial PDF output, got %d bytes", len(data))
	}
}

func TestExportPDF_WithRecords(t *testing.T) {
	store := &mockExportStore{
		usageRecords: []UsageRecord{
			{
				ProjectID:        "proj-a",
				PeriodDate:       time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
				RunsCount:        42,
				ComputeCostMicro: 5000000,
			},
			{
				ProjectID:        "proj-b",
				PeriodDate:       time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC),
				RunsCount:        10,
				ComputeCostMicro: 1000000,
			},
		},
	}
	period := ExportPeriod{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	data, err := ExportPDF(context.Background(), store, "org-1", period)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Errorf("expected PDF output to start with %%PDF-, got %q", string(data[:20]))
	}

	// PDF with records should be larger than an empty one.
	emptyStore := &mockExportStore{}
	emptyData, err := ExportPDF(context.Background(), emptyStore, "org-1", period)
	if err != nil {
		t.Fatalf("unexpected error generating empty PDF: %v", err)
	}
	if len(data) <= len(emptyData) {
		t.Errorf("expected PDF with records (%d bytes) to be larger than empty PDF (%d bytes)", len(data), len(emptyData))
	}
}

func TestExportPDF_NoSubscription(t *testing.T) {
	// mockExportStore embeds mockBillingStore which returns ErrSubscriptionNotFound
	// by default, so the PDF should still generate with "free" as the plan tier.
	store := &mockExportStore{
		usageRecords: []UsageRecord{
			{
				ProjectID:        "proj-a",
				PeriodDate:       time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
				RunsCount:        5,
				ComputeCostMicro: 100000,
			},
		},
	}
	period := ExportPeriod{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	data, err := ExportPDF(context.Background(), store, "org-no-sub", period)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Errorf("expected PDF output to start with %%PDF-, got %q", string(data[:20]))
	}
}

func TestMicroToUSDString_NegativeValue(t *testing.T) {
	got := microToUSDString(-1500000)
	if got != "-1.500000" {
		t.Errorf("microToUSDString(-1500000) = %s, want -1.500000", got)
	}
}

func TestExportCSV_VerifyAllColumns(t *testing.T) {
	store := &mockExportStore{
		usageRecords: []UsageRecord{
			{
				ProjectID:        "proj-test",
				PeriodDate:       time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
				RunsCount:        100,
				ComputeCostMicro: 10000000,
			},
		},
	}
	period := ExportPeriod{
		From: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
	}

	data, err := ExportCSV(context.Background(), store, "org-1", period)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reader := csv.NewReader(strings.NewReader(string(data)))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 rows (header + 1 data), got %d", len(records))
	}

	if records[1][4] != "10.000000" {
		t.Errorf("expected total_usd 10.000000, got %s", records[1][4])
	}
}

func TestExportPDF_LargeDataSet(t *testing.T) {
	// Test with many records to ensure multi-page PDF works
	records := make([]UsageRecord, 0, 100)
	for i := range 100 {
		records = append(records, UsageRecord{
			ProjectID:        fmt.Sprintf("proj-%d", i%5),
			PeriodDate:       time.Date(2026, 1, 1+(i%28), 0, 0, 0, 0, time.UTC),
			RunsCount:        int64(i * 10),
			ComputeCostMicro: int64(i * 100000),
		})
	}
	store := &mockExportStore{usageRecords: records}
	period := ExportPeriod{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	data, err := ExportPDF(context.Background(), store, "org-1", period)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Errorf("expected PDF header")
	}
	// Large dataset should produce a substantial PDF
	if len(data) < 1000 {
		t.Errorf("expected large PDF, got only %d bytes", len(data))
	}
}

func TestMicroToUSDString(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0.000000"},
		{1000000, "1.000000"},
		{5000000, "5.000000"},
		{1500, "0.001500"},
	}

	for _, tt := range tests {
		got := microToUSDString(tt.input)
		if got != tt.expected {
			t.Errorf("microToUSDString(%d) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}
