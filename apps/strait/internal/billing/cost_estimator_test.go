package billing

import (
	"testing"

	"strait/internal/compute"
)

func TestEstimateJobCost_ValidPreset(t *testing.T) {
	wantCost := compute.CostMicro * 60
	est, err := EstimateJobCost("micro", 60, 10000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.CostMicro != wantCost {
		t.Errorf("expected cost %d, got %d", wantCost, est.CostMicro)
	}
	if est.RateMicroPerSec != compute.CostMicro {
		t.Errorf("expected rate %d, got %d", compute.CostMicro, est.RateMicroPerSec)
	}
	wantRuns := int64(10000) / wantCost
	if est.CreditRunsRemaining != wantRuns {
		t.Errorf("expected %d runs remaining, got %d", wantRuns, est.CreditRunsRemaining)
	}
	if est.CheaperAlternative != nil {
		t.Errorf("expected no cheaper alternative for micro preset")
	}
}

func TestEstimateJobCost_WithCheaperAlternative(t *testing.T) {
	wantCost := compute.CostMedium1x * 60
	est, err := EstimateJobCost("medium-1x", 60, 100000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.CostMicro != wantCost {
		t.Errorf("expected cost %d, got %d", wantCost, est.CostMicro)
	}
	if est.CheaperAlternative == nil {
		t.Fatal("expected a cheaper alternative")
	}
	if est.CheaperAlternative.Preset != "micro" {
		t.Errorf("expected cheaper alternative to be micro, got %s", est.CheaperAlternative.Preset)
	}
	if est.CheaperAlternative.SavingsPct <= 0 {
		t.Errorf("expected positive savings percentage, got %f", est.CheaperAlternative.SavingsPct)
	}
}

func TestEstimateJobCost_UnknownPreset(t *testing.T) {
	_, err := EstimateJobCost("nonexistent", 60, 10000)
	if err == nil {
		t.Fatal("expected error for unknown preset")
	}
}

func TestEstimateJobCost_ZeroCredit(t *testing.T) {
	est, err := EstimateJobCost("micro", 60, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.CreditRunsRemaining != 0 {
		t.Errorf("expected 0 runs remaining with 0 credit, got %d", est.CreditRunsRemaining)
	}
}

func TestCronRunsPerDay_EveryMinute(t *testing.T) {
	t.Parallel()
	rpd, err := CronRunsPerDay("* * * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rpd != 1440 {
		t.Errorf("CronRunsPerDay(every minute) = %.0f, want 1440", rpd)
	}
}

func TestCronRunsPerDay_Hourly(t *testing.T) {
	t.Parallel()
	rpd, err := CronRunsPerDay("0 * * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rpd != 24 {
		t.Errorf("CronRunsPerDay(hourly) = %.0f, want 24", rpd)
	}
}

func TestCronRunsPerDay_Empty(t *testing.T) {
	t.Parallel()
	rpd, err := CronRunsPerDay("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rpd != 0 {
		t.Errorf("CronRunsPerDay(empty) = %.0f, want 0", rpd)
	}
}

func TestCronRunsPerDay_Invalid(t *testing.T) {
	t.Parallel()
	_, err := CronRunsPerDay("not-a-cron")
	if err == nil {
		t.Fatal("expected error for invalid cron expression")
	}
}

func TestEstimateWhatIf_ValidPresetAndCron(t *testing.T) {
	t.Parallel()
	est, err := EstimateWhatIf("micro", 60, "0 * * * *", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.RunsPerDay != 24 {
		t.Errorf("RunsPerDay = %.0f, want 24", est.RunsPerDay)
	}
	if est.DailyCostUsd <= 0 {
		t.Error("expected positive daily cost")
	}
	if est.MonthlyCostUsd <= 0 {
		t.Error("expected positive monthly cost")
	}
	if est.CostPerRunUsd <= 0 {
		t.Error("expected positive cost per run")
	}
}

func TestEstimateWhatIf_NoCron(t *testing.T) {
	t.Parallel()
	est, err := EstimateWhatIf("micro", 60, "", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.RunsPerDay != 3 {
		t.Errorf("RunsPerDay = %.0f, want 3", est.RunsPerDay)
	}
}

func TestEstimateWhatIf_ZeroCount(t *testing.T) {
	t.Parallel()
	est, err := EstimateWhatIf("micro", 60, "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.RunsPerDay != 1 {
		t.Errorf("RunsPerDay = %.0f, want 1 (zero normalized to 1)", est.RunsPerDay)
	}
}

func TestEstimateWhatIf_InvalidPreset(t *testing.T) {
	t.Parallel()
	_, err := EstimateWhatIf("nonexistent", 60, "", 1)
	if err == nil {
		t.Fatal("expected error for invalid preset")
	}
}

func TestEstimateWhatIf_InvalidCron(t *testing.T) {
	t.Parallel()
	_, err := EstimateWhatIf("micro", 60, "bad-cron", 1)
	if err == nil {
		t.Fatal("expected error for invalid cron")
	}
}

func TestFindCheaperAlternative_ZeroCost(t *testing.T) {
	t.Parallel()
	result := findCheaperAlternative("micro", 0, 0)
	if result != nil {
		t.Error("expected nil for zero cost")
	}
}

func TestFindCheaperAlternative_SmallestPreset(t *testing.T) {
	t.Parallel()
	result := findCheaperAlternative("micro", 60, 100)
	if result != nil {
		t.Error("expected nil for smallest preset (no cheaper alternative)")
	}
}

func TestFindCheaperAlternative_LargePreset(t *testing.T) {
	t.Parallel()
	largeCost := compute.CostMedium1x * 60
	result := findCheaperAlternative("medium-1x", 60, largeCost)
	if result == nil {
		t.Fatal("expected cheaper alternative for medium-1x")
	}
	if result.Preset != "micro" {
		t.Errorf("expected micro as cheapest alternative, got %s", result.Preset)
	}
	if result.SavingsPct <= 0 || result.SavingsPct >= 100 {
		t.Errorf("savings pct should be between 0 and 100, got %f", result.SavingsPct)
	}
	if result.CostMicro >= largeCost {
		t.Errorf("cheaper alternative cost %d should be less than %d", result.CostMicro, largeCost)
	}
}

func TestEstimateWhatIf_ArithmeticValues(t *testing.T) {
	t.Parallel()
	est, err := EstimateWhatIf("micro", 60, "", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedCostPerRun := float64(compute.CostMicro*60) / 1_000_000
	assertCostApprox(t, est.CostPerRunUsd, expectedCostPerRun)
	assertCostApprox(t, est.DailyCostUsd, expectedCostPerRun*1)
	assertCostApprox(t, est.MonthlyCostUsd, expectedCostPerRun*1*30)
}

func TestEstimateWhatIf_CronMultiplier(t *testing.T) {
	t.Parallel()
	est, err := EstimateWhatIf("micro", 60, "0 * * * *", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Hourly cron * 2 jobs = 48 runs/day
	if est.RunsPerDay != 48 {
		t.Errorf("RunsPerDay = %.0f, want 48", est.RunsPerDay)
	}
}

func TestEstimateJobCost_NegativeCredit(t *testing.T) {
	t.Parallel()
	est, err := EstimateJobCost("micro", 60, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.CreditRunsRemaining > 0 {
		t.Errorf("negative credit should yield non-positive runs remaining, got %d", est.CreditRunsRemaining)
	}
}

func assertCostApprox(t *testing.T, got, want float64) {
	t.Helper()
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > 0.0001 {
		t.Errorf("got %f, want %f", got, want)
	}
}
