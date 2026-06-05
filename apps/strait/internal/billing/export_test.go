package billing

import (
	"context"
	"encoding/csv"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t,
		err)

	reader := csv.NewReader(strings.NewReader(string(data)))
	records, err := reader.ReadAll()
	require.NoError(t,
		err)
	require.Len(t, records,
		1)
	assert.Equal(t, "date",
		records[0][0])

	// Should have just the header row.
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
	require.NoError(t,
		err)

	reader := csv.NewReader(strings.NewReader(string(data)))
	records, err := reader.ReadAll()
	require.NoError(t,
		err)
	require.Len(t, records,
		3)

	// Header + 2 data rows.

	expectedHeader := []string{"date", "project", "runs", "orchestration_cost_usd", "total_usd"}
	for i, col := range expectedHeader {
		assert.Equal(t, col,
			records[0][i])
	}
	assert.Equal(t, "2026-01-15",
		records[1][0],
	)
	assert.Equal(t, "proj-a",
		records[1][1])
	assert.Equal(t, "42",
		records[1][2])
	assert.Equal(t, "5.000000",
		records[1][3])
	assert.Equal(t, "5.000000",
		records[1][4])
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
	require.NoError(t,
		err)

	reader := csv.NewReader(strings.NewReader(string(data)))
	records, err := reader.ReadAll()
	require.NoError(t,
		err)
	require.Len(t, records,
		2)
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
	require.NoError(t,
		err)
	require.True(t, strings.HasPrefix(string(data), "%PDF-"))
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
	require.NoError(t,
		err)

	reader := csv.NewReader(strings.NewReader(string(data)))
	records, err := reader.ReadAll()
	require.NoError(t,
		err)

	if got := records[1][1]; !strings.HasPrefix(got, "'=") {
		require.Failf(t, "test failure",

			"project cell = %q, want formula escaped with apostrophe", got)
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
			require.NoError(t,
				err)

			reader := csv.NewReader(strings.NewReader(string(data)))
			records, err := reader.ReadAll()
			require.NoError(t,
				err)

			if got := records[1][1]; !strings.HasPrefix(got, "'") {
				require.Failf(t, "test failure",

					"project cell = %q, want formula escaped with apostrophe", got)
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
	require.NoError(t,
		err)

	reader := csv.NewReader(strings.NewReader(string(data)))
	records, err := reader.ReadAll()
	require.NoError(t,
		err)
	require.Equal(t,
		projectID, records[1][1])
}

func TestDeepSecExportCSV_RejectsOversizedPeriod(t *testing.T) {
	store := &mockExportStore{}
	period := ExportPeriod{
		From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	if _, err := ExportCSV(context.Background(), store, "org-1", period); err == nil {
		require.Fail(t,

			"expected oversized export period error")
	}
}

func TestDeepSecExportPDF_RejectsOversizedPeriod(t *testing.T) {
	store := &mockExportStore{}
	period := ExportPeriod{
		From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	if _, err := ExportPDF(context.Background(), store, "org-1", period); err == nil {
		require.Fail(t,

			"expected oversized export period error")
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
	require.ErrorIs(t, err, ErrUsageExportTooLarge)
	require.Equal(t,
		maxUsageExportRows+
			1, store.
			limitedQueryLimit,
	)
}

func TestDeepSecExportPDF_RejectsRowOverflowWithBoundedQuery(t *testing.T) {
	t.Parallel()

	store := &mockExportStore{usageRecords: makeUsageExportRecords(maxUsageExportRows + 1)}
	period := ExportPeriod{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	_, err := ExportPDF(context.Background(), store, "org-1", period)
	require.ErrorIs(t, err, ErrUsageExportTooLarge)
	require.Equal(t,
		maxUsageExportRows+
			1, store.
			limitedQueryLimit,
	)
}

func TestExportPDF_Empty(t *testing.T) {
	store := &mockExportStore{}
	period := ExportPeriod{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	data, err := ExportPDF(context.Background(), store, "org-1", period)
	require.NoError(t,
		err)
	assert.True(t, strings.HasPrefix(
		string(data), "%PDF-",
	))
	assert.GreaterOrEqual(t, len(data), 100)
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
	require.NoError(t,
		err)
	assert.True(t, strings.HasPrefix(
		string(data), "%PDF-",
	))

	// PDF with records should be larger than an empty one.
	emptyStore := &mockExportStore{}
	emptyData, err := ExportPDF(context.Background(), emptyStore, "org-1", period)
	require.NoError(t,
		err)
	assert.Greater(t, len(data), len(
		emptyData),
	)
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
	require.NoError(t,
		err)
	assert.True(t, strings.HasPrefix(
		string(data), "%PDF-",
	))
}

func TestMicroToUSDString_NegativeValue(t *testing.T) {
	got := microToUSDString(-1500000)
	assert.Equal(t, "-1.500000",
		got)
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
	require.NoError(t,
		err)

	reader := csv.NewReader(strings.NewReader(string(data)))
	records, err := reader.ReadAll()
	require.NoError(t,
		err)
	require.Len(t, records,
		2)
	assert.Equal(t, "10.000000",
		records[1][4])
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
	require.NoError(t,
		err)
	assert.True(t, strings.HasPrefix(
		string(data), "%PDF-",
	))
	assert.GreaterOrEqual(t, len(data), 1000)

	// Large dataset should produce a substantial PDF
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
		assert.Equal(t, tt.
			expected, got)
	}
}
