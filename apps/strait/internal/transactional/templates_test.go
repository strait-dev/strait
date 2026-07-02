package transactional

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func propsMap(t *testing.T, props any) map[string]any {
	t.Helper()
	payload, err := json.Marshal(props)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(payload, &out))
	return out
}

func TestTemplateBuilders(t *testing.T) {
	t.Parallel()

	to := []string{"admin@example.com"}
	from := "billing@strait.dev"
	graceEnd := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	periodStart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name              string
		req               Request
		wantTemplate      TemplateID
		wantProps         map[string]any
		wantIDKey         string
		wantAttachment    bool
		wantAttachmentB64 string
	}{
		{
			name:         "billing spending limit warning",
			req:          BillingSpendingLimitWarningRequest(to, from, "Pro", "$80.00", "$100.00", "80%"),
			wantTemplate: TemplateBillingSpendingLimitWarning,
			wantIDKey:    "billing:spending_limit_warning:admin@example.com:Pro:80%",
			wantProps: map[string]any{
				"currentSpend":  "$80.00",
				"name":          "",
				"percentUsed":   "80%",
				"planName":      "Pro",
				"spendingLimit": "$100.00",
			},
		},
		{
			name:         "billing overage alert",
			req:          BillingOverageAlertRequest(to, from, "Pro", "$4.50", "$10.00"),
			wantTemplate: TemplateBillingOverageAlert,
			wantIDKey:    "billing:overage_alert:admin@example.com:Pro:$4.50",
			wantProps: map[string]any{
				"includedAllowance": "$10.00",
				"name":              "",
				"overageAmount":     "$4.50",
				"planName":          "Pro",
			},
		},
		{
			name:         "billing payment failed",
			req:          BillingPaymentFailedRequest(to, from, "Pro", graceEnd),
			wantTemplate: TemplateBillingPaymentFailed,
			wantIDKey:    "billing:payment_failed:admin@example.com:Pro:2026-04-15",
			wantProps: map[string]any{
				"gracePeriodEnd": "April 15, 2026",
				"name":           "",
				"planName":       "Pro",
			},
		},
		{
			name:         "billing plan changed",
			req:          BillingPlanChangedRequest(to, from, "Pro", "Scale", "April 15, 2026"),
			wantTemplate: TemplateBillingPlanChanged,
			wantIDKey:    "billing:plan_changed:admin@example.com:Pro:Scale",
			wantProps: map[string]any{
				"effectiveDate": "April 15, 2026",
				"name":          "",
				"newPlan":       "Scale",
				"previousPlan":  "Pro",
			},
		},
		{
			name:         "billing enterprise reminder",
			req:          BillingEnterpriseContractReminderRequest(to, from, "December 31, 2026", true, 30),
			wantTemplate: TemplateBillingEnterpriseContractReminder,
			wantIDKey:    "billing:enterprise_contract_reminder:admin@example.com:December 31, 2026:true:30",
			wantProps: map[string]any{
				"autoRenew":       true,
				"contractEndDate": "December 31, 2026",
				"daysRemaining":   30,
			},
		},
		{
			name:         "billing downgrade http jobs warning",
			req:          BillingDowngradeHTTPJobsWarningRequest(to, from, "April 30, 2026", 3),
			wantTemplate: TemplateBillingDowngradeHTTPJobsWarning,
			wantIDKey:    "billing:downgrade_http_jobs_warning:admin@example.com:April 30, 2026:3",
			wantProps: map[string]any{
				"jobCount":  3,
				"periodEnd": "April 30, 2026",
			},
		},
		{
			name:         "billing contract expired",
			req:          BillingContractExpiredRequest(to, from, "December 31, 2026"),
			wantTemplate: TemplateBillingContractExpired,
			wantIDKey:    "billing:contract_expired:admin@example.com:December 31, 2026",
			wantProps: map[string]any{
				"contractEndDate": "December 31, 2026",
			},
		},
		{
			name:         "billing trial ending soon",
			req:          BillingTrialEndingSoonRequest(to, from, "April 15, 2026", 3),
			wantTemplate: TemplateBillingTrialEndingSoon,
			wantIDKey:    "billing:trial_ending_soon:admin@example.com:April 15, 2026:3",
			wantProps: map[string]any{
				"daysRemaining": 3,
				"trialEndDate":  "April 15, 2026",
			},
		},
		{
			name:         "billing dispute alert",
			req:          BillingDisputeAlertRequest(to, from, "$25.00"),
			wantTemplate: TemplateBillingDisputeAlert,
			wantIDKey:    "billing:dispute_alert:admin@example.com:$25.00",
			wantProps: map[string]any{
				"disputeAmount": "$25.00",
			},
		},
		{
			name:         "billing invoice upcoming",
			req:          BillingInvoiceUpcomingRequest(to, from, "$125.00", "May 1, 2026"),
			wantTemplate: TemplateBillingInvoiceUpcoming,
			wantIDKey:    "billing:invoice_upcoming:admin@example.com:$125.00:May 1, 2026",
			wantProps: map[string]any{
				"amountDue": "$125.00",
				"dueDate":   "May 1, 2026",
			},
		},
		{
			name:         "billing dunning step",
			req:          BillingDunningStepRequest(to, from, "Pro", 2),
			wantTemplate: TemplateBillingDunningStep,
			wantIDKey:    "billing:dunning_step:admin@example.com:Pro:2",
			wantProps: map[string]any{
				"planName": "Pro",
				"step":     2,
			},
		},
		{
			name:         "billing paid plan welcome",
			req:          BillingPaidPlanWelcomeRequest(to, from, "org-1", "pro", "admin@example.com", "Pro", "1000000"),
			wantTemplate: TemplateBillingPaidPlanWelcome,
			wantIDKey:    "billing:welcome:org-1:pro:admin@example.com",
			wantProps: map[string]any{
				"monthlyRunAllowance": "1000000",
				"name":                "",
				"planName":            "Pro",
			},
		},
		{
			name:         "billing enterprise welcome",
			req:          BillingEnterpriseWelcomeRequest(to, from, "org-1", "enterprise", "admin@example.com"),
			wantTemplate: TemplateBillingEnterpriseWelcome,
			wantIDKey:    "billing:welcome:org-1:enterprise:admin@example.com",
			wantProps:    map[string]any{},
		},
		{
			name:              "billing usage report",
			req:               BillingUsageReportRequest(to, from, "org-1", "pro", periodStart, periodEnd, 1, "$1.00", []byte("pdf")),
			wantTemplate:      TemplateBillingUsageReport,
			wantIDKey:         "billing:usage_report:org-1:2026-04-30",
			wantAttachment:    true,
			wantAttachmentB64: base64.StdEncoding.EncodeToString([]byte("pdf")),
			wantProps: map[string]any{
				"addonCount":    1,
				"orgId":         "org-1",
				"overageAmount": "$1.00",
				"periodEnd":     "Apr 30, 2026",
				"periodStart":   "Apr 1",
				"planTier":      "pro",
			},
		},
		{
			name:         "notification spending limit warning",
			req:          NotificationSpendingLimitWarningRequest(to, "alerts@strait.dev", "delivery-1", "spending.limit_warning", "org-1", "80%", "$100.00", "$80.00"),
			wantTemplate: TemplateNotificationSpendingLimitWarning,
			wantIDKey:    "notification:delivery-1:spending.limit_warning",
			wantProps: map[string]any{
				"currentSpend":   "$80.00",
				"orgId":          "org-1",
				"overagePercent": "80%",
				"spendingLimit":  "$100.00",
			},
		},
		{
			name:         "notification spending limit reached",
			req:          NotificationSpendingLimitReachedRequest(to, "alerts@strait.dev", "delivery-1", "spending.limit_reached", "org-1", "$100.00", "$100.00"),
			wantTemplate: TemplateNotificationSpendingLimitReached,
			wantIDKey:    "notification:delivery-1:spending.limit_reached",
			wantProps: map[string]any{
				"currentSpend":  "$100.00",
				"orgId":         "org-1",
				"spendingLimit": "$100.00",
			},
		},
		{
			name:         "notification cost anomaly",
			req:          NotificationCostAnomalyRequest(to, "alerts@strait.dev", "delivery-1", "cost.anomaly", "org-1", "high", "3.5x", "50000 micro-USD", "15000 micro-USD", "job-x"),
			wantTemplate: TemplateNotificationCostAnomaly,
			wantIDKey:    "notification:delivery-1:cost.anomaly",
			wantProps: map[string]any{
				"orgId":           "org-1",
				"sevenDayAverage": "15000 micro-USD",
				"severity":        "high",
				"spikeRatio":      "3.5x",
				"todaySpend":      "50000 micro-USD",
				"topContributor":  "job-x",
			},
		},
		{
			name:         "notification budget threshold",
			req:          NotificationBudgetThresholdRequest(to, "alerts@strait.dev", "delivery-1", "budget.threshold_reached", "project-1", "80%", "85000 micro-USD", "1000000 micro-USD"),
			wantTemplate: TemplateNotificationBudgetThreshold,
			wantIDKey:    "notification:delivery-1:budget.threshold_reached",
			wantProps: map[string]any{
				"budgetLimit":      "1000000 micro-USD",
				"dailyCost":        "85000 micro-USD",
				"projectId":        "project-1",
				"thresholdPercent": "80%",
			},
		},
		{
			name:         "notification usage forecast warning",
			req:          NotificationUsageForecastWarningRequest(to, "alerts@strait.dev", "delivery-1", "usage.forecast_warning", "org-1", 2, "scale", 1_200_000),
			wantTemplate: TemplateNotificationUsageForecastWarning,
			wantIDKey:    "notification:delivery-1:usage.forecast_warning",
			wantProps: map[string]any{
				"daysUntilLimit":  2,
				"orgId":           "org-1",
				"projectedRuns":   1_200_000,
				"recommendedPlan": "scale",
			},
		},
		{
			name:         "notification generic",
			req:          NotificationGenericRequest(to, "alerts@strait.dev", "delivery-1", "unknown.event", `{"field":"value"}`),
			wantTemplate: TemplateNotificationGeneric,
			wantIDKey:    "notification:delivery-1:unknown.event",
			wantProps: map[string]any{
				"eventType": "unknown.event",
				"payload":   `{"field":"value"}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.wantTemplate, tt.req.Template)
			assert.Equal(t, tt.wantIDKey, tt.req.IdempotencyKey)
			require.Equal(t, to, tt.req.To)
			assert.NotEmpty(t, tt.req.From)

			gotProps := propsMap(t, tt.req.Props)
			for key, want := range tt.wantProps {
				assert.EqualValues(t, want, gotProps[key])
			}
			assert.Len(t, gotProps, len(tt.wantProps))

			if tt.wantAttachment {
				require.Len(t, tt.req.Attachments, 1)
				assert.Equal(t, "strait-usage-2026-04-01-to-2026-04-30.pdf", tt.req.Attachments[0].Filename)
				assert.Equal(t, tt.wantAttachmentB64, tt.req.Attachments[0].ContentBase64)
				assert.Equal(t, "application/pdf", tt.req.Attachments[0].ContentType)
			} else {
				assert.Empty(t, tt.req.Attachments)
			}
		})
	}
}
