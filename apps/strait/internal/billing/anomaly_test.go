package billing

import (
	"context"
	"testing"
	"time"
)

type mockAnomalyStore struct {
	mockBillingStore
	usageRecords []UsageRecord
}

func (m *mockAnomalyStore) GetOrgUsageForPeriod(_ context.Context, _ string, _, _ time.Time) ([]UsageRecord, error) {
	return m.usageRecords, nil
}

func TestAnomalyDetector_DetectAnomalies_NoHistory(t *testing.T) {
	store := &mockAnomalyStore{}
	detector := NewAnomalyDetector(store)

	alerts, err := detector.DetectAnomalies(context.Background(), []string{"org-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 0 {
		t.Fatalf("expected no alerts with no history, got %d", len(alerts))
	}
}

func TestAnomalyDetector_DetectAnomalies_Spike(t *testing.T) {
	today := time.Now().UTC()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	var records []UsageRecord
	// Create 7 days of history with spend of 1000 per day.
	for i := 1; i <= 7; i++ {
		records = append(records, UsageRecord{
			OrgID:            "org-1",
			ProjectID:        "proj-a",
			PeriodDate:       today.AddDate(0, 0, -i),
			ComputeCostMicro: 1000,
			AICostMicro:      0,
		})
	}
	// Today: 5000 (5x the average).
	records = append(records, UsageRecord{
		OrgID:            "org-1",
		ProjectID:        "proj-a",
		PeriodDate:       today,
		ComputeCostMicro: 5000,
		AICostMicro:      0,
	})

	store := &mockAnomalyStore{usageRecords: records}
	detector := NewAnomalyDetector(store)

	alerts, err := detector.DetectAnomalies(context.Background(), []string{"org-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.OrgID != "org-1" {
		t.Errorf("expected org_id org-1, got %s", alert.OrgID)
	}
	if alert.Severity != AnomalySeverityHigh {
		t.Errorf("expected severity high for 5x spike, got %s", alert.Severity)
	}
	if alert.SpikeRatio != 5.0 {
		t.Errorf("expected spike ratio 5.0, got %f", alert.SpikeRatio)
	}
}

func TestAnomalyDetector_DetectAnomalies_NoSpike(t *testing.T) {
	today := time.Now().UTC()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	var records []UsageRecord
	for i := 1; i <= 7; i++ {
		records = append(records, UsageRecord{
			OrgID:            "org-1",
			ProjectID:        "proj-a",
			PeriodDate:       today.AddDate(0, 0, -i),
			ComputeCostMicro: 1000,
		})
	}
	// Today: 2000 (2x, below threshold).
	records = append(records, UsageRecord{
		OrgID:            "org-1",
		ProjectID:        "proj-a",
		PeriodDate:       today,
		ComputeCostMicro: 2000,
	})

	store := &mockAnomalyStore{usageRecords: records}
	detector := NewAnomalyDetector(store)

	alerts, err := detector.DetectAnomalies(context.Background(), []string{"org-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 0 {
		t.Fatalf("expected no alerts for 2x spend, got %d", len(alerts))
	}
}

func TestAnomalyDetector_DetectAnomalies_AISpendSpike(t *testing.T) {
	today := time.Now().UTC()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	var records []UsageRecord
	for i := 1; i <= 7; i++ {
		records = append(records, UsageRecord{
			OrgID:       "org-ai",
			ProjectID:   "proj-a",
			PeriodDate:  today.AddDate(0, 0, -i),
			AICostMicro: 1_000,
		})
	}
	records = append(records, UsageRecord{
		OrgID:       "org-ai",
		ProjectID:   "proj-a",
		PeriodDate:  today,
		AICostMicro: 6_000,
	})

	store := &mockAnomalyStore{usageRecords: records}
	detector := NewAnomalyDetector(store)

	alerts, err := detector.DetectAnomalies(context.Background(), []string{"org-ai"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].OrgID != "org-ai" {
		t.Fatalf("org_id = %q, want org-ai", alerts[0].OrgID)
	}
}

func TestClassifySeverity(t *testing.T) {
	tests := []struct {
		ratio    float64
		expected AnomalySeverity
	}{
		{3.0, AnomalySeverityWarning},
		{4.9, AnomalySeverityWarning},
		{5.0, AnomalySeverityHigh},
		{9.9, AnomalySeverityHigh},
		{10.0, AnomalySeverityCritical},
		{15.0, AnomalySeverityCritical},
	}

	for _, tt := range tests {
		got := classifySeverity(tt.ratio)
		if got != tt.expected {
			t.Errorf("classifySeverity(%f) = %s, want %s", tt.ratio, got, tt.expected)
		}
	}
}
