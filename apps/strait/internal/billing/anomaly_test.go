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
			UsageCostMicro:   0,
		})
	}
	// Today: 5000 (5x the average).
	records = append(records, UsageRecord{
		OrgID:            "org-1",
		ProjectID:        "proj-a",
		PeriodDate:       today,
		ComputeCostMicro: 5000,
		UsageCostMicro:   0,
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

func TestAnomalyDetector_DetectAnomalies_IgnoresUsageCost(t *testing.T) {
	today := time.Now().UTC()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	var records []UsageRecord
	for i := 1; i <= 7; i++ {
		records = append(records, UsageRecord{
			OrgID:          "org-usage",
			ProjectID:      "proj-a",
			PeriodDate:     today.AddDate(0, 0, -i),
			UsageCostMicro: 1_000,
		})
	}
	records = append(records, UsageRecord{
		OrgID:          "org-usage",
		ProjectID:      "proj-a",
		PeriodDate:     today,
		UsageCostMicro: 6_000,
	})

	store := &mockAnomalyStore{usageRecords: records}
	detector := NewAnomalyDetector(store)

	alerts, err := detector.DetectAnomalies(context.Background(), []string{"org-usage"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 0 {
		t.Fatalf("expected no alerts from usage cost, got %d", len(alerts))
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

func TestAnomalyDetector_CustomThresholds_Warning(t *testing.T) {
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
	// Today: 2500 (2.5x the average, above custom warning of 2.0).
	records = append(records, UsageRecord{
		OrgID:            "org-1",
		ProjectID:        "proj-a",
		PeriodDate:       today,
		ComputeCostMicro: 2500,
	})

	store := &mockAnomalyStore{usageRecords: records}
	detector := NewAnomalyDetectorWithConfig(store, AnomalyConfig{
		WarningThreshold:  2.0,
		CriticalThreshold: 10.0,
	})

	alerts, err := detector.DetectAnomalies(context.Background(), []string{"org-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Severity != AnomalySeverityWarning {
		t.Errorf("expected severity warning, got %s", alerts[0].Severity)
	}
	if alerts[0].SpikeRatio != 2.5 {
		t.Errorf("expected spike ratio 2.5, got %f", alerts[0].SpikeRatio)
	}
}

func TestAnomalyDetector_CustomThresholds_Critical(t *testing.T) {
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
	// Today: 7000 (7x the average, at or above custom critical of 7.0).
	records = append(records, UsageRecord{
		OrgID:            "org-1",
		ProjectID:        "proj-a",
		PeriodDate:       today,
		ComputeCostMicro: 7000,
	})

	store := &mockAnomalyStore{usageRecords: records}
	detector := NewAnomalyDetectorWithConfig(store, AnomalyConfig{
		WarningThreshold:  2.0,
		CriticalThreshold: 7.0,
	})

	alerts, err := detector.DetectAnomalies(context.Background(), []string{"org-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Severity != AnomalySeverityCritical {
		t.Errorf("expected severity critical, got %s", alerts[0].Severity)
	}
	if alerts[0].SpikeRatio != 7.0 {
		t.Errorf("expected spike ratio 7.0, got %f", alerts[0].SpikeRatio)
	}
}

func TestAnomalyDetector_CustomThresholds_BelowWarning(t *testing.T) {
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
	// Today: 1500 (1.5x the average, below custom warning of 2.0).
	records = append(records, UsageRecord{
		OrgID:            "org-1",
		ProjectID:        "proj-a",
		PeriodDate:       today,
		ComputeCostMicro: 1500,
	})

	store := &mockAnomalyStore{usageRecords: records}
	detector := NewAnomalyDetectorWithConfig(store, AnomalyConfig{
		WarningThreshold:  2.0,
		CriticalThreshold: 10.0,
	})

	alerts, err := detector.DetectAnomalies(context.Background(), []string{"org-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 0 {
		t.Fatalf("expected no alerts for 1.5x spike with 2.0 warning threshold, got %d", len(alerts))
	}
}

func TestAnomalyDetector_CustomThresholds_HighAutoComputed(t *testing.T) {
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
	// Today: 6000 (6x the average).
	// With warning=2.0 and critical=8.0, high threshold = (2.0+8.0)/2 = 5.0.
	// 6x >= 5.0 and < 8.0, so severity should be "high".
	records = append(records, UsageRecord{
		OrgID:            "org-1",
		ProjectID:        "proj-a",
		PeriodDate:       today,
		ComputeCostMicro: 6000,
	})

	store := &mockAnomalyStore{usageRecords: records}
	detector := NewAnomalyDetectorWithConfig(store, AnomalyConfig{
		WarningThreshold:  2.0,
		CriticalThreshold: 8.0,
	})

	alerts, err := detector.DetectAnomalies(context.Background(), []string{"org-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Severity != AnomalySeverityHigh {
		t.Errorf("expected severity high for 6x spike (high threshold 5.0), got %s", alerts[0].Severity)
	}
	if alerts[0].SpikeRatio != 6.0 {
		t.Errorf("expected spike ratio 6.0, got %f", alerts[0].SpikeRatio)
	}
}

func TestAnomalyDetector_DefaultConfig_BackwardsCompatible(t *testing.T) {
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
	// Today: 5000 (5x the average).
	records = append(records, UsageRecord{
		OrgID:            "org-1",
		ProjectID:        "proj-a",
		PeriodDate:       today,
		ComputeCostMicro: 5000,
	})

	store := &mockAnomalyStore{usageRecords: records}

	// Using default config (3.0/10.0) via NewAnomalyDetectorWithConfig should
	// produce the same results as NewAnomalyDetector.
	detectorDefault := NewAnomalyDetector(store)
	detectorWithConfig := NewAnomalyDetectorWithConfig(store, DefaultAnomalyConfig())

	alertsDefault, err := detectorDefault.DetectAnomalies(context.Background(), []string{"org-1"})
	if err != nil {
		t.Fatalf("unexpected error from default detector: %v", err)
	}
	alertsWithConfig, err := detectorWithConfig.DetectAnomalies(context.Background(), []string{"org-1"})
	if err != nil {
		t.Fatalf("unexpected error from config detector: %v", err)
	}

	if len(alertsDefault) != len(alertsWithConfig) {
		t.Fatalf("alert count mismatch: default=%d, withConfig=%d", len(alertsDefault), len(alertsWithConfig))
	}
	if len(alertsDefault) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alertsDefault))
	}
	if alertsDefault[0].Severity != alertsWithConfig[0].Severity {
		t.Errorf("severity mismatch: default=%s, withConfig=%s", alertsDefault[0].Severity, alertsWithConfig[0].Severity)
	}
	if alertsDefault[0].SpikeRatio != alertsWithConfig[0].SpikeRatio {
		t.Errorf("spike ratio mismatch: default=%f, withConfig=%f", alertsDefault[0].SpikeRatio, alertsWithConfig[0].SpikeRatio)
	}
}

func TestAnomalyDetector_MixedComputeAndUsageSpendUsesComputeOnly(t *testing.T) {
	today := time.Now().UTC()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	var records []UsageRecord
	for i := 1; i <= 7; i++ {
		records = append(records, UsageRecord{
			OrgID:            "org-mixed",
			ProjectID:        "proj-a",
			PeriodDate:       today.AddDate(0, 0, -i),
			ComputeCostMicro: 500,
			UsageCostMicro:   500,
		})
	}
	// Launch anomaly detection ignores usage cost: 3000 compute / 500 baseline = 6x.
	records = append(records, UsageRecord{
		OrgID:            "org-mixed",
		ProjectID:        "proj-a",
		PeriodDate:       today,
		ComputeCostMicro: 3000,
		UsageCostMicro:   2000,
	})

	store := &mockAnomalyStore{usageRecords: records}
	detector := NewAnomalyDetector(store)

	alerts, err := detector.DetectAnomalies(context.Background(), []string{"org-mixed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].SpikeRatio != 6.0 {
		t.Errorf("expected spike ratio 6.0, got %f", alerts[0].SpikeRatio)
	}
	if alerts[0].TodaySpend != 3000 {
		t.Errorf("expected today spend 3000, got %d", alerts[0].TodaySpend)
	}
	if alerts[0].Avg7dSpend != 500 {
		t.Errorf("expected avg7d 500, got %d", alerts[0].Avg7dSpend)
	}
}

func TestAnomalyDetector_ExactWarningThreshold_Triggers(t *testing.T) {
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
	// Today: exactly 3000 = 3.0x avg (exactly at default warning threshold).
	records = append(records, UsageRecord{
		OrgID:            "org-1",
		ProjectID:        "proj-a",
		PeriodDate:       today,
		ComputeCostMicro: 3000,
	})

	store := &mockAnomalyStore{usageRecords: records}
	detector := NewAnomalyDetector(store)

	alerts, err := detector.DetectAnomalies(context.Background(), []string{"org-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// spikeRatio == 3.0, threshold is 3.0. Condition is `<`, so 3.0 is NOT less
	// than 3.0 — it should trigger.
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert at exact threshold, got %d", len(alerts))
	}
	if alerts[0].Severity != AnomalySeverityWarning {
		t.Errorf("expected warning severity, got %s", alerts[0].Severity)
	}
}

func TestAnomalyDetector_TopContributor_MultipleRecords(t *testing.T) {
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
	// Two records today: proj-a with 2000, proj-b with 5000. proj-b is top.
	records = append(records,
		UsageRecord{OrgID: "org-1", ProjectID: "proj-a", PeriodDate: today, ComputeCostMicro: 2000},
		UsageRecord{OrgID: "org-1", ProjectID: "proj-b", PeriodDate: today, ComputeCostMicro: 5000},
	)

	store := &mockAnomalyStore{usageRecords: records}
	detector := NewAnomalyDetector(store)

	alerts, err := detector.DetectAnomalies(context.Background(), []string{"org-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].TopContributor != "proj-b" {
		t.Errorf("expected top contributor proj-b, got %s", alerts[0].TopContributor)
	}
	if alerts[0].TodaySpend != 7000 {
		t.Errorf("expected today spend 7000, got %d", alerts[0].TodaySpend)
	}
}

func TestAnomalyDetector_ZeroThresholds_NoAlerts(t *testing.T) {
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
	// Today: 2000 (2x the average).
	// Zero thresholds normalize to defaults (3.0/10.0), so 2x should not trigger.
	records = append(records, UsageRecord{
		OrgID:            "org-1",
		ProjectID:        "proj-a",
		PeriodDate:       today,
		ComputeCostMicro: 2000,
	})

	store := &mockAnomalyStore{usageRecords: records}
	detector := NewAnomalyDetectorWithConfig(store, AnomalyConfig{
		WarningThreshold:  0,
		CriticalThreshold: 0,
	})

	alerts, err := detector.DetectAnomalies(context.Background(), []string{"org-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 0 {
		t.Fatalf("expected no alerts for 2x spike with zero thresholds (defaults to 3.0), got %d", len(alerts))
	}
}
