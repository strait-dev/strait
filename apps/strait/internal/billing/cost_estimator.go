package billing

import (
	"fmt"
	"sort"
	"time"

	"strait/internal/compute"

	"github.com/robfig/cron/v3"
)

// CheaperAlternative describes a less expensive preset option.
type CheaperAlternative struct {
	Preset     string  `json:"preset"`
	CostMicro  int64   `json:"cost_microusd"`
	SavingsPct float64 `json:"savings_pct"`
}

// CostEstimate is the result of estimating a job's compute cost.
type CostEstimate struct {
	CostMicro           int64               `json:"cost_microusd"`
	RateMicroPerSec     int64               `json:"rate_microusd_per_sec"`
	CreditRunsRemaining int64               `json:"credit_runs_remaining"`
	CheaperAlternative  *CheaperAlternative `json:"cheaper_alternative,omitempty"`
}

// EstimateJobCost estimates the worst-case cost for running a job with the given
// preset and timeout. It also checks for cheaper preset alternatives and calculates
// how many runs of this type the remaining credit can cover.
func EstimateJobCost(presetName string, timeoutSecs int, creditRemaining int64) (*CostEstimate, error) {
	cost, err := compute.EstimateCost(presetName, timeoutSecs)
	if err != nil {
		return nil, fmt.Errorf("estimating compute cost: %w", err)
	}

	preset, err := compute.PresetFromName(presetName)
	if err != nil {
		return nil, fmt.Errorf("looking up preset: %w", err)
	}

	var runsRemaining int64
	if cost > 0 {
		runsRemaining = creditRemaining / cost
	}

	est := &CostEstimate{
		CostMicro:           cost,
		RateMicroPerSec:     preset.CostPerSecond,
		CreditRunsRemaining: runsRemaining,
	}

	// Find cheaper alternative by looking at smaller presets.
	cheaper := findCheaperAlternative(presetName, timeoutSecs, cost)
	if cheaper != nil {
		est.CheaperAlternative = cheaper
	}

	return est, nil
}

func findCheaperAlternative(currentPreset string, timeoutSecs int, currentCost int64) *CheaperAlternative {
	if currentCost == 0 {
		return nil
	}

	currentIdx := compute.PresetIndex(currentPreset)
	if currentIdx <= 0 {
		return nil
	}

	type candidate struct {
		name string
		cost int64
	}

	var candidates []candidate
	for i := range currentIdx {
		name := compute.PresetOrder[i]
		cost, err := compute.EstimateCost(name, timeoutSecs)
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{name: name, cost: cost})
	}

	if len(candidates) == 0 {
		return nil
	}

	// Pick the cheapest alternative (which is the smallest preset).
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].cost < candidates[j].cost
	})

	best := candidates[0]
	savingsPct := float64(currentCost-best.cost) / float64(currentCost) * 100

	return &CheaperAlternative{
		Preset:     best.name,
		CostMicro:  best.cost,
		SavingsPct: savingsPct,
	}
}

// CronRunsPerDay estimates how many times a cron expression fires in 24 hours.
// Returns 0 for empty expressions. Returns error for invalid expressions.
func CronRunsPerDay(cronExpr string) (float64, error) {
	if cronExpr == "" {
		return 0, nil
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(cronExpr)
	if err != nil {
		return 0, fmt.Errorf("parsing cron expression: %w", err)
	}

	// Count firings in a 24-hour window starting from a fixed reference point.
	start := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC) // Monday
	end := start.Add(24 * time.Hour)

	count := 0
	t := sched.Next(start)
	for !t.IsZero() && t.Before(end) {
		count++
		t = sched.Next(t)
	}

	return float64(count), nil
}

// WhatIfEstimate describes the projected cost of a hypothetical job configuration.
type WhatIfEstimate struct {
	MonthlyCostUsd float64 `json:"monthly_cost_usd"`
	DailyCostUsd   float64 `json:"daily_cost_usd"`
	CostPerRunUsd  float64 `json:"cost_per_run_usd"`
	RunsPerDay     float64 `json:"runs_per_day"`
}

// EstimateWhatIf estimates the monthly cost of a hypothetical job configuration.
func EstimateWhatIf(preset string, timeoutSecs int, cronExpr string, count int) (*WhatIfEstimate, error) {
	if count <= 0 {
		count = 1
	}

	costMicro, err := compute.EstimateCost(preset, timeoutSecs)
	if err != nil {
		return nil, fmt.Errorf("estimating cost: %w", err)
	}

	runsPerDay := float64(count) // default: 1 run per day per job
	if cronExpr != "" {
		rpd, cronErr := CronRunsPerDay(cronExpr)
		if cronErr != nil {
			return nil, cronErr
		}
		runsPerDay = rpd * float64(count)
	}

	costPerRunUsd := float64(costMicro) / 1_000_000
	dailyCostUsd := costPerRunUsd * runsPerDay
	monthlyCostUsd := dailyCostUsd * 30

	return &WhatIfEstimate{
		MonthlyCostUsd: monthlyCostUsd,
		DailyCostUsd:   dailyCostUsd,
		CostPerRunUsd:  costPerRunUsd,
		RunsPerDay:     runsPerDay,
	}, nil
}
