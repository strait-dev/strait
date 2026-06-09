package worker

import (
	"strconv"
	"strings"
)

func buildPartitionCycle(partitions []string, weightsRaw string) []string {
	if len(partitions) == 0 {
		return nil
	}

	weights := parsePartitionWeights(weightsRaw)
	cycle := make([]string, 0, len(partitions))
	for _, partition := range partitions {
		w := weights[partition]
		if w <= 0 {
			w = 1
		}
		for range w {
			cycle = append(cycle, partition)
		}
	}

	return cycle
}

func parsePartitionWeights(raw string) map[string]int {
	if raw == "" {
		return nil
	}

	weights := make(map[string]int)
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' }) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		keyRaw, weightRaw, ok := strings.Cut(token, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(keyRaw)
		weight, err := strconv.Atoi(strings.TrimSpace(weightRaw))
		if err != nil || weight <= 0 {
			continue
		}
		weights[key] = weight
	}

	return weights
}
