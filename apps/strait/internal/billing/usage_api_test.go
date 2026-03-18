package billing

import (
	"context"
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestUsageService_GetCurrentUsage(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	svc := NewUsageService(store, enforcer)

	resp, err := svc.GetCurrentUsage(context.Background(), "org_test", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Plan != "free" {
		t.Errorf("plan = %q, want free", resp.Plan)
	}
	if resp.Usage.RunsToday.Limit != 5000 {
		t.Errorf("runs limit = %d, want 5000", resp.Usage.RunsToday.Limit)
	}
	if resp.Usage.Projects.Used != 1 {
		t.Errorf("projects used = %d, want 1", resp.Usage.Projects.Used)
	}
	if resp.Usage.Members.Used != 2 {
		t.Errorf("members used = %d, want 2", resp.Usage.Members.Used)
	}
	if resp.Usage.RetentionDays != 1 {
		t.Errorf("retention = %d, want 1", resp.Usage.RetentionDays)
	}
}

func TestUsageService_AlertsAt80Percent(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	svc := NewUsageService(store, enforcer)

	// Simulate 4100 runs (82% of 5000 free limit)
	ctx := context.Background()
	for range 4100 {
		_ = enforcer.CheckDailyRunLimit(ctx, "org_alert")
	}

	resp, err := svc.GetCurrentUsage(ctx, "org_alert", 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Alerts) == 0 {
		t.Fatal("expected alerts at 82% usage")
	}
	if resp.Alerts[0].Dimension != "runs_today" {
		t.Errorf("alert dimension = %q, want runs_today", resp.Alerts[0].Dimension)
	}
}

func TestRecommendPlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		runs    int64
		compute int64
		want    string
	}{
		{"low_usage", 1000, 0, "free"},
		{"moderate", 200000, 10000000, "starter"},
		{"high", 1000000, 30000000, "pro"},
		{"very_high", 5000000, 60000000, "enterprise"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := recommendPlan(tt.runs, tt.compute)
			if got != tt.want {
				t.Errorf("recommendPlan(%d, %d) = %q, want %q", tt.runs, tt.compute, got, tt.want)
			}
		})
	}
}

func TestSafePercent(t *testing.T) {
	t.Parallel()

	if got := safePercent(50, 100); got != 50.0 {
		t.Errorf("safePercent(50, 100) = %f, want 50.0", got)
	}
	if got := safePercent(0, 0); got != 0.0 {
		t.Errorf("safePercent(0, 0) = %f, want 0.0", got)
	}
	if got := safePercent(100, -1); got != 0.0 {
		t.Errorf("safePercent(100, -1) = %f, want 0.0", got)
	}
}
