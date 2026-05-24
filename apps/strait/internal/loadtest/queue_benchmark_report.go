package loadtest

import (
	"fmt"
	"sort"
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
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
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
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
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
	return b.String()
}
