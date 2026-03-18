package billing

import (
	"context"
	"fmt"
	"time"
)

// AnomalySeverity indicates the severity of a spending anomaly.
type AnomalySeverity string

const (
	AnomalySeverityWarning  AnomalySeverity = "warning"
	AnomalySeverityHigh     AnomalySeverity = "high"
	AnomalySeverityCritical AnomalySeverity = "critical"

	// anomalyBaselineDays is the minimum number of days of history required
	// before anomaly detection activates.
	anomalyBaselineDays = 7

	// Spike ratio thresholds.
	spikeWarning  = 3.0
	spikeHigh     = 5.0
	spikeCritical = 10.0
)

// AnomalyAlert describes a detected spending anomaly for an organization.
type AnomalyAlert struct {
	OrgID          string          `json:"org_id"`
	TodaySpend     int64           `json:"today_spend"`
	Avg7dSpend     int64           `json:"avg_7d_spend"`
	SpikeRatio     float64         `json:"spike_ratio"`
	TopContributor string          `json:"top_contributor"`
	Severity       AnomalySeverity `json:"severity"`
}

// AnomalyDetector checks for spending spikes across organizations.
type AnomalyDetector struct {
	store Store
}

// NewAnomalyDetector creates a new anomaly detector.
func NewAnomalyDetector(store Store) *AnomalyDetector {
	return &AnomalyDetector{store: store}
}

// DetectAnomalies checks all provided org IDs for spending spikes. It compares
// today's spend against the rolling 7-day average and returns alerts for orgs
// whose spending is 3x or more above their average.
func (d *AnomalyDetector) DetectAnomalies(ctx context.Context, orgIDs []string) ([]AnomalyAlert, error) {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	var alerts []AnomalyAlert

	for _, orgID := range orgIDs {
		alert, found, err := d.detectForOrg(ctx, orgID, today)
		if err != nil {
			return nil, fmt.Errorf("detecting anomalies for org %s: %w", orgID, err)
		}
		if found {
			alerts = append(alerts, alert)
		}
	}

	return alerts, nil
}

func (d *AnomalyDetector) detectForOrg(ctx context.Context, orgID string, today time.Time) (AnomalyAlert, bool, error) {
	var zero AnomalyAlert
	// Fetch the last 8 days of usage: 7 historical days + today.
	windowStart := today.AddDate(0, 0, -anomalyBaselineDays)
	records, err := d.store.GetOrgUsageForPeriod(ctx, orgID, windowStart, today)
	if err != nil {
		return zero, false, fmt.Errorf("getting usage for anomaly detection: %w", err)
	}

	// Split records into today vs historical and find top contributor.
	var todaySpend int64
	var historicalSpend int64
	historicalDays := make(map[string]bool)
	topContributor := ""
	var topContributorSpend int64

	for _, r := range records {
		spend := r.ComputeCostMicro + r.AICostMicro
		dateStr := r.PeriodDate.Format("2006-01-02")
		todayStr := today.Format("2006-01-02")

		if dateStr == todayStr {
			todaySpend += spend
			if spend > topContributorSpend {
				topContributorSpend = spend
				topContributor = r.ProjectID
			}
		} else {
			historicalSpend += spend
			historicalDays[dateStr] = true
		}
	}

	// Need at least 7 days of historical data to establish a baseline.
	if len(historicalDays) < anomalyBaselineDays {
		return zero, false, nil
	}

	avg7d := historicalSpend / int64(len(historicalDays))
	if avg7d == 0 {
		return zero, false, nil
	}

	spikeRatio := float64(todaySpend) / float64(avg7d)
	if spikeRatio < spikeWarning {
		return zero, false, nil
	}

	severity := classifySeverity(spikeRatio)

	return AnomalyAlert{
		OrgID:          orgID,
		TodaySpend:     todaySpend,
		Avg7dSpend:     avg7d,
		SpikeRatio:     spikeRatio,
		TopContributor: topContributor,
		Severity:       severity,
	}, true, nil
}

func classifySeverity(ratio float64) AnomalySeverity {
	switch {
	case ratio >= spikeCritical:
		return AnomalySeverityCritical
	case ratio >= spikeHigh:
		return AnomalySeverityHigh
	default:
		return AnomalySeverityWarning
	}
}
