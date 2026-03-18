package billing

import (
	"fmt"
	"sort"

	"strait/internal/compute"
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
