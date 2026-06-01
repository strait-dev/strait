package loadtest

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// RelationBloatSample is a point-in-time sample for one PostgreSQL relation.
type RelationBloatSample struct {
	Name           string `json:"name"`
	LiveTuples     int64  `json:"live_tuples"`
	DeadTuples     int64  `json:"dead_tuples"`
	TotalUpdates   int64  `json:"total_updates"`
	HOTUpdates     int64  `json:"hot_updates"`
	RelationSize   int64  `json:"relation_size_bytes"`
	TotalIndexSize int64  `json:"total_index_size_bytes"`
	TotalTableSize int64  `json:"total_table_size_bytes"`
}

func (s RelationBloatSample) DeadTupleRatio() float64 {
	total := s.LiveTuples + s.DeadTuples
	if total <= 0 {
		return 0
	}
	return float64(s.DeadTuples) / float64(total)
}

func (s RelationBloatSample) HOTUpdateRatio() float64 {
	if s.TotalUpdates <= 0 {
		return 0
	}
	return float64(s.HOTUpdates) / float64(s.TotalUpdates)
}

// LatencySummary captures stable percentile fields for benchmark reports.
type LatencySummary struct {
	Count int           `json:"count"`
	Min   time.Duration `json:"min"`
	P50   time.Duration `json:"p50"`
	P95   time.Duration `json:"p95"`
	P99   time.Duration `json:"p99"`
	Max   time.Duration `json:"max"`
}

func SummarizeLatencies(samples []time.Duration) LatencySummary {
	if len(samples) == 0 {
		return LatencySummary{}
	}
	sorted := append([]time.Duration(nil), samples...)
	slices.Sort(sorted)
	return LatencySummary{
		Count: len(sorted),
		Min:   sorted[0],
		P50:   percentile(sorted, 0.50),
		P95:   percentile(sorted, 0.95),
		P99:   percentile(sorted, 0.99),
		Max:   sorted[len(sorted)-1],
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := int(float64(len(sorted)-1)*p + 0.5)
	idx = min(max(idx, 0), len(sorted)-1)
	return sorted[idx]
}

type QueueBenchmarkCounters struct {
	Enqueued        int64 `json:"enqueued"`
	Dequeued        int64 `json:"dequeued"`
	Completed       int64 `json:"completed"`
	RetryRedelivery int64 `json:"retry_redelivery"`
	DuplicateClaims int64 `json:"duplicate_claims"`
	LostClaims      int64 `json:"lost_claims"`
	NotifyCount     int64 `json:"notify_count"`
	WALBytes        int64 `json:"wal_bytes"`
}

type QueueBenchmarkReport struct {
	Name           string                 `json:"name"`
	Engine         string                 `json:"engine"`
	StartedAt      time.Time              `json:"started_at"`
	Duration       time.Duration          `json:"duration"`
	Counters       QueueBenchmarkCounters `json:"counters"`
	DequeueLatency LatencySummary         `json:"dequeue_latency"`
	Relations      []RelationBloatSample  `json:"relations"`
	Plans          []SQLPlanSample        `json:"plans,omitempty"`
}

type SQLPlanSample struct {
	Name  string   `json:"name"`
	Lines []string `json:"lines"`
}

// RelationBloatDelta compares a relation sample to a baseline sample.
type RelationBloatDelta struct {
	Name                    string  `json:"name"`
	LiveTuplesDelta         int64   `json:"live_tuples_delta"`
	DeadTuplesDelta         int64   `json:"dead_tuples_delta"`
	DeadTupleRatioDelta     float64 `json:"dead_tuple_ratio_delta"`
	TotalUpdatesDelta       int64   `json:"total_updates_delta"`
	HOTUpdatesDelta         int64   `json:"hot_updates_delta"`
	HOTUpdateRatioDelta     float64 `json:"hot_update_ratio_delta"`
	RelationSizeDelta       int64   `json:"relation_size_bytes_delta"`
	TotalIndexSizeDelta     int64   `json:"total_index_size_bytes_delta"`
	TotalTableSizeDelta     int64   `json:"total_table_size_bytes_delta"`
	DeadTuplesPerKCompleted float64 `json:"dead_tuples_per_1000_completed"`
	TableBytesPerKCompleted float64 `json:"table_bytes_per_1000_completed"`
	IndexBytesPerKCompleted float64 `json:"index_bytes_per_1000_completed"`
}

// QueueBenchmarkComparison captures the before/after signal used to decide
// whether a queue implementation reduces bloat enough to justify latency cost.
type QueueBenchmarkComparison struct {
	Name             string                 `json:"name"`
	BaselineEngine   string                 `json:"baseline_engine"`
	CandidateEngine  string                 `json:"candidate_engine"`
	Baseline         QueueBenchmarkReport   `json:"baseline"`
	Candidate        QueueBenchmarkReport   `json:"candidate"`
	CounterDelta     QueueBenchmarkCounters `json:"counter_delta"`
	RelationDeltas   []RelationBloatDelta   `json:"relation_deltas"`
	P99LatencyDelta  time.Duration          `json:"p99_latency_delta"`
	ThroughputDelta  float64                `json:"throughput_delta_runs_per_second"`
	WALBytesDelta    int64                  `json:"wal_bytes_delta"`
	ImprovementHints []ImprovementHint      `json:"improvement_hints"`
}

type ImprovementHint struct {
	Area   string `json:"area"`
	Metric string `json:"metric"`
	Detail string `json:"detail"`
}

func CompareQueueBenchmarkReports(name string, baseline, candidate QueueBenchmarkReport) QueueBenchmarkComparison {
	comparison := QueueBenchmarkComparison{
		Name:            name,
		BaselineEngine:  baseline.Engine,
		CandidateEngine: candidate.Engine,
		Baseline:        baseline,
		Candidate:       candidate,
		CounterDelta: QueueBenchmarkCounters{
			Enqueued:        candidate.Counters.Enqueued - baseline.Counters.Enqueued,
			Dequeued:        candidate.Counters.Dequeued - baseline.Counters.Dequeued,
			Completed:       candidate.Counters.Completed - baseline.Counters.Completed,
			RetryRedelivery: candidate.Counters.RetryRedelivery - baseline.Counters.RetryRedelivery,
			DuplicateClaims: candidate.Counters.DuplicateClaims - baseline.Counters.DuplicateClaims,
			LostClaims:      candidate.Counters.LostClaims - baseline.Counters.LostClaims,
			NotifyCount:     candidate.Counters.NotifyCount - baseline.Counters.NotifyCount,
			WALBytes:        candidate.Counters.WALBytes - baseline.Counters.WALBytes,
		},
		P99LatencyDelta: candidate.DequeueLatency.P99 - baseline.DequeueLatency.P99,
		ThroughputDelta: throughput(candidate) - throughput(baseline),
		WALBytesDelta:   candidate.Counters.WALBytes - baseline.Counters.WALBytes,
		RelationDeltas:  CompareRelationBloatSamples(baseline.Relations, candidate.Relations, candidate.Counters.Completed),
	}
	comparison.ImprovementHints = BuildImprovementHints(comparison)
	return comparison
}

func CompareRelationBloatSamples(baseline, candidate []RelationBloatSample, completed int64) []RelationBloatDelta {
	byName := make(map[string]RelationBloatSample, len(baseline))
	for _, sample := range baseline {
		byName[sample.Name] = sample
	}
	names := make([]string, 0, len(candidate))
	candidateByName := make(map[string]RelationBloatSample, len(candidate))
	for _, sample := range candidate {
		candidateByName[sample.Name] = sample
		names = append(names, sample.Name)
	}
	for _, sample := range baseline {
		if _, ok := candidateByName[sample.Name]; !ok {
			names = append(names, sample.Name)
		}
	}
	slices.Sort(names)

	out := make([]RelationBloatDelta, 0, len(names))
	for _, name := range names {
		base := byName[name]
		cand := candidateByName[name]
		delta := RelationBloatDelta{
			Name:                    name,
			LiveTuplesDelta:         cand.LiveTuples - base.LiveTuples,
			DeadTuplesDelta:         cand.DeadTuples - base.DeadTuples,
			DeadTupleRatioDelta:     cand.DeadTupleRatio() - base.DeadTupleRatio(),
			TotalUpdatesDelta:       cand.TotalUpdates - base.TotalUpdates,
			HOTUpdatesDelta:         cand.HOTUpdates - base.HOTUpdates,
			HOTUpdateRatioDelta:     cand.HOTUpdateRatio() - base.HOTUpdateRatio(),
			RelationSizeDelta:       cand.RelationSize - base.RelationSize,
			TotalIndexSizeDelta:     cand.TotalIndexSize - base.TotalIndexSize,
			TotalTableSizeDelta:     cand.TotalTableSize - base.TotalTableSize,
			DeadTuplesPerKCompleted: perK(cand.DeadTuples-base.DeadTuples, completed),
			TableBytesPerKCompleted: perK(cand.TotalTableSize-base.TotalTableSize, completed),
			IndexBytesPerKCompleted: perK(cand.TotalIndexSize-base.TotalIndexSize, completed),
		}
		out = append(out, delta)
	}
	return out
}

func BuildImprovementHints(comparison QueueBenchmarkComparison) []ImprovementHint {
	hints := make([]ImprovementHint, 0, 4)
	if comparison.P99LatencyDelta > 0 {
		hints = append(hints, ImprovementHint{
			Area:   "dequeue_latency",
			Metric: "p99_latency_delta",
			Detail: fmt.Sprintf("candidate p99 is %s slower than baseline", comparison.P99LatencyDelta),
		})
	}
	if comparison.ThroughputDelta < 0 {
		hints = append(hints, ImprovementHint{
			Area:   "throughput",
			Metric: "runs_per_second_delta",
			Detail: fmt.Sprintf("candidate throughput is %.2f runs/s lower than baseline", -comparison.ThroughputDelta),
		})
	}
	if comparison.WALBytesDelta > 0 {
		hints = append(hints, ImprovementHint{
			Area:   "wal",
			Metric: "wal_bytes_delta",
			Detail: fmt.Sprintf("candidate wrote %d more WAL bytes than baseline", comparison.WALBytesDelta),
		})
	}
	for _, delta := range comparison.RelationDeltas {
		if delta.DeadTuplesDelta > 0 || delta.TotalIndexSizeDelta > 0 {
			hints = append(hints, ImprovementHint{
				Area:   delta.Name,
				Metric: "relation_bloat_delta",
				Detail: fmt.Sprintf("dead tuples %+d, index bytes %+d", delta.DeadTuplesDelta, delta.TotalIndexSizeDelta),
			})
		}
	}
	return hints
}

func (r QueueBenchmarkReport) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", r.Name)
	fmt.Fprintf(&b, "- Engine: `%s`\n", r.Engine)
	fmt.Fprintf(&b, "- Duration: `%s`\n", r.Duration)
	fmt.Fprintf(&b, "- Enqueued: `%d`\n", r.Counters.Enqueued)
	fmt.Fprintf(&b, "- Dequeued: `%d`\n", r.Counters.Dequeued)
	fmt.Fprintf(&b, "- Completed: `%d`\n", r.Counters.Completed)
	fmt.Fprintf(&b, "- Duplicate claims: `%d`\n", r.Counters.DuplicateClaims)
	fmt.Fprintf(&b, "- Lost claims: `%d`\n", r.Counters.LostClaims)
	fmt.Fprintf(&b, "- Notifications observed: `%d`\n", r.Counters.NotifyCount)
	fmt.Fprintf(&b, "- WAL bytes: `%d`\n\n", r.Counters.WALBytes)
	fmt.Fprintf(&b, "## Dequeue Latency\n\n")
	fmt.Fprintf(&b, "| Count | Min | P50 | P95 | P99 | Max |\n")
	fmt.Fprintf(&b, "|---:|---:|---:|---:|---:|---:|\n")
	fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s |\n\n",
		r.DequeueLatency.Count,
		r.DequeueLatency.Min,
		r.DequeueLatency.P50,
		r.DequeueLatency.P95,
		r.DequeueLatency.P99,
		r.DequeueLatency.Max,
	)
	fmt.Fprintf(&b, "## Relations\n\n")
	fmt.Fprintf(&b, "| Relation | Live | Dead | Dead %% | Updates | HOT %% | Table bytes | Index bytes |\n")
	fmt.Fprintf(&b, "|---|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, rel := range r.Relations {
		fmt.Fprintf(&b, "| `%s` | %d | %d | %.2f | %d | %.2f | %d | %d |\n",
			rel.Name,
			rel.LiveTuples,
			rel.DeadTuples,
			rel.DeadTupleRatio()*100,
			rel.TotalUpdates,
			rel.HOTUpdateRatio()*100,
			rel.TotalTableSize,
			rel.TotalIndexSize,
		)
	}
	if len(r.Plans) > 0 {
		fmt.Fprintf(&b, "\n## SQL Plans\n\n")
		for _, plan := range r.Plans {
			fmt.Fprintf(&b, "### %s\n\n", plan.Name)
			fmt.Fprintf(&b, "```text\n")
			for _, line := range plan.Lines {
				fmt.Fprintf(&b, "%s\n", line)
			}
			fmt.Fprintf(&b, "```\n\n")
		}
	}
	return b.String()
}

func (c QueueBenchmarkComparison) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", c.Name)
	fmt.Fprintf(&b, "- Baseline: `%s`\n", c.BaselineEngine)
	fmt.Fprintf(&b, "- Candidate: `%s`\n", c.CandidateEngine)
	fmt.Fprintf(&b, "- P99 latency delta: `%s`\n", c.P99LatencyDelta)
	fmt.Fprintf(&b, "- Throughput delta: `%.2f runs/s`\n", c.ThroughputDelta)
	fmt.Fprintf(&b, "- WAL bytes delta: `%d`\n\n", c.WALBytesDelta)

	fmt.Fprintf(&b, "## Counters\n\n")
	fmt.Fprintf(&b, "| Metric | Baseline | Candidate | Delta |\n")
	fmt.Fprintf(&b, "|---|---:|---:|---:|\n")
	counterRows := []struct {
		name      string
		baseline  int64
		candidate int64
		delta     int64
	}{
		{"enqueued", c.Baseline.Counters.Enqueued, c.Candidate.Counters.Enqueued, c.CounterDelta.Enqueued},
		{"dequeued", c.Baseline.Counters.Dequeued, c.Candidate.Counters.Dequeued, c.CounterDelta.Dequeued},
		{"completed", c.Baseline.Counters.Completed, c.Candidate.Counters.Completed, c.CounterDelta.Completed},
		{"retry_redelivery", c.Baseline.Counters.RetryRedelivery, c.Candidate.Counters.RetryRedelivery, c.CounterDelta.RetryRedelivery},
		{"duplicate_claims", c.Baseline.Counters.DuplicateClaims, c.Candidate.Counters.DuplicateClaims, c.CounterDelta.DuplicateClaims},
		{"lost_claims", c.Baseline.Counters.LostClaims, c.Candidate.Counters.LostClaims, c.CounterDelta.LostClaims},
		{"notify_count", c.Baseline.Counters.NotifyCount, c.Candidate.Counters.NotifyCount, c.CounterDelta.NotifyCount},
		{"wal_bytes", c.Baseline.Counters.WALBytes, c.Candidate.Counters.WALBytes, c.CounterDelta.WALBytes},
	}
	for _, row := range counterRows {
		fmt.Fprintf(&b, "| `%s` | %d | %d | %+d |\n", row.name, row.baseline, row.candidate, row.delta)
	}

	fmt.Fprintf(&b, "\n## Relation Deltas\n\n")
	fmt.Fprintf(&b, "| Relation | Dead tuples | Dead / 1k completed | Updates | HOT updates | HOT delta | Table bytes | Index bytes |\n")
	fmt.Fprintf(&b, "|---|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, delta := range c.RelationDeltas {
		fmt.Fprintf(&b, "| `%s` | %+d | %.2f | %+d | %+d | %+.2f%% | %+d | %+d |\n",
			delta.Name,
			delta.DeadTuplesDelta,
			delta.DeadTuplesPerKCompleted,
			delta.TotalUpdatesDelta,
			delta.HOTUpdatesDelta,
			delta.HOTUpdateRatioDelta*100,
			delta.TotalTableSizeDelta,
			delta.TotalIndexSizeDelta,
		)
	}

	if len(c.ImprovementHints) > 0 {
		fmt.Fprintf(&b, "\n## Improvement Hints\n\n")
		for _, hint := range c.ImprovementHints {
			fmt.Fprintf(&b, "- `%s` / `%s`: %s\n", hint.Area, hint.Metric, hint.Detail)
		}
	}
	if len(c.Baseline.Plans) > 0 || len(c.Candidate.Plans) > 0 {
		fmt.Fprintf(&b, "\n## SQL Plans\n\n")
		writePlansMarkdown(&b, "Baseline", c.Baseline.Plans)
		writePlansMarkdown(&b, "Candidate", c.Candidate.Plans)
	}
	return b.String()
}

func writePlansMarkdown(b *strings.Builder, label string, plans []SQLPlanSample) {
	for _, plan := range plans {
		fmt.Fprintf(b, "### %s: %s\n\n", label, plan.Name)
		fmt.Fprintf(b, "```text\n")
		for _, line := range plan.Lines {
			fmt.Fprintf(b, "%s\n", line)
		}
		fmt.Fprintf(b, "```\n\n")
	}
}

func throughput(report QueueBenchmarkReport) float64 {
	if report.Duration <= 0 {
		return 0
	}
	return float64(report.Counters.Dequeued) / report.Duration.Seconds()
}

func perK(value, completed int64) float64 {
	if completed <= 0 {
		return 0
	}
	return float64(value) * 1000 / float64(completed)
}
