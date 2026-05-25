package loadtest

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// ComplexityClass labels the asymptotic shape of a measured hot path.
type ComplexityClass string

const (
	ComplexityConstant      ComplexityClass = "O(1)"
	ComplexityBatch         ComplexityClass = "O(batch)"
	ComplexityProjectActive ComplexityClass = "O(project_active)"
	ComplexityJobHistory    ComplexityClass = "O(job_history)"
	ComplexityWorkflowSteps ComplexityClass = "O(workflow_steps)"
)

// ScenarioMetric captures the user-visible shape of one load scenario.
type ScenarioMetric struct {
	Name      string         `json:"name"`
	RPS       float64        `json:"rps"`
	ErrorRate float64        `json:"error_rate"`
	Latency   LatencySummary `json:"latency"`
}

// SQLStatementMetric captures the pg_stat_statements view of one query family.
type SQLStatementMetric struct {
	Name        string        `json:"name"`
	QueryMatch  string        `json:"query_match"`
	Calls       int64         `json:"calls"`
	TotalTime   time.Duration `json:"total_time"`
	MeanTime    time.Duration `json:"mean_time"`
	P95Time     time.Duration `json:"p95_time"`
	Rows        int64         `json:"rows"`
	SharedReads int64         `json:"shared_reads"`
	SharedHits  int64         `json:"shared_hits"`
	WALBytes    int64         `json:"wal_bytes"`
}

// WaitMetric captures lock and pool waits that can dominate latency without
// showing up as CPU usage in Go profiles.
type WaitMetric struct {
	Name     string         `json:"name"`
	Count    int64          `json:"count"`
	Total    time.Duration  `json:"total"`
	P95      time.Duration  `json:"p95"`
	Max      time.Duration  `json:"max"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// TransactionMetric captures round-trip cost for a request or worker path.
type TransactionMetric struct {
	Name              string  `json:"name"`
	Operations        int64   `json:"operations"`
	Transactions      int64   `json:"transactions"`
	Statements        int64   `json:"statements"`
	TransactionsPerOp float64 `json:"transactions_per_op"`
	StatementsPerOp   float64 `json:"statements_per_op"`
}

// ProfileArtifact records a profile generated during a benchmark run.
type ProfileArtifact struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Path string `json:"path"`
}

// ComplexityLedgerEntry records current and target complexity for a hot path.
type ComplexityLedgerEntry struct {
	Area              string          `json:"area"`
	Current           ComplexityClass `json:"current"`
	Target            ComplexityClass `json:"target"`
	Evidence          string          `json:"evidence"`
	ImprovementReason string          `json:"improvement_reason"`
}

// DefaultPerformanceComplexityLedger returns the 0.1.6 hot-path ledger used by
// the phased performance work. Keeping it executable prevents the report and
// implementation plan from drifting apart as individual phases land.
func DefaultPerformanceComplexityLedger() []ComplexityLedgerEntry {
	return []ComplexityLedgerEntry{
		{
			Area:              "trigger admission",
			Current:           ComplexityJobHistory,
			Target:            ComplexityConstant,
			Evidence:          "trigger guard combines a blocking advisory lock with history-backed quota/rate checks",
			ImprovementReason: "row-atomic counters remove lock queues and bound work per trigger",
		},
		{
			Area:              "enqueue idempotency",
			Current:           ComplexityProjectActive,
			Target:            ComplexityConstant,
			Evidence:          "empty idempotency keys serialize same-job enqueues through one advisory key",
			ImprovementReason: "non-idempotent enqueue should not acquire idempotency serialization",
		},
		{
			Area:              "job health stats",
			Current:           ComplexityJobHistory,
			Target:            ComplexityConstant,
			Evidence:          "GetJobHealthStats aggregates over job_runs history for each requested window",
			ImprovementReason: "incremental or bounded stats avoid history scans in trigger/executor hot paths",
		},
		{
			Area:              "executor job load",
			Current:           ComplexityConstant,
			Target:            ComplexityConstant,
			Evidence:          "executor uses cache/singleflight but still has redundant call sites to remove",
			ImprovementReason: "passing the resolved job through helpers removes extra round trips",
		},
		{
			Area:              "workflow progression",
			Current:           ComplexityWorkflowSteps,
			Target:            ComplexityBatch,
			Evidence:          "fan-in progression can revisit workflow step state per completion",
			ImprovementReason: "batching by workflow run bounds progression work per processor batch",
		},
		{
			Area:              "cdc side effects",
			Current:           ComplexityBatch,
			Target:            ComplexityBatch,
			Evidence:          "one CDC message can run best-effort fan-out and durable handlers together",
			ImprovementReason: "separating durable side effects prevents redelivery amplification",
		},
		{
			Area:              "side-effect outbox claim",
			Current:           ComplexityBatch,
			Target:            ComplexityBatch,
			Evidence:          "durable side effects should claim bounded batches with indexed ready rows",
			ImprovementReason: "bounded leases keep flush cost proportional to claimed work",
		},
	}
}

// PerformanceBaselineReport is the phase-level report used to compare each
// optimization against the pre-change baseline.
type PerformanceBaselineReport struct {
	Name         string                  `json:"name"`
	StartedAt    time.Time               `json:"started_at"`
	Duration     time.Duration           `json:"duration"`
	Scenarios    []ScenarioMetric        `json:"scenarios"`
	SQL          []SQLStatementMetric    `json:"sql"`
	Waits        []WaitMetric            `json:"waits"`
	Transactions []TransactionMetric     `json:"transactions"`
	Bloat        []RelationBloatSample   `json:"bloat"`
	Profiles     []ProfileArtifact       `json:"profiles"`
	Complexity   []ComplexityLedgerEntry `json:"complexity"`
}

// PerformanceBaselineComparison captures whether a later phase actually moved
// the metrics that motivated the work.
type PerformanceBaselineComparison struct {
	Name                  string                    `json:"name"`
	Baseline              PerformanceBaselineReport `json:"baseline"`
	Candidate             PerformanceBaselineReport `json:"candidate"`
	ScenarioDeltas        []ScenarioDelta           `json:"scenario_deltas"`
	SQLDeltas             []SQLStatementDelta       `json:"sql_deltas"`
	WaitDeltas            []WaitDelta               `json:"wait_deltas"`
	TransactionDeltas     []TransactionDelta        `json:"transaction_deltas"`
	BloatDeltas           []RelationBloatDelta      `json:"bloat_deltas"`
	ComplexityRegressions []ComplexityLedgerEntry   `json:"complexity_regressions,omitempty"`
}

type ScenarioDelta struct {
	Name           string        `json:"name"`
	RPSDelta       float64       `json:"rps_delta"`
	ErrorRateDelta float64       `json:"error_rate_delta"`
	P95Delta       time.Duration `json:"p95_delta"`
	P99Delta       time.Duration `json:"p99_delta"`
}

type SQLStatementDelta struct {
	Name           string        `json:"name"`
	CallsDelta     int64         `json:"calls_delta"`
	TotalTimeDelta time.Duration `json:"total_time_delta"`
	MeanTimeDelta  time.Duration `json:"mean_time_delta"`
	WALBytesDelta  int64         `json:"wal_bytes_delta"`
}

type WaitDelta struct {
	Name       string        `json:"name"`
	CountDelta int64         `json:"count_delta"`
	TotalDelta time.Duration `json:"total_delta"`
	P95Delta   time.Duration `json:"p95_delta"`
}

type TransactionDelta struct {
	Name                   string  `json:"name"`
	TransactionsPerOpDelta float64 `json:"transactions_per_op_delta"`
	StatementsPerOpDelta   float64 `json:"statements_per_op_delta"`
}

func NewTransactionMetric(name string, operations, transactions, statements int64) TransactionMetric {
	metric := TransactionMetric{
		Name:         name,
		Operations:   operations,
		Transactions: transactions,
		Statements:   statements,
	}
	if operations > 0 {
		metric.TransactionsPerOp = float64(transactions) / float64(operations)
		metric.StatementsPerOp = float64(statements) / float64(operations)
	}
	return metric
}

func ComparePerformanceBaselineReports(name string, baseline, candidate PerformanceBaselineReport) PerformanceBaselineComparison {
	return PerformanceBaselineComparison{
		Name:                  name,
		Baseline:              baseline,
		Candidate:             candidate,
		ScenarioDeltas:        compareScenarios(baseline.Scenarios, candidate.Scenarios),
		SQLDeltas:             compareSQLStatements(baseline.SQL, candidate.SQL),
		WaitDeltas:            compareWaits(baseline.Waits, candidate.Waits),
		TransactionDeltas:     compareTransactions(baseline.Transactions, candidate.Transactions),
		BloatDeltas:           CompareRelationBloatSamples(baseline.Bloat, candidate.Bloat, completedFromScenarios(candidate.Scenarios)),
		ComplexityRegressions: complexityRegressions(baseline.Complexity, candidate.Complexity),
	}
}

func (r PerformanceBaselineReport) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", r.Name)
	fmt.Fprintf(&b, "- Duration: `%s`\n", r.Duration)
	fmt.Fprintf(&b, "- Scenarios: `%d`\n", len(r.Scenarios))
	fmt.Fprintf(&b, "- SQL families: `%d`\n", len(r.SQL))
	fmt.Fprintf(&b, "- Complexity entries: `%d`\n\n", len(r.Complexity))

	if len(r.Scenarios) > 0 {
		fmt.Fprintf(&b, "## Scenarios\n\n")
		fmt.Fprintf(&b, "| Scenario | RPS | Error %% | P50 | P95 | P99 |\n")
		fmt.Fprintf(&b, "|---|---:|---:|---:|---:|---:|\n")
		for _, scenario := range sortedScenarios(r.Scenarios) {
			fmt.Fprintf(&b, "| `%s` | %.2f | %.3f | %s | %s | %s |\n",
				scenario.Name,
				scenario.RPS,
				scenario.ErrorRate*100,
				scenario.Latency.P50,
				scenario.Latency.P95,
				scenario.Latency.P99,
			)
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(r.SQL) > 0 {
		fmt.Fprintf(&b, "## SQL\n\n")
		fmt.Fprintf(&b, "| Query family | Calls | Total | Mean | P95 | WAL bytes |\n")
		fmt.Fprintf(&b, "|---|---:|---:|---:|---:|---:|\n")
		for _, stmt := range sortedSQL(r.SQL) {
			fmt.Fprintf(&b, "| `%s` | %d | %s | %s | %s | %d |\n",
				stmt.Name,
				stmt.Calls,
				stmt.TotalTime,
				stmt.MeanTime,
				stmt.P95Time,
				stmt.WALBytes,
			)
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(r.Complexity) > 0 {
		fmt.Fprintf(&b, "## Complexity Ledger\n\n")
		fmt.Fprintf(&b, "| Area | Current | Target | Evidence |\n")
		fmt.Fprintf(&b, "|---|---:|---:|---|\n")
		for _, entry := range sortedComplexity(r.Complexity) {
			fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | %s |\n",
				entry.Area,
				entry.Current,
				entry.Target,
				entry.Evidence,
			)
		}
	}
	return b.String()
}

func (c PerformanceBaselineComparison) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", c.Name)
	fmt.Fprintf(&b, "- Baseline: `%s`\n", c.Baseline.Name)
	fmt.Fprintf(&b, "- Candidate: `%s`\n\n", c.Candidate.Name)

	if len(c.ScenarioDeltas) > 0 {
		fmt.Fprintf(&b, "## Scenario Deltas\n\n")
		fmt.Fprintf(&b, "| Scenario | RPS | Error %% | P95 | P99 |\n")
		fmt.Fprintf(&b, "|---|---:|---:|---:|---:|\n")
		for _, delta := range c.ScenarioDeltas {
			fmt.Fprintf(&b, "| `%s` | %.2f | %.3f | %s | %s |\n",
				delta.Name,
				delta.RPSDelta,
				delta.ErrorRateDelta*100,
				delta.P95Delta,
				delta.P99Delta,
			)
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(c.SQLDeltas) > 0 {
		fmt.Fprintf(&b, "## SQL Deltas\n\n")
		fmt.Fprintf(&b, "| Query family | Calls | Total | Mean | WAL bytes |\n")
		fmt.Fprintf(&b, "|---|---:|---:|---:|---:|\n")
		for _, delta := range c.SQLDeltas {
			fmt.Fprintf(&b, "| `%s` | %+d | %s | %s | %+d |\n",
				delta.Name,
				delta.CallsDelta,
				delta.TotalTimeDelta,
				delta.MeanTimeDelta,
				delta.WALBytesDelta,
			)
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(c.TransactionDeltas) > 0 {
		fmt.Fprintf(&b, "## Transaction Deltas\n\n")
		fmt.Fprintf(&b, "| Path | Txns/op | Statements/op |\n")
		fmt.Fprintf(&b, "|---|---:|---:|\n")
		for _, delta := range c.TransactionDeltas {
			fmt.Fprintf(&b, "| `%s` | %+.2f | %+.2f |\n",
				delta.Name,
				delta.TransactionsPerOpDelta,
				delta.StatementsPerOpDelta,
			)
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(c.ComplexityRegressions) > 0 {
		fmt.Fprintf(&b, "## Complexity Regressions\n\n")
		for _, entry := range c.ComplexityRegressions {
			fmt.Fprintf(&b, "- `%s`: current `%s`, target `%s`\n", entry.Area, entry.Current, entry.Target)
		}
	}
	return b.String()
}

func compareScenarios(baseline, candidate []ScenarioMetric) []ScenarioDelta {
	base := scenarioMap(baseline)
	names := scenarioNames(baseline, candidate)
	out := make([]ScenarioDelta, 0, len(names))
	cand := scenarioMap(candidate)
	for _, name := range names {
		a := base[name]
		b := cand[name]
		out = append(out, ScenarioDelta{
			Name:           name,
			RPSDelta:       b.RPS - a.RPS,
			ErrorRateDelta: b.ErrorRate - a.ErrorRate,
			P95Delta:       b.Latency.P95 - a.Latency.P95,
			P99Delta:       b.Latency.P99 - a.Latency.P99,
		})
	}
	return out
}

func compareSQLStatements(baseline, candidate []SQLStatementMetric) []SQLStatementDelta {
	base := sqlMap(baseline)
	cand := sqlMap(candidate)
	names := sqlNames(baseline, candidate)
	out := make([]SQLStatementDelta, 0, len(names))
	for _, name := range names {
		a := base[name]
		b := cand[name]
		out = append(out, SQLStatementDelta{
			Name:           name,
			CallsDelta:     b.Calls - a.Calls,
			TotalTimeDelta: b.TotalTime - a.TotalTime,
			MeanTimeDelta:  b.MeanTime - a.MeanTime,
			WALBytesDelta:  b.WALBytes - a.WALBytes,
		})
	}
	return out
}

func compareWaits(baseline, candidate []WaitMetric) []WaitDelta {
	base := waitMap(baseline)
	cand := waitMap(candidate)
	names := waitNames(baseline, candidate)
	out := make([]WaitDelta, 0, len(names))
	for _, name := range names {
		a := base[name]
		b := cand[name]
		out = append(out, WaitDelta{
			Name:       name,
			CountDelta: b.Count - a.Count,
			TotalDelta: b.Total - a.Total,
			P95Delta:   b.P95 - a.P95,
		})
	}
	return out
}

func compareTransactions(baseline, candidate []TransactionMetric) []TransactionDelta {
	base := transactionMap(baseline)
	cand := transactionMap(candidate)
	names := transactionNames(baseline, candidate)
	out := make([]TransactionDelta, 0, len(names))
	for _, name := range names {
		a := base[name]
		b := cand[name]
		out = append(out, TransactionDelta{
			Name:                   name,
			TransactionsPerOpDelta: b.TransactionsPerOp - a.TransactionsPerOp,
			StatementsPerOpDelta:   b.StatementsPerOp - a.StatementsPerOp,
		})
	}
	return out
}

func complexityRegressions(baseline, candidate []ComplexityLedgerEntry) []ComplexityLedgerEntry {
	targets := make(map[string]ComplexityClass, len(baseline))
	for _, entry := range baseline {
		targets[entry.Area] = entry.Target
	}
	out := make([]ComplexityLedgerEntry, 0)
	for _, entry := range candidate {
		target, ok := targets[entry.Area]
		if !ok {
			target = entry.Target
		}
		if complexityRank(entry.Current) > complexityRank(target) {
			out = append(out, entry)
		}
	}
	return out
}

func complexityRank(class ComplexityClass) int {
	switch class {
	case ComplexityConstant:
		return 1
	case ComplexityBatch:
		return 2
	case ComplexityProjectActive:
		return 3
	case ComplexityWorkflowSteps:
		return 4
	case ComplexityJobHistory:
		return 5
	default:
		return 6
	}
}

func completedFromScenarios(scenarios []ScenarioMetric) int64 {
	var total int64
	for _, scenario := range scenarios {
		total += int64(scenario.Latency.Count)
	}
	return total
}

func scenarioMap(values []ScenarioMetric) map[string]ScenarioMetric {
	out := make(map[string]ScenarioMetric, len(values))
	for _, value := range values {
		out[value.Name] = value
	}
	return out
}

func sqlMap(values []SQLStatementMetric) map[string]SQLStatementMetric {
	out := make(map[string]SQLStatementMetric, len(values))
	for _, value := range values {
		out[value.Name] = value
	}
	return out
}

func waitMap(values []WaitMetric) map[string]WaitMetric {
	out := make(map[string]WaitMetric, len(values))
	for _, value := range values {
		out[value.Name] = value
	}
	return out
}

func transactionMap(values []TransactionMetric) map[string]TransactionMetric {
	out := make(map[string]TransactionMetric, len(values))
	for _, value := range values {
		out[value.Name] = value
	}
	return out
}

func scenarioNames(a, b []ScenarioMetric) []string {
	names := make([]string, 0, len(a)+len(b))
	seen := make(map[string]bool, len(a)+len(b))
	for _, value := range a {
		if !seen[value.Name] {
			names = append(names, value.Name)
			seen[value.Name] = true
		}
	}
	for _, value := range b {
		if !seen[value.Name] {
			names = append(names, value.Name)
			seen[value.Name] = true
		}
	}
	slices.Sort(names)
	return names
}

func sqlNames(a, b []SQLStatementMetric) []string {
	names := make([]string, 0, len(a)+len(b))
	seen := make(map[string]bool, len(a)+len(b))
	for _, value := range a {
		if !seen[value.Name] {
			names = append(names, value.Name)
			seen[value.Name] = true
		}
	}
	for _, value := range b {
		if !seen[value.Name] {
			names = append(names, value.Name)
			seen[value.Name] = true
		}
	}
	slices.Sort(names)
	return names
}

func waitNames(a, b []WaitMetric) []string {
	names := make([]string, 0, len(a)+len(b))
	seen := make(map[string]bool, len(a)+len(b))
	for _, value := range a {
		if !seen[value.Name] {
			names = append(names, value.Name)
			seen[value.Name] = true
		}
	}
	for _, value := range b {
		if !seen[value.Name] {
			names = append(names, value.Name)
			seen[value.Name] = true
		}
	}
	slices.Sort(names)
	return names
}

func transactionNames(a, b []TransactionMetric) []string {
	names := make([]string, 0, len(a)+len(b))
	seen := make(map[string]bool, len(a)+len(b))
	for _, value := range a {
		if !seen[value.Name] {
			names = append(names, value.Name)
			seen[value.Name] = true
		}
	}
	for _, value := range b {
		if !seen[value.Name] {
			names = append(names, value.Name)
			seen[value.Name] = true
		}
	}
	slices.Sort(names)
	return names
}

func sortedScenarios(values []ScenarioMetric) []ScenarioMetric {
	out := append([]ScenarioMetric(nil), values...)
	slices.SortFunc(out, func(a, b ScenarioMetric) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

func sortedSQL(values []SQLStatementMetric) []SQLStatementMetric {
	out := append([]SQLStatementMetric(nil), values...)
	slices.SortFunc(out, func(a, b SQLStatementMetric) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

func sortedComplexity(values []ComplexityLedgerEntry) []ComplexityLedgerEntry {
	out := append([]ComplexityLedgerEntry(nil), values...)
	slices.SortFunc(out, func(a, b ComplexityLedgerEntry) int {
		return strings.Compare(a.Area, b.Area)
	})
	return out
}
