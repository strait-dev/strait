package billing

import (
	"testing"
)

func TestEstimateJobCost_ValidPreset(t *testing.T) {
	// micro preset: 17 micro-USD/sec, 60 sec timeout = 1020 micro-USD.
	est, err := EstimateJobCost("micro", 60, 10000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.CostMicro != 1020 {
		t.Errorf("expected cost 1020, got %d", est.CostMicro)
	}
	if est.RateMicroPerSec != 17 {
		t.Errorf("expected rate 17, got %d", est.RateMicroPerSec)
	}
	if est.CreditRunsRemaining != 9 {
		t.Errorf("expected 9 runs remaining (10000/1020=9), got %d", est.CreditRunsRemaining)
	}
	// micro is the smallest preset, so no cheaper alternative.
	if est.CheaperAlternative != nil {
		t.Errorf("expected no cheaper alternative for micro preset")
	}
}

func TestEstimateJobCost_WithCheaperAlternative(t *testing.T) {
	// medium-1x preset: 85 micro-USD/sec.
	est, err := EstimateJobCost("medium-1x", 60, 100000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.CostMicro != 5100 {
		t.Errorf("expected cost 5100, got %d", est.CostMicro)
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
