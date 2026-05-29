package loadtest

import (
	"fmt"
	"time"
)

// QueueBloatGate defines the benchmark acceptance contract for queue storage
// engines. It is intentionally bloat-first: latency is capped, but WAL and
// relation growth decide whether an engine is allowed to become the default.
type QueueBloatGate struct {
	MaxDuplicateClaims int64
	MaxLostClaims      int64
	MaxP99Latency      time.Duration

	RequireWALImprovement bool
	RelationGates         []RelationBloatGate
}

type RelationBloatGate struct {
	Name                  string
	MaxDeadTupleDelta     int64
	MaxDeadTupleRatio     float64
	MaxTotalTableByteGain int64
	MaxTotalIndexByteGain int64
}

type QueueBloatGateResult struct {
	Passed   bool     `json:"passed"`
	Failures []string `json:"failures,omitempty"`
}

func EvaluateQueueBloatGate(comparison QueueBenchmarkComparison, gate QueueBloatGate) QueueBloatGateResult {
	var failures []string

	if comparison.Candidate.Counters.DuplicateClaims > gate.MaxDuplicateClaims {
		failures = append(failures, fmt.Sprintf(
			"duplicate claims = %d, max %d",
			comparison.Candidate.Counters.DuplicateClaims,
			gate.MaxDuplicateClaims,
		))
	}
	if comparison.Candidate.Counters.LostClaims > gate.MaxLostClaims {
		failures = append(failures, fmt.Sprintf(
			"lost claims = %d, max %d",
			comparison.Candidate.Counters.LostClaims,
			gate.MaxLostClaims,
		))
	}
	if gate.MaxP99Latency > 0 && comparison.Candidate.DequeueLatency.P99 > gate.MaxP99Latency {
		failures = append(failures, fmt.Sprintf(
			"candidate p99 = %s, max %s",
			comparison.Candidate.DequeueLatency.P99,
			gate.MaxP99Latency,
		))
	}
	if gate.RequireWALImprovement && comparison.WALBytesDelta >= 0 {
		failures = append(failures, fmt.Sprintf(
			"wal bytes delta = %+d, want improvement below baseline",
			comparison.WALBytesDelta,
		))
	}

	deltasByName := make(map[string]RelationBloatDelta, len(comparison.RelationDeltas))
	for _, delta := range comparison.RelationDeltas {
		deltasByName[delta.Name] = delta
	}
	candidateRelations := make(map[string]RelationBloatSample, len(comparison.Candidate.Relations))
	for _, rel := range comparison.Candidate.Relations {
		candidateRelations[rel.Name] = rel
	}

	for _, relGate := range gate.RelationGates {
		delta, ok := deltasByName[relGate.Name]
		if !ok {
			failures = append(failures, fmt.Sprintf("missing relation delta for %s", relGate.Name))
			continue
		}
		if delta.DeadTuplesDelta > relGate.MaxDeadTupleDelta {
			failures = append(failures, fmt.Sprintf(
				"%s dead tuple delta = %+d, max %+d",
				relGate.Name,
				delta.DeadTuplesDelta,
				relGate.MaxDeadTupleDelta,
			))
		}
		if relGate.MaxTotalTableByteGain > 0 && delta.TotalTableSizeDelta > relGate.MaxTotalTableByteGain {
			failures = append(failures, fmt.Sprintf(
				"%s table byte delta = %+d, max %+d",
				relGate.Name,
				delta.TotalTableSizeDelta,
				relGate.MaxTotalTableByteGain,
			))
		}
		if relGate.MaxTotalIndexByteGain > 0 && delta.TotalIndexSizeDelta > relGate.MaxTotalIndexByteGain {
			failures = append(failures, fmt.Sprintf(
				"%s index byte delta = %+d, max %+d",
				relGate.Name,
				delta.TotalIndexSizeDelta,
				relGate.MaxTotalIndexByteGain,
			))
		}
		if relGate.MaxDeadTupleRatio > 0 {
			rel, ok := candidateRelations[relGate.Name]
			if !ok {
				failures = append(failures, fmt.Sprintf("missing candidate relation sample for %s", relGate.Name))
				continue
			}
			if ratio := rel.DeadTupleRatio(); ratio > relGate.MaxDeadTupleRatio {
				failures = append(failures, fmt.Sprintf(
					"%s dead tuple ratio = %.4f, max %.4f",
					relGate.Name,
					ratio,
					relGate.MaxDeadTupleRatio,
				))
			}
		}
	}

	return QueueBloatGateResult{
		Passed:   len(failures) == 0,
		Failures: failures,
	}
}
