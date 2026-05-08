//go:build !cloud

package billing

import "context"

func recordBillingLimitRejection(context.Context, string, string) {}

func recordBillingFailOpen(context.Context, string, string) {}

func recordBillingStripeUsageIngested(context.Context, string) {}

func recordBillingStripeUsageDropped(context.Context, string) {}

func recordBillingOverageEntered(context.Context, string) {}

func recordBillingQuotaUsage(context.Context, string, string, float64) {}

func recordBillingQuotaBlock(context.Context, string, string) {}

func recordBillingOverageRun(context.Context, string, string) {}

func recordBillingWebhookProcessed(context.Context, string, string) {}

func RecordHTTPModeRunCompleted(context.Context) {}

func RecordHTTPModeGateRejected(context.Context, string, string) {}

func RecordFeatureGateRejected(context.Context, string, string) {}

func recordBillingUsageRecord(context.Context, string, string) {}

func recordBillingUsageRecordCost(context.Context, string, int64) {}

func recordBillingIdempotencyDuplicate(context.Context, string) {}
