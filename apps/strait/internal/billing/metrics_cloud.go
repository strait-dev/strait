//go:build cloud

package billing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var billingMetrics = newBillingMetrics()

type billingRuntimeMetrics struct {
	limitRejections       metric.Int64Counter
	enforcementFailOpen   metric.Int64Counter
	stripeIngested        metric.Int64Counter
	stripeDropped         metric.Int64Counter
	overageEntered        metric.Int64Counter
	quotaUsage            metric.Float64Gauge
	quotaBlock            metric.Int64Counter
	overageRuns           metric.Int64Counter
	webhookProcessed      metric.Int64Counter
	httpModeCompleted     metric.Int64Counter
	httpModeGateRejected  metric.Int64Counter
	featureGateRejected   metric.Int64Counter
	usageRecords          metric.Int64Counter
	usageRecordCostMicros metric.Int64Histogram
	idempotencyDuplicates metric.Int64Counter
}

func newBillingMetrics() billingRuntimeMetrics {
	meter := otel.Meter("strait/billing")
	limitRejections, _ := meter.Int64Counter(
		"strait_billing_limit_rejections_total",
		metric.WithDescription("Total billing limit rejections by reason and plan tier"),
		metric.WithUnit("1"),
	)
	enforcementFailOpen, _ := meter.Int64Counter(
		"strait_billing_enforcement_fail_open_total",
		metric.WithDescription("Total enforcement checks that failed open due to infrastructure errors"),
		metric.WithUnit("1"),
	)
	stripeIngested, _ := meter.Int64Counter(
		"strait_billing_stripe_usage_events_ingested_total",
		metric.WithDescription("Total Stripe usage events ingested"),
		metric.WithUnit("1"),
	)
	stripeDropped, _ := meter.Int64Counter(
		"strait_billing_stripe_usage_events_dropped_total",
		metric.WithDescription("Total Stripe usage events dropped"),
		metric.WithUnit("1"),
	)
	overageEntered, _ := meter.Int64Counter(
		"strait_billing_overage_entered_total",
		metric.WithDescription("Total times a paid plan entered daily run overage"),
		metric.WithUnit("1"),
	)
	quotaUsage, _ := meter.Float64Gauge(
		"strait_billing_quota_usage",
		metric.WithDescription("Billing quota usage ratio by resource and plan tier"),
		metric.WithUnit("1"),
	)
	quotaBlock, _ := meter.Int64Counter(
		"strait_billing_quota_block_total",
		metric.WithDescription("Total billing quota blocks by reason and plan tier"),
		metric.WithUnit("1"),
	)
	overageRuns, _ := meter.Int64Counter(
		"strait_billing_overage_runs_total",
		metric.WithDescription("Total paid-plan run quota overages by resource and plan tier"),
		metric.WithUnit("1"),
	)
	webhookProcessed, _ := meter.Int64Counter(
		"strait_billing_webhook_processed_total",
		metric.WithDescription("Total Stripe billing webhooks processed by event type and outcome"),
		metric.WithUnit("1"),
	)
	httpModeCompleted, _ := meter.Int64Counter(
		"strait_billing_http_mode_runs_completed_total",
		metric.WithDescription("Total HTTP mode runs with cost recorded"),
		metric.WithUnit("1"),
	)
	httpModeGateRejected, _ := meter.Int64Counter(
		"strait_billing_http_mode_gate_rejected_total",
		metric.WithDescription("Total HTTP mode rejections by plan gating"),
		metric.WithUnit("1"),
	)
	featureGateRejected, _ := meter.Int64Counter(
		"strait_billing_feature_gate_rejected_total",
		metric.WithDescription("Total feature gate rejections by feature and plan tier"),
		metric.WithUnit("1"),
	)
	usageRecords, _ := meter.Int64Counter(
		"strait_billing_usage_records_total",
		metric.WithDescription("Total billing usage-record writes by execution mode and outcome"),
		metric.WithUnit("1"),
	)
	usageRecordCostMicros, _ := meter.Int64Histogram(
		"strait_billing_usage_record_cost_microusd",
		metric.WithDescription("Billing usage-record cost in micro-USD"),
		metric.WithUnit("1"),
		metric.WithExplicitBucketBoundaries(1, 10, 20, 50, 100, 500, 1000, 10000, 100000, 1000000),
	)
	idempotencyDuplicates, _ := meter.Int64Counter(
		"strait_billing_idempotency_duplicates_total",
		metric.WithDescription("Total billing idempotency duplicate suppressions"),
		metric.WithUnit("1"),
	)
	return billingRuntimeMetrics{
		limitRejections:       limitRejections,
		enforcementFailOpen:   enforcementFailOpen,
		stripeIngested:        stripeIngested,
		stripeDropped:         stripeDropped,
		overageEntered:        overageEntered,
		quotaUsage:            quotaUsage,
		quotaBlock:            quotaBlock,
		overageRuns:           overageRuns,
		webhookProcessed:      webhookProcessed,
		httpModeCompleted:     httpModeCompleted,
		httpModeGateRejected:  httpModeGateRejected,
		featureGateRejected:   featureGateRejected,
		usageRecords:          usageRecords,
		usageRecordCostMicros: usageRecordCostMicros,
		idempotencyDuplicates: idempotencyDuplicates,
	}
}

func recordBillingLimitRejection(ctx context.Context, reason, planTier string) {
	billingMetrics.limitRejections.Add(ctx, 1, metric.WithAttributes(
		attribute.String("reason", reason),
		attribute.String("plan_tier", planTier),
	))
	recordBillingQuotaBlock(ctx, reason, planTier)
}

func recordBillingFailOpen(ctx context.Context, checkType, errorType string) {
	billingMetrics.enforcementFailOpen.Add(ctx, 1, metric.WithAttributes(
		attribute.String("check_type", checkType),
		attribute.String("error_type", errorType),
	))
}

func recordBillingStripeUsageIngested(ctx context.Context, status string) {
	billingMetrics.stripeIngested.Add(ctx, 1, metric.WithAttributes(attribute.String("status", status)))
}

func recordBillingStripeUsageDropped(ctx context.Context, status string) {
	billingMetrics.stripeDropped.Add(ctx, 1, metric.WithAttributes(attribute.String("status", status)))
}

func recordBillingOverageEntered(ctx context.Context, planTier string) {
	billingMetrics.overageEntered.Add(ctx, 1, metric.WithAttributes(attribute.String("plan_tier", planTier)))
}

func recordBillingQuotaUsage(ctx context.Context, resource, planTier string, usageRatio float64) {
	if usageRatio < 0 {
		usageRatio = 0
	}
	billingMetrics.quotaUsage.Record(ctx, usageRatio, metric.WithAttributes(
		attribute.String("resource", normalizeBillingResource(resource)),
		attribute.String("plan_tier", planTier),
	))
}

func recordBillingQuotaBlock(ctx context.Context, reason, planTier string) {
	billingMetrics.quotaBlock.Add(ctx, 1, metric.WithAttributes(
		attribute.String("reason", normalizeBillingResource(reason)),
		attribute.String("plan_tier", planTier),
	))
}

func recordBillingOverageRun(ctx context.Context, resource, planTier string) {
	billingMetrics.overageRuns.Add(ctx, 1, metric.WithAttributes(
		attribute.String("resource", normalizeBillingResource(resource)),
		attribute.String("plan_tier", planTier),
	))
}

func recordBillingWebhookProcessed(ctx context.Context, eventType, outcome string) {
	billingMetrics.webhookProcessed.Add(ctx, 1, metric.WithAttributes(
		attribute.String("event_type", normalizeBillingWebhookEventType(eventType)),
		attribute.String("outcome", normalizeBillingWebhookOutcome(outcome)),
	))
}

func RecordHTTPModeRunCompleted(ctx context.Context) {
	billingMetrics.httpModeCompleted.Add(ctx, 1)
}

func RecordHTTPModeGateRejected(ctx context.Context, planTier, source string) {
	billingMetrics.httpModeGateRejected.Add(ctx, 1, metric.WithAttributes(
		attribute.String("plan_tier", planTier),
		attribute.String("source", source),
	))
}

func RecordFeatureGateRejected(ctx context.Context, feature, planTier string) {
	billingMetrics.featureGateRejected.Add(ctx, 1, metric.WithAttributes(
		attribute.String("feature", feature),
		attribute.String("plan_tier", planTier),
	))
}

func recordBillingUsageRecord(ctx context.Context, mode, outcome string) {
	billingMetrics.usageRecords.Add(ctx, 1, metric.WithAttributes(
		attribute.String("mode", normalizeBillingMode(mode)),
		attribute.String("outcome", outcome),
	))
}

func recordBillingUsageRecordCost(ctx context.Context, mode string, costMicroUSD int64) {
	billingMetrics.usageRecordCostMicros.Record(ctx, costMicroUSD, metric.WithAttributes(
		attribute.String("mode", normalizeBillingMode(mode)),
	))
}

func recordBillingIdempotencyDuplicate(ctx context.Context, mode string) {
	billingMetrics.idempotencyDuplicates.Add(ctx, 1, metric.WithAttributes(
		attribute.String("mode", normalizeBillingMode(mode)),
	))
}

func normalizeBillingMode(mode string) string {
	switch mode {
	case "http", "worker", "webhook_delivery":
		return mode
	default:
		return "unknown"
	}
}

func normalizeBillingResource(resource string) string {
	switch resource {
	case "daily_runs", "monthly_runs", "concurrent_runs", "projects", "members", "organizations", "spend",
		"daily_run_limit", "monthly_run_limit", "concurrent_limit", "project_limit", "member_limit",
		"org_creation_limit", "spending_limit", "dispatch_priority", "project_suspension":
		return resource
	default:
		return "unknown"
	}
}

func normalizeBillingWebhookEventType(eventType string) string {
	if eventType == "" {
		return "unknown"
	}
	return eventType
}

func normalizeBillingWebhookOutcome(outcome string) string {
	switch outcome {
	case "success", "failure", "duplicate", "ignored":
		return outcome
	default:
		return "failure"
	}
}
