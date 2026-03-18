package billing

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

type mockExportStore struct {
	mockBillingStore
	usageRecords []UsageRecord
}

func (m *mockExportStore) GetOrgUsageForPeriod(_ context.Context, _ string, _, _ time.Time) ([]UsageRecord, error) {
	return m.usageRecords, nil
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
				AITokensTotal:    1000,
				AICostMicro:      2000000,
			},
			{
				ProjectID:        "proj-b",
				PeriodDate:       time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC),
				RunsCount:        10,
				ComputeCostMicro: 1000000,
				AITokensTotal:    500,
				AICostMicro:      500000,
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

	// Verify header columns.
	expectedHeader := []string{"date", "project", "runs", "compute_cost_usd", "ai_tokens", "ai_cost_usd", "total_usd"}
	for i, col := range expectedHeader {
		if records[0][i] != col {
			t.Errorf("header[%d]: expected %s, got %s", i, col, records[0][i])
		}
	}

	// Verify first data row.
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
		t.Errorf("expected compute_cost_usd 5.000000, got %s", records[1][3])
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
