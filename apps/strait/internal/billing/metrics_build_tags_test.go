package billing

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBillingMetricsBuildTags(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)

	assertBuildTag(t, filepath.Join(dir, "metrics_cloud.go"), "//go:build cloud")
	assertBuildTag(t, filepath.Join(dir, "metrics_community.go"), "//go:build !cloud")
}

func TestBillingMetricsHelpersNoPanic(t *testing.T) {
	ctx := context.Background()
	recordBillingLimitRejection(ctx, "daily_run_limit", "free")
	recordBillingFailOpen(ctx, "daily_run", "redis_error")
	recordBillingStripeUsageIngested(ctx, "ok")
	recordBillingStripeUsageDropped(ctx, "error")
	recordBillingOverageEntered(ctx, "pro")
	recordBillingQuotaUsage(ctx, "daily_runs", "pro", 0.8)
	recordBillingQuotaBlock(ctx, "daily_run_limit", "free")
	recordBillingOverageRun(ctx, "monthly_runs", "pro")
	recordBillingWebhookProcessed(ctx, "invoice.paid", "success")
	RecordHTTPModeRunCompleted(ctx)
	RecordHTTPModeGateRejected(ctx, "free", "job_create")
	RecordFeatureGateRejected(ctx, "approval_gates", "free")
	recordBillingUsageRecord(ctx, "http", "success")
	recordBillingUsageRecordCost(ctx, "http", 20)
	recordBillingIdempotencyDuplicate(ctx, "http")
}

func TestBillingCloudMetricNames(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	content, err := os.ReadFile(filepath.Join(dir, "metrics_cloud.go"))
	if err != nil {
		t.Fatalf("read metrics_cloud.go: %v", err)
	}

	for _, name := range []string{
		"strait_billing_quota_usage",
		"strait_billing_quota_block_total",
		"strait_billing_overage_runs_total",
		"strait_billing_webhook_processed_total",
	} {
		if !strings.Contains(string(content), name) {
			t.Fatalf("metrics_cloud.go missing %s", name)
		}
	}
}

func assertBuildTag(t *testing.T, path, tag string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.SplitN(string(content), "\n", 2)
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != tag {
		t.Fatalf("%s first line = %q, want %q", filepath.Base(path), lines[0], tag)
	}
}
