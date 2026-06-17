//go:build !integration

package clickhouse

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"strait/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedQuery struct {
	columns []string
	rows    [][]driver.Value
	err     error
	rowsErr error
}

type scriptedSQLState struct {
	mu           sync.Mutex
	queries      []scriptedQuery
	queryIndex   int
	execErr      error
	execCount    int
	acceptsExecs bool
}

type scriptedConnector struct {
	state *scriptedSQLState
}

func (c *scriptedConnector) Connect(context.Context) (driver.Conn, error) {
	return &scriptedConn{state: c.state}, nil
}

func (*scriptedConnector) Driver() driver.Driver {
	return scriptedDriver{}
}

type scriptedDriver struct{}

func (scriptedDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("scripted driver requires OpenDB")
}

type scriptedConn struct {
	state *scriptedSQLState
}

func (*scriptedConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("scripted driver does not prepare statements")
}

func (*scriptedConn) Close() error {
	return nil
}

func (*scriptedConn) Begin() (driver.Tx, error) {
	return nil, errors.New("scripted driver does not start transactions")
}

func (*scriptedConn) CheckNamedValue(*driver.NamedValue) error {
	return nil
}

func (c *scriptedConn) QueryContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	if c.state.queryIndex >= len(c.state.queries) {
		return nil, fmt.Errorf("unexpected query %d", c.state.queryIndex+1)
	}
	query := c.state.queries[c.state.queryIndex]
	c.state.queryIndex++
	if query.err != nil {
		return nil, query.err
	}
	return &scriptedRows{
		columns: query.columns,
		rows:    query.rows,
		rowsErr: query.rowsErr,
	}, nil
}

func (c *scriptedConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	if !c.state.acceptsExecs {
		return nil, errors.New("unexpected exec")
	}
	c.state.execCount++
	if c.state.execErr != nil {
		return nil, c.state.execErr
	}
	return driver.RowsAffected(1), nil
}

type scriptedRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
	rowsErr error
}

func (r *scriptedRows) Columns() []string {
	if len(r.columns) > 0 {
		return r.columns
	}
	if len(r.rows) == 0 {
		return nil
	}
	columns := make([]string, len(r.rows[0]))
	for i := range columns {
		columns[i] = fmt.Sprintf("col_%d", i)
	}
	return columns
}

func (*scriptedRows) Close() error {
	return nil
}

func (r *scriptedRows) Next(dest []driver.Value) error {
	if r.index < len(r.rows) {
		copy(dest, r.rows[r.index])
		r.index++
		return nil
	}
	if r.rowsErr != nil {
		err := r.rowsErr
		r.rowsErr = nil
		return err
	}
	return io.EOF
}

func row(values ...driver.Value) []driver.Value {
	return values
}

func cols(count int) []string {
	columns := make([]string, count)
	for i := range columns {
		columns[i] = fmt.Sprintf("col_%d", i)
	}
	return columns
}

func q(values ...driver.Value) scriptedQuery {
	return scriptedQuery{
		columns: cols(len(values)),
		rows:    [][]driver.Value{row(values...)},
	}
}

func qRows(values ...[]driver.Value) scriptedQuery {
	columnCount := 0
	if len(values) > 0 {
		columnCount = len(values[0])
	}
	return scriptedQuery{
		columns: cols(columnCount),
		rows:    values,
	}
}

func newScriptedClient(t *testing.T, state *scriptedSQLState) *Client {
	t.Helper()

	db := sql.OpenDB(&scriptedConnector{state: state})
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return &Client{db: db, logger: slog.Default()}
}

type successfulPgHealthQuerier struct {
	total  int
	active int
	depth  int
}

func (q successfulPgHealthQuerier) CountJobs(context.Context, string) (int, int, error) {
	return q.total, q.active, nil
}

func (q successfulPgHealthQuerier) QueueDepth(context.Context, string) (int, error) {
	return q.depth, nil
}

func TestAnalyticsStore_GetPerformanceAnalyticsSuccess(t *testing.T) {
	t.Parallel()

	state := &scriptedSQLState{
		queries: []scriptedQuery{
			qRows(row("job-1", "email-digest", 1.25, 2.5, int64(6), int64(1))),
			q(int64(8), int64(2), int64(1), int64(0)),
			q(0.7273, 3.75),
		},
	}
	s := NewAnalyticsStore(newScriptedClient(t, state), successfulPgHealthQuerier{
		total:  12,
		active: 9,
		depth:  4,
	})

	got, err := s.GetPerformanceAnalytics(context.Background(), "proj-1", 24)
	require.NoError(t, err)
	require.Len(t, got.SlowestJobs, 1)
	assert.Equal(t, store.JobPerformance{
		JobID:           "job-1",
		JobSlug:         "email-digest",
		AvgDurationSecs: 1.25,
		P95DurationSecs: 2.5,
		TotalRuns:       6,
		FailedRuns:      1,
	}, got.SlowestJobs[0])
	assert.Equal(t, store.ThroughputStats{
		Completed:   8,
		Failed:      2,
		TimedOut:    1,
		Canceled:    0,
		PeriodHours: 24,
	}, got.Throughput)
	assert.Equal(t, store.HealthSummary{
		TotalJobs:       12,
		ActiveJobs:      9,
		SuccessRate:     0.7273,
		AvgDurationSecs: 3.75,
		QueueDepth:      4,
	}, got.HealthSummary)
}

func TestAnalyticsStore_GetCostAnalyticsSuccess(t *testing.T) {
	t.Parallel()

	state := &scriptedSQLState{
		queries: []scriptedQuery{
			q(int64(1200), int64(3)),
			qRows(row("job-1", "email-digest", int64(900), int64(2))),
		},
	}
	s := NewAnalyticsStore(newScriptedClient(t, state), nil)
	from, to := longRange()

	got, err := s.GetCostAnalytics(context.Background(), "proj-1", from, to)
	require.NoError(t, err)
	assert.Equal(t, int64(1200), got.TotalSpendMicrousd)
	assert.Equal(t, 3, got.RunCount)
	require.Len(t, got.ByJob, 1)
	assert.Equal(t, store.CostByJob{
		JobID:        "job-1",
		JobSlug:      "email-digest",
		CostMicrousd: 900,
		RunCount:     2,
	}, got.ByJob[0])
}

func TestAnalyticsStore_SuccessfulRowScans(t *testing.T) {
	t.Parallel()

	period := time.Date(2025, 2, 3, 4, 0, 0, 0, time.UTC)
	later := period.Add(time.Hour)
	webhookURL := "https://user:pass@example.com/hook?token=secret"
	state := &scriptedSQLState{
		queries: []scriptedQuery{
			qRows(row(period, int64(100), int64(2))),
			qRows(row("job-1", "email-digest", int64(200), int64(3))),
			qRows(row("run-1", "job-1", int64(300), 100.0, 20.0, 10.0)),
			q(int64(4), int64(3), int64(1), int64(0), 45.5),
			qRows(row(period, int64(7), int64(2), int64(1), int64(10))),
			qRows(row("<1s", int64(2)), row("1-5s", int64(6))),
			qRows(
				row("deadline exceeded", int64(2), period, "run-old"),
				row("timeout waiting for worker", int64(3), later, "run-new"),
				row("panic: boom", int64(1), period, "run-panic"),
			),
			q(int64(10), int64(8), int64(1), int64(1), 0.8, 12.5, 20.5),
			qRows(row("api", int64(5), int64(4), int64(1), 10.5)),
			qRows(row(period, int64(3), int64(1), 11.0, 19.0)),
			qRows(row("job-1", "email-digest", int64(7), 0.9, 13.0, int64(444))),
			qRows(row("job-1", "email-digest", int64(9), 0.8, int64(2), int64(1))),
			qRows(row("version-1", int64(4), int64(3), int64(1), 9.5)),
			qRows(row("job-1", "email-digest", int64(500), int64(5), 100.0)),
			qRows(row("job-1", "email-digest", int64(2), int64(5), 0.4)),
			qRows(row("env", "prod", int64(10), int64(9), int64(1), 15.0)),
			qRows(row("env", "prod", int64(2), int64(10), 0.2)),
			qRows(row("env", "prod", int64(1000), int64(10))),
			qRows(row("send-email", 11.0, 17.0, int64(3), 0.33)),
			qRows(row(period, int64(8), int64(1), int64(1))),
			q(int64(10), int64(8), int64(1), 0.8, 33.0),
			qRows(row(webhookURL, int64(5), int64(4), int64(1), int64(0), 10.0, 20.0)),
			qRows(row(webhookURL, period, 0.8, 12.0)),
			qRows(row(webhookURL, int64(2), int64(10), 0.2, "")),
			qRows(row(period, int64(3), int64(2), int64(1))),
			q(10.0, 8.0, 20.0, 30.0, int64(5)),
			q(100.0, 3000.0, 12.5),
			qRows(row("api", int64(100), int64(2), 0.0), row("cron", int64(300), int64(3), 0.0)),
		},
	}
	s := NewAnalyticsStore(newScriptedClient(t, state), nil)
	from, to := longRange()

	trends, err := s.GetCostTrends(context.Background(), "proj-1", from, to)
	require.NoError(t, err)
	require.Len(t, trends, 1)
	assert.Equal(t, period.Format(time.RFC3339), trends[0].Period)
	assert.Equal(t, int64(100), trends[0].SpendMicrousd)

	topCosts, err := s.GetTopCosts(context.Background(), "proj-1", from, to, 10)
	require.NoError(t, err)
	require.Len(t, topCosts, 1)
	assert.Equal(t, "job", topCosts[0].ItemType)
	assert.Equal(t, int64(200), topCosts[0].CostMicrousd)

	outliers, err := s.GetCostOutliers(context.Background(), "proj-1", from, to, 2.0)
	require.NoError(t, err)
	require.Len(t, outliers, 1)
	assert.InDelta(t, 10.0, outliers[0].DeviationsAbove, 0.001)

	approvals, err := s.GetApprovalStats(context.Background(), "proj-1", from, to)
	require.NoError(t, err)
	assert.Equal(t, 4, approvals.TotalRequested)
	assert.InDelta(t, 45.5, approvals.AvgApprovalTimeSecs, 0.001)

	timeline, err := s.GetRunTimeline(context.Background(), "proj-1", from, to, "day")
	require.NoError(t, err)
	require.Len(t, timeline, 1)
	assert.Equal(t, 10, timeline[0].Total)

	durations, err := s.GetRunDurationDistribution(context.Background(), "proj-1", from, to)
	require.NoError(t, err)
	require.Len(t, durations, 2)
	assert.InDelta(t, 25.0, durations[0].Pct, 0.001)
	assert.InDelta(t, 75.0, durations[1].Pct, 0.001)

	reasons, err := s.GetRunFailureReasons(context.Background(), "proj-1", from, to, 10)
	require.NoError(t, err)
	require.Len(t, reasons, 2)
	assert.Equal(t, "timeout", reasons[0].Message)
	assert.Equal(t, 5, reasons[0].Count)
	assert.Equal(t, "run-new", reasons[0].ExampleRunID)
	assert.Equal(t, "panic", reasons[1].Message)

	summary, err := s.GetRunSummary(context.Background(), "proj-1", from, to)
	require.NoError(t, err)
	assert.Equal(t, 10, summary.Total)
	assert.InDelta(t, 0.8, summary.SuccessRate, 0.001)

	byTrigger, err := s.GetRunsByTrigger(context.Background(), "proj-1", from, to)
	require.NoError(t, err)
	require.Len(t, byTrigger, 1)
	assert.Equal(t, "api", byTrigger[0].TriggerType)

	history, err := s.GetJobHistory(context.Background(), "proj-1", "job-1", from, to, "day")
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, period.Format(time.RFC3339), history[0].Period)

	comparison, err := s.GetJobComparison(context.Background(), "proj-1", []string{"job-1"}, from, to)
	require.NoError(t, err)
	require.Len(t, comparison, 1)
	assert.Equal(t, int64(444), comparison[0].Cost)

	reliability, err := s.GetJobReliability(context.Background(), "proj-1", from, to, 10)
	require.NoError(t, err)
	require.Len(t, reliability, 1)
	assert.Equal(t, 1, reliability[0].ConsecutiveFailures)

	versions, err := s.GetRunsByVersion(context.Background(), "proj-1", "job-1", from, to)
	require.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, "version-1", versions[0].VersionID)

	rankings, err := s.GetJobCostRanking(context.Background(), "proj-1", from, to, 10)
	require.NoError(t, err)
	require.Len(t, rankings, 1)
	assert.InDelta(t, 100.0, rankings[0].AvgCostPerRun, 0.001)

	failingJobs, err := s.GetTopFailingJobs(context.Background(), "proj-1", from, to, 10)
	require.NoError(t, err)
	require.Len(t, failingJobs, 1)
	assert.InDelta(t, 0.4, failingJobs[0].FailureRate, 0.001)

	tagSummary, err := s.GetTagSummary(context.Background(), "proj-1", from, to, 10)
	require.NoError(t, err)
	require.Len(t, tagSummary, 1)
	assert.Equal(t, "env", tagSummary[0].TagKey)

	failingTags, err := s.GetTopFailingTags(context.Background(), "proj-1", from, to, 10)
	require.NoError(t, err)
	require.Len(t, failingTags, 1)
	assert.Equal(t, 2, failingTags[0].Failed)

	tagCosts, err := s.GetTagCost(context.Background(), "proj-1", from, to, 10)
	require.NoError(t, err)
	require.Len(t, tagCosts, 1)
	assert.Equal(t, int64(1000), tagCosts[0].TotalCost)

	stepDurations, err := s.GetWorkflowStepDurations(context.Background(), "proj-1", "workflow-1", from, to)
	require.NoError(t, err)
	require.Len(t, stepDurations, 1)
	assert.Equal(t, "send-email", stepDurations[0].StepRef)

	completions, err := s.GetWorkflowCompletionRates(context.Background(), "proj-1", from, to, "day")
	require.NoError(t, err)
	require.Len(t, completions, 1)
	assert.Equal(t, 8, completions[0].Completed)

	workflowSummary, err := s.GetWorkflowSummary(context.Background(), "proj-1", from, to)
	require.NoError(t, err)
	assert.Equal(t, 10, workflowSummary.Total)

	webhookStats, err := s.GetWebhookDeliveryStats(context.Background(), "proj-1", from, to)
	require.NoError(t, err)
	require.Len(t, webhookStats, 1)
	assert.NotContains(t, webhookStats[0].URL, "secret")

	webhookHealth, err := s.GetWebhookEndpointHealth(context.Background(), "proj-1", from, to, "day")
	require.NoError(t, err)
	require.Len(t, webhookHealth, 1)
	assert.Equal(t, period.Format(time.RFC3339), webhookHealth[0].Period)
	assert.NotContains(t, webhookHealth[0].URL, "pass")

	failingWebhooks, err := s.GetTopFailingWebhooks(context.Background(), "proj-1", from, to, 10)
	require.NoError(t, err)
	require.Len(t, failingWebhooks, 1)
	assert.Equal(t, 2, failingWebhooks[0].Failed)
	assert.NotContains(t, failingWebhooks[0].URL, "token")

	eventVolume, err := s.GetEventVolume(context.Background(), "proj-1", from, to, "day")
	require.NoError(t, err)
	require.Len(t, eventVolume, 1)
	assert.Equal(t, 3, eventVolume[0].Created)

	latency, err := s.GetEventLatency(context.Background(), "proj-1", from, to)
	require.NoError(t, err)
	assert.Equal(t, 5, latency.Count)

	forecast, err := s.GetCostForecast(context.Background(), "proj-1", from, to)
	require.NoError(t, err)
	assert.InDelta(t, 3000.0, forecast.ProjectedMonthly, 0.001)

	costByTrigger, err := s.GetCostByTrigger(context.Background(), "proj-1", from, to)
	require.NoError(t, err)
	require.Len(t, costByTrigger, 2)
	assert.InDelta(t, 25.0, costByTrigger[0].Pct, 0.001)
	assert.InDelta(t, 75.0, costByTrigger[1].Pct, 0.001)
}

func TestCreateSchema_SucceedsWithScriptedClient(t *testing.T) {
	t.Parallel()

	state := &scriptedSQLState{acceptsExecs: true}
	client := newScriptedClient(t, state)

	require.NoError(t, CreateSchema(context.Background(), client))
	assert.Positive(t, state.execCount)
}

func TestExporter_DefaultMaxBufferRecordsAndNilSnapshot(t *testing.T) {
	t.Parallel()

	var nilExporter *Exporter
	assert.Equal(t, 10000, nilExporter.maxBufferRecords())
	assert.Nil(t, nilExporter.PendingSnapshot())
}

func TestPgHealthAdapter_QueryErrors(t *testing.T) {
	t.Parallel()

	cfg, err := pgxpool.ParseConfig("postgres://127.0.0.1:1/strait?sslmode=disable")
	require.NoError(t, err)
	cfg.MaxConns = 1

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	adapter := NewPgHealthAdapter(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, _, err = adapter.CountJobs(ctx, "proj-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "count jobs")

	_, err = adapter.QueueDepth(ctx, "proj-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "queue depth")
}
