package transactional

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// TemplateID identifies a template registered in packages/transactional.
type TemplateID string

const (
	TemplateBillingContractExpired            TemplateID = "billing.contract_expired"
	TemplateBillingDisputeAlert               TemplateID = "billing.dispute_alert"
	TemplateBillingDowngradeHTTPJobsWarning   TemplateID = "billing.downgrade_http_jobs_warning"
	TemplateBillingDunningStep                TemplateID = "billing.dunning_step"
	TemplateBillingEnterpriseContractReminder TemplateID = "billing.enterprise_contract_reminder"
	TemplateBillingEnterpriseWelcome          TemplateID = "billing.enterprise_welcome"
	TemplateBillingInvoiceUpcoming            TemplateID = "billing.invoice_upcoming"
	TemplateBillingOverageAlert               TemplateID = "billing.overage_alert"
	TemplateBillingPaidPlanWelcome            TemplateID = "billing.paid_plan_welcome"
	TemplateBillingPaymentFailed              TemplateID = "billing.payment_failed"
	TemplateBillingPlanChanged                TemplateID = "billing.plan_changed"
	TemplateBillingSpendingLimitWarning       TemplateID = "billing.spending_limit_warning"
	TemplateBillingTrialEndingSoon            TemplateID = "billing.trial_ending_soon"
	TemplateBillingUsageReport                TemplateID = "billing.usage_report"
	TemplateNotificationBudgetThreshold       TemplateID = "notification.budget_threshold"
	TemplateNotificationCostAnomaly           TemplateID = "notification.cost_anomaly"
	TemplateNotificationGeneric               TemplateID = "notification.generic"
	TemplateNotificationSpendingLimitReached  TemplateID = "notification.spending_limit_reached"
	TemplateNotificationSpendingLimitWarning  TemplateID = "notification.spending_limit_warning"
	TemplateNotificationUsageForecastWarning  TemplateID = "notification.usage_forecast_warning"
	usageReportAttachmentContentType                     = "application/pdf"
)

type billingContractExpiredProps struct {
	ContractEndDate string `json:"contractEndDate"`
}

type billingDisputeAlertProps struct {
	DisputeAmount string `json:"disputeAmount"`
}

type billingDowngradeHTTPJobsWarningProps struct {
	JobCount  int    `json:"jobCount"`
	PeriodEnd string `json:"periodEnd"`
}

type billingDunningStepProps struct {
	PlanName string `json:"planName"`
	Step     int    `json:"step"`
}

type billingEnterpriseContractReminderProps struct {
	AutoRenew       bool   `json:"autoRenew"`
	ContractEndDate string `json:"contractEndDate"`
	DaysRemaining   int    `json:"daysRemaining"`
}

type billingInvoiceUpcomingProps struct {
	AmountDue string `json:"amountDue"`
	DueDate   string `json:"dueDate"`
}

type billingOverageAlertProps struct {
	IncludedAllowance string `json:"includedAllowance"`
	Name              string `json:"name"`
	OverageAmount     string `json:"overageAmount"`
	PlanName          string `json:"planName"`
}

type billingPaidPlanWelcomeProps struct {
	MonthlyRunAllowance string `json:"monthlyRunAllowance"`
	Name                string `json:"name"`
	PlanName            string `json:"planName"`
}

type billingPaymentFailedProps struct {
	GracePeriodEnd string `json:"gracePeriodEnd"`
	Name           string `json:"name"`
	PlanName       string `json:"planName"`
}

type billingPlanChangedProps struct {
	EffectiveDate string `json:"effectiveDate"`
	Name          string `json:"name"`
	NewPlan       string `json:"newPlan"`
	PreviousPlan  string `json:"previousPlan"`
}

type billingSpendingLimitWarningProps struct {
	CurrentSpend  string `json:"currentSpend"`
	Name          string `json:"name"`
	PercentUsed   string `json:"percentUsed"`
	PlanName      string `json:"planName"`
	SpendingLimit string `json:"spendingLimit"`
}

type billingTrialEndingSoonProps struct {
	DaysRemaining int    `json:"daysRemaining"`
	TrialEndDate  string `json:"trialEndDate"`
}

type billingUsageReportProps struct {
	AddonCount    int    `json:"addonCount"`
	OrgID         string `json:"orgId"`
	OverageAmount string `json:"overageAmount,omitempty"`
	PeriodEnd     string `json:"periodEnd"`
	PeriodStart   string `json:"periodStart"`
	PlanTier      string `json:"planTier"`
}

type notificationBudgetThresholdProps struct {
	BudgetLimit      string `json:"budgetLimit"`
	DailyCost        string `json:"dailyCost"`
	ProjectID        string `json:"projectId"`
	ThresholdPercent string `json:"thresholdPercent"`
}

type notificationCostAnomalyProps struct {
	OrgID           string `json:"orgId"`
	SevenDayAverage string `json:"sevenDayAverage"`
	Severity        string `json:"severity"`
	SpikeRatio      string `json:"spikeRatio"`
	TodaySpend      string `json:"todaySpend"`
	TopContributor  string `json:"topContributor"`
}

type notificationGenericProps struct {
	EventType string `json:"eventType"`
	Payload   string `json:"payload"`
}

type notificationSpendingLimitReachedProps struct {
	CurrentSpend  string `json:"currentSpend"`
	OrgID         string `json:"orgId"`
	SpendingLimit string `json:"spendingLimit"`
}

type notificationSpendingLimitWarningProps struct {
	CurrentSpend   string `json:"currentSpend"`
	OrgID          string `json:"orgId"`
	OveragePercent string `json:"overagePercent"`
	SpendingLimit  string `json:"spendingLimit"`
}

type notificationUsageForecastWarningProps struct {
	DaysUntilLimit  int64  `json:"daysUntilLimit"`
	OrgID           string `json:"orgId"`
	ProjectedRuns   int64  `json:"projectedRuns"`
	RecommendedPlan string `json:"recommendedPlan"`
}

func BillingSpendingLimitWarningRequest(to []string, from, planName, currentSpend, limit, percent string) Request {
	return Request{
		Template:       TemplateBillingSpendingLimitWarning,
		To:             to,
		From:           from,
		IdempotencyKey: fmt.Sprintf("billing:spending_limit_warning:%s:%s:%s", recipientsKey(to), planName, percent),
		Props: billingSpendingLimitWarningProps{
			Name:          "",
			PlanName:      planName,
			CurrentSpend:  currentSpend,
			SpendingLimit: limit,
			PercentUsed:   percent,
		},
	}
}

func BillingOverageAlertRequest(to []string, from, planName, overageAmount, includedCredit string) Request {
	return Request{
		Template:       TemplateBillingOverageAlert,
		To:             to,
		From:           from,
		IdempotencyKey: fmt.Sprintf("billing:overage_alert:%s:%s:%s", recipientsKey(to), planName, overageAmount),
		Props: billingOverageAlertProps{
			Name:              "",
			PlanName:          planName,
			OverageAmount:     overageAmount,
			IncludedAllowance: includedCredit,
		},
	}
}

func BillingPaymentFailedRequest(to []string, from, planName string, gracePeriodEnd time.Time) Request {
	return Request{
		Template:       TemplateBillingPaymentFailed,
		To:             to,
		From:           from,
		IdempotencyKey: fmt.Sprintf("billing:payment_failed:%s:%s:%s", recipientsKey(to), planName, gracePeriodEnd.Format("2006-01-02")),
		Props: billingPaymentFailedProps{
			Name:           "",
			PlanName:       planName,
			GracePeriodEnd: gracePeriodEnd.Format("January 2, 2006"),
		},
	}
}

func BillingPlanChangedRequest(to []string, from, previousPlan, newPlan, effectiveDate string) Request {
	return Request{
		Template:       TemplateBillingPlanChanged,
		To:             to,
		From:           from,
		IdempotencyKey: fmt.Sprintf("billing:plan_changed:%s:%s:%s", recipientsKey(to), previousPlan, newPlan),
		Props: billingPlanChangedProps{
			Name:          "",
			PreviousPlan:  previousPlan,
			NewPlan:       newPlan,
			EffectiveDate: effectiveDate,
		},
	}
}

func BillingEnterpriseContractReminderRequest(to []string, from, contractEndDate string, autoRenew bool, daysRemaining int) Request {
	return Request{
		Template:       TemplateBillingEnterpriseContractReminder,
		To:             to,
		From:           from,
		IdempotencyKey: fmt.Sprintf("billing:enterprise_contract_reminder:%s:%s:%t:%d", recipientsKey(to), contractEndDate, autoRenew, daysRemaining),
		Props: billingEnterpriseContractReminderProps{
			ContractEndDate: contractEndDate,
			AutoRenew:       autoRenew,
			DaysRemaining:   daysRemaining,
		},
	}
}

func BillingDowngradeHTTPJobsWarningRequest(to []string, from, periodEnd string, jobCount int) Request {
	return Request{
		Template:       TemplateBillingDowngradeHTTPJobsWarning,
		To:             to,
		From:           from,
		IdempotencyKey: fmt.Sprintf("billing:downgrade_http_jobs_warning:%s:%s:%d", recipientsKey(to), periodEnd, jobCount),
		Props: billingDowngradeHTTPJobsWarningProps{
			PeriodEnd: periodEnd,
			JobCount:  jobCount,
		},
	}
}

func BillingContractExpiredRequest(to []string, from, contractEndDate string) Request {
	return Request{
		Template:       TemplateBillingContractExpired,
		To:             to,
		From:           from,
		IdempotencyKey: fmt.Sprintf("billing:contract_expired:%s:%s", recipientsKey(to), contractEndDate),
		Props: billingContractExpiredProps{
			ContractEndDate: contractEndDate,
		},
	}
}

func BillingTrialEndingSoonRequest(to []string, from, trialEndDate string, daysRemaining int) Request {
	return Request{
		Template:       TemplateBillingTrialEndingSoon,
		To:             to,
		From:           from,
		IdempotencyKey: fmt.Sprintf("billing:trial_ending_soon:%s:%s:%d", recipientsKey(to), trialEndDate, daysRemaining),
		Props: billingTrialEndingSoonProps{
			TrialEndDate:  trialEndDate,
			DaysRemaining: daysRemaining,
		},
	}
}

func BillingDisputeAlertRequest(to []string, from, disputeAmount string) Request {
	return Request{
		Template:       TemplateBillingDisputeAlert,
		To:             to,
		From:           from,
		IdempotencyKey: fmt.Sprintf("billing:dispute_alert:%s:%s", recipientsKey(to), disputeAmount),
		Props: billingDisputeAlertProps{
			DisputeAmount: disputeAmount,
		},
	}
}

func BillingInvoiceUpcomingRequest(to []string, from, amountDue, dueDate string) Request {
	return Request{
		Template:       TemplateBillingInvoiceUpcoming,
		To:             to,
		From:           from,
		IdempotencyKey: fmt.Sprintf("billing:invoice_upcoming:%s:%s:%s", recipientsKey(to), amountDue, dueDate),
		Props: billingInvoiceUpcomingProps{
			AmountDue: amountDue,
			DueDate:   dueDate,
		},
	}
}

func BillingDunningStepRequest(to []string, from, planName string, step int) Request {
	return Request{
		Template:       TemplateBillingDunningStep,
		To:             to,
		From:           from,
		IdempotencyKey: fmt.Sprintf("billing:dunning_step:%s:%s:%d", recipientsKey(to), planName, step),
		Props: billingDunningStepProps{
			PlanName: planName,
			Step:     step,
		},
	}
}

func BillingPaidPlanWelcomeRequest(to []string, from, orgID, tier, customerEmail, planName, monthlyRunAllowance string) Request {
	return Request{
		Template:       TemplateBillingPaidPlanWelcome,
		To:             to,
		From:           from,
		IdempotencyKey: fmt.Sprintf("billing:welcome:%s:%s:%s", orgID, tier, customerEmail),
		Props: billingPaidPlanWelcomeProps{
			Name:                "",
			PlanName:            planName,
			MonthlyRunAllowance: monthlyRunAllowance,
		},
	}
}

func BillingEnterpriseWelcomeRequest(to []string, from, orgID, tier, customerEmail string) Request {
	return Request{
		Template:       TemplateBillingEnterpriseWelcome,
		To:             to,
		From:           from,
		IdempotencyKey: fmt.Sprintf("billing:welcome:%s:%s:%s", orgID, tier, customerEmail),
		Props:          struct{}{},
	}
}

func BillingUsageReportRequest(to []string, from, orgID, planTier string, periodStart, periodEnd time.Time, addonCount int, overageAmount string, pdfBytes []byte) Request {
	return Request{
		Template:       TemplateBillingUsageReport,
		To:             to,
		From:           from,
		IdempotencyKey: fmt.Sprintf("billing:usage_report:%s:%s", orgID, periodEnd.Format("2006-01-02")),
		Props: billingUsageReportProps{
			OrgID:         orgID,
			PlanTier:      planTier,
			PeriodStart:   periodStart.Format("Jan 2"),
			PeriodEnd:     periodEnd.Format("Jan 2, 2006"),
			AddonCount:    addonCount,
			OverageAmount: overageAmount,
		},
		Attachments: []Attachment{
			{
				Filename: fmt.Sprintf("strait-usage-%s-to-%s.pdf",
					periodStart.Format("2006-01-02"),
					periodEnd.Format("2006-01-02")),
				ContentBase64: base64.StdEncoding.EncodeToString(pdfBytes),
				ContentType:   usageReportAttachmentContentType,
			},
		},
	}
}

func NotificationSpendingLimitWarningRequest(to []string, from, deliveryID, eventType, orgID, overagePercent, spendingLimit, currentSpend string) Request {
	return Request{
		Template:       TemplateNotificationSpendingLimitWarning,
		To:             to,
		From:           from,
		IdempotencyKey: notificationIdempotencyKey(deliveryID, eventType),
		Props: notificationSpendingLimitWarningProps{
			OrgID:          orgID,
			OveragePercent: overagePercent,
			SpendingLimit:  spendingLimit,
			CurrentSpend:   currentSpend,
		},
	}
}

func NotificationSpendingLimitReachedRequest(to []string, from, deliveryID, eventType, orgID, spendingLimit, currentSpend string) Request {
	return Request{
		Template:       TemplateNotificationSpendingLimitReached,
		To:             to,
		From:           from,
		IdempotencyKey: notificationIdempotencyKey(deliveryID, eventType),
		Props: notificationSpendingLimitReachedProps{
			OrgID:         orgID,
			SpendingLimit: spendingLimit,
			CurrentSpend:  currentSpend,
		},
	}
}

func NotificationCostAnomalyRequest(to []string, from, deliveryID, eventType, orgID, severity, spikeRatio, todaySpend, sevenDayAverage, topContributor string) Request {
	return Request{
		Template:       TemplateNotificationCostAnomaly,
		To:             to,
		From:           from,
		IdempotencyKey: notificationIdempotencyKey(deliveryID, eventType),
		Props: notificationCostAnomalyProps{
			OrgID:           orgID,
			Severity:        severity,
			SpikeRatio:      spikeRatio,
			TodaySpend:      todaySpend,
			SevenDayAverage: sevenDayAverage,
			TopContributor:  topContributor,
		},
	}
}

func NotificationBudgetThresholdRequest(to []string, from, deliveryID, eventType, projectID, thresholdPercent, dailyCost, budgetLimit string) Request {
	return Request{
		Template:       TemplateNotificationBudgetThreshold,
		To:             to,
		From:           from,
		IdempotencyKey: notificationIdempotencyKey(deliveryID, eventType),
		Props: notificationBudgetThresholdProps{
			ProjectID:        projectID,
			ThresholdPercent: thresholdPercent,
			DailyCost:        dailyCost,
			BudgetLimit:      budgetLimit,
		},
	}
}

func NotificationUsageForecastWarningRequest(to []string, from, deliveryID, eventType, orgID string, daysUntilLimit int64, recommendedPlan string, projectedRuns int64) Request {
	return Request{
		Template:       TemplateNotificationUsageForecastWarning,
		To:             to,
		From:           from,
		IdempotencyKey: notificationIdempotencyKey(deliveryID, eventType),
		Props: notificationUsageForecastWarningProps{
			OrgID:           orgID,
			DaysUntilLimit:  daysUntilLimit,
			RecommendedPlan: recommendedPlan,
			ProjectedRuns:   projectedRuns,
		},
	}
}

func NotificationGenericRequest(to []string, from, deliveryID, eventType, payload string) Request {
	return Request{
		Template:       TemplateNotificationGeneric,
		To:             to,
		From:           from,
		IdempotencyKey: notificationIdempotencyKey(deliveryID, eventType),
		Props: notificationGenericProps{
			EventType: eventType,
			Payload:   payload,
		},
	}
}

func notificationIdempotencyKey(deliveryID, eventType string) string {
	return fmt.Sprintf("notification:%s:%s", deliveryID, eventType)
}

func recipientsKey(to []string) string {
	return strings.Join(to, ",")
}
