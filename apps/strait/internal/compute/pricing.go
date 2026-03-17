package compute

import "math"

// CalculateCost returns the cost in micro-USD for running a given preset for the specified duration.
// Negative durations are clamped to zero.
func CalculateCost(presetName string, durationSecs float64) (int64, error) {
	preset, err := PresetFromName(presetName)
	if err != nil {
		return 0, err
	}
	if durationSecs <= 0 {
		return 0, nil
	}
	cost := float64(preset.CostPerSecond) * durationSecs
	rounded := int64(math.Round(cost))
	if rounded < 0 {
		return 0, nil
	}
	return rounded, nil
}

// EstimateCost returns the worst-case cost in micro-USD for a preset running for the full timeout.
func EstimateCost(presetName string, timeoutSecs int) (int64, error) {
	return CalculateCost(presetName, float64(timeoutSecs))
}
