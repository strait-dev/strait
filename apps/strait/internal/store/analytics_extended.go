package store

import (
	"context"
	"time"
)

// Run Analytics types.

// RunTimelineBucket is a single time-series bucket for run status counts.
type RunTimelineBucket struct {
	Period    string `json:"period"`
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
	TimedOut  int    `json:"timed_out"`
	Total     int    `json:"total"`
}

// RunDurationBucket groups runs by duration range.
type RunDurationBucket struct {
	Range string  `json:"range"`
	Count int     `json:"count"`
	Pct   float64 `json:"pct"`
}

// RunFailureReason is an aggregated failure reason from run events.
type RunFailureReason struct {
	Message      string `json:"message"`
	Count        int    `json:"count"`
	LastSeen     string `json:"last_seen"`
	ExampleRunID string `json:"example_run_id"`
}

// RunSummary is an aggregate summary of run analytics.
type RunSummary struct {
	Total         int     `json:"total"`
	Completed     int     `json:"completed"`
	Failed        int     `json:"failed"`
	TimedOut      int     `json:"timed_out"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	P95DurationMs float64 `json:"p95_duration_ms"`
}

// RunsByTrigger groups run stats by trigger type.
type RunsByTrigger struct {
	TriggerType   string  `json:"trigger_type"`
	Total         int     `json:"total"`
	Completed     int     `json:"completed"`
	Failed        int     `json:"failed"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
}

// Job Analytics types.

// JobHistoryBucket is a single bucket in a job's history timeline.
type JobHistoryBucket struct {
	Period        string  `json:"period"`
	Completed     int     `json:"completed"`
	Failed        int     `json:"failed"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	P95DurationMs float64 `json:"p95_duration_ms"`
}

// JobComparison compares metrics across jobs.
type JobComparison struct {
	JobID         string  `json:"job_id"`
	Slug          string  `json:"slug"`
	Total         int     `json:"total"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	Cost          int64   `json:"cost"`
}

// JobReliability ranks a job by failure rate.
type JobReliability struct {
	JobID               string  `json:"job_id"`
	Slug                string  `json:"slug"`
	Total               int     `json:"total"`
	SuccessRate         float64 `json:"success_rate"`
	Failed              int     `json:"failed"`
	ConsecutiveFailures int     `json:"consecutive_failures"`
}

// RunsByVersion groups run stats by job version.
type RunsByVersion struct {
	VersionID     string  `json:"version_id"`
	Total         int     `json:"total"`
	Completed     int     `json:"completed"`
	Failed        int     `json:"failed"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
}

// JobCostRanking ranks jobs by total cost.
type JobCostRanking struct {
	JobID         string  `json:"job_id"`
	Slug          string  `json:"slug"`
	TotalCost     int64   `json:"total_cost"`
	RunCount      int     `json:"run_count"`
	AvgCostPerRun float64 `json:"avg_cost_per_run"`
}

// TopFailingJob lists jobs by failure count.
type TopFailingJob struct {
	JobID       string  `json:"job_id"`
	Slug        string  `json:"slug"`
	FailedCount int     `json:"failed_count"`
	Total       int     `json:"total"`
	FailureRate float64 `json:"failure_rate"`
}

// Tag Analytics types.

// TagSummary groups run stats by tag key/value.
type TagSummary struct {
	TagKey        string  `json:"tag_key"`
	TagValue      string  `json:"tag_value"`
	Total         int     `json:"total"`
	Completed     int     `json:"completed"`
	Failed        int     `json:"failed"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
}

// TopFailingTag ranks tags by failure rate.
type TopFailingTag struct {
	TagKey      string  `json:"tag_key"`
	TagValue    string  `json:"tag_value"`
	Failed      int     `json:"failed"`
	Total       int     `json:"total"`
	FailureRate float64 `json:"failure_rate"`
}

// TagCost groups cost by tag key/value.
type TagCost struct {
	TagKey    string `json:"tag_key"`
	TagValue  string `json:"tag_value"`
	TotalCost int64  `json:"total_cost"`
	RunCount  int    `json:"run_count"`
}

// Workflow Analytics types.

// StepDuration shows duration stats per workflow step.
type StepDuration struct {
	StepRef     string  `json:"step_ref"`
	AvgMs       float64 `json:"avg_ms"`
	P95Ms       float64 `json:"p95_ms"`
	Count       int     `json:"count"`
	FailureRate float64 `json:"failure_rate"`
}

// WorkflowCompletionBucket is a time-series bucket for workflow completion rates.
type WorkflowCompletionBucket struct {
	Period    string `json:"period"`
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
	TimedOut  int    `json:"timed_out"`
}

// WorkflowSummary is an aggregate summary of workflow analytics.
type WorkflowSummary struct {
	Total         int     `json:"total"`
	Completed     int     `json:"completed"`
	Failed        int     `json:"failed"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
}

// Webhook Analytics types.

// WebhookEndpointStats shows delivery stats per webhook URL.
type WebhookEndpointStats struct {
	URL          string  `json:"url"`
	Total        int     `json:"total"`
	Delivered    int     `json:"delivered"`
	Failed       int     `json:"failed"`
	Dead         int     `json:"dead"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
}

// WebhookHealthBucket is a time-series bucket per webhook URL.
type WebhookHealthBucket struct {
	URL          string  `json:"url"`
	Period       string  `json:"period"`
	SuccessRate  float64 `json:"success_rate"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

// TopFailingEndpoint ranks webhook endpoints by failure count.
type TopFailingEndpoint struct {
	URL         string  `json:"url"`
	Failed      int     `json:"failed"`
	Total       int     `json:"total"`
	FailureRate float64 `json:"failure_rate"`
	LastError   string  `json:"last_error"`
}

// Event Analytics types.

// EventVolumeBucket is a time-series bucket for event trigger counts.
type EventVolumeBucket struct {
	Period   string `json:"period"`
	Created  int    `json:"created"`
	Received int    `json:"received"`
	TimedOut int    `json:"timed_out"`
}

// EventLatencyStats aggregates event wait durations.
type EventLatencyStats struct {
	AvgMs float64 `json:"avg_ms"`
	P50Ms float64 `json:"p50_ms"`
	P95Ms float64 `json:"p95_ms"`
	P99Ms float64 `json:"p99_ms"`
	Count int     `json:"count"`
}

// Cost Analytics types (new endpoints).

// CostForecast projects future costs based on recent trends.
type CostForecast struct {
	DailyRate        float64 `json:"daily_rate"`
	ProjectedMonthly float64 `json:"projected_monthly"`
	TrendPct         float64 `json:"trend_pct"`
}

// CostByTrigger groups cost by trigger type.
type CostByTrigger struct {
	Trigger  string  `json:"trigger"`
	Cost     int64   `json:"cost"`
	RunCount int     `json:"run_count"`
	Pct      float64 `json:"pct"`
}

// CostByMachine groups cost by machine preset.
type CostByMachine struct {
	Preset       string  `json:"preset"`
	Cost         int64   `json:"cost"`
	DurationSecs float64 `json:"duration_secs"`
	RunCount     int     `json:"run_count"`
}

// Postgres fallback stubs.
// These return empty results since ClickHouse is the primary data source for new analytics.

func (q *Queries) GetRunTimeline(_ context.Context, _ string, _, _ time.Time, _ string) ([]RunTimelineBucket, error) {
	return []RunTimelineBucket{}, nil
}

func (q *Queries) GetRunDurationDistribution(_ context.Context, _ string, _, _ time.Time) ([]RunDurationBucket, error) {
	return []RunDurationBucket{}, nil
}

func (q *Queries) GetRunFailureReasons(_ context.Context, _ string, _, _ time.Time, _ int) ([]RunFailureReason, error) {
	return []RunFailureReason{}, nil
}

func (q *Queries) GetRunSummary(_ context.Context, _ string, _, _ time.Time) (*RunSummary, error) {
	return &RunSummary{}, nil
}

func (q *Queries) GetRunsByTrigger(_ context.Context, _ string, _, _ time.Time) ([]RunsByTrigger, error) {
	return []RunsByTrigger{}, nil
}

func (q *Queries) GetJobHistory(_ context.Context, _, _ string, _, _ time.Time, _ string) ([]JobHistoryBucket, error) {
	return []JobHistoryBucket{}, nil
}

func (q *Queries) GetJobComparison(_ context.Context, _ string, _ []string, _, _ time.Time) ([]JobComparison, error) {
	return []JobComparison{}, nil
}

func (q *Queries) GetJobReliability(_ context.Context, _ string, _, _ time.Time, _ int) ([]JobReliability, error) {
	return []JobReliability{}, nil
}

func (q *Queries) GetRunsByVersion(_ context.Context, _, _ string, _, _ time.Time) ([]RunsByVersion, error) {
	return []RunsByVersion{}, nil
}

func (q *Queries) GetJobCostRanking(_ context.Context, _ string, _, _ time.Time, _ int) ([]JobCostRanking, error) {
	return []JobCostRanking{}, nil
}

func (q *Queries) GetTopFailingJobs(_ context.Context, _ string, _, _ time.Time, _ int) ([]TopFailingJob, error) {
	return []TopFailingJob{}, nil
}

func (q *Queries) GetTagSummary(_ context.Context, _ string, _, _ time.Time, _ int) ([]TagSummary, error) {
	return []TagSummary{}, nil
}

func (q *Queries) GetTopFailingTags(_ context.Context, _ string, _, _ time.Time, _ int) ([]TopFailingTag, error) {
	return []TopFailingTag{}, nil
}

func (q *Queries) GetTagCost(_ context.Context, _ string, _, _ time.Time, _ int) ([]TagCost, error) {
	return []TagCost{}, nil
}

func (q *Queries) GetWorkflowStepDurations(_ context.Context, _, _ string, _, _ time.Time) ([]StepDuration, error) {
	return []StepDuration{}, nil
}

func (q *Queries) GetWorkflowCompletionRates(_ context.Context, _ string, _, _ time.Time, _ string) ([]WorkflowCompletionBucket, error) {
	return []WorkflowCompletionBucket{}, nil
}

func (q *Queries) GetWorkflowSummary(_ context.Context, _ string, _, _ time.Time) (*WorkflowSummary, error) {
	return &WorkflowSummary{}, nil
}

func (q *Queries) GetWebhookDeliveryStats(_ context.Context, _ string, _, _ time.Time) ([]WebhookEndpointStats, error) {
	return []WebhookEndpointStats{}, nil
}

func (q *Queries) GetWebhookEndpointHealth(_ context.Context, _ string, _, _ time.Time, _ string) ([]WebhookHealthBucket, error) {
	return []WebhookHealthBucket{}, nil
}

func (q *Queries) GetTopFailingWebhooks(_ context.Context, _ string, _, _ time.Time, _ int) ([]TopFailingEndpoint, error) {
	return []TopFailingEndpoint{}, nil
}

func (q *Queries) GetEventVolume(_ context.Context, _ string, _, _ time.Time, _ string) ([]EventVolumeBucket, error) {
	return []EventVolumeBucket{}, nil
}

func (q *Queries) GetEventLatency(_ context.Context, _ string, _, _ time.Time) (*EventLatencyStats, error) {
	return &EventLatencyStats{}, nil
}

func (q *Queries) GetCostForecast(_ context.Context, _ string, _, _ time.Time) (*CostForecast, error) {
	return &CostForecast{}, nil
}

func (q *Queries) GetCostByTrigger(_ context.Context, _ string, _, _ time.Time) ([]CostByTrigger, error) {
	return []CostByTrigger{}, nil
}

func (q *Queries) GetCostByMachine(_ context.Context, _ string, _, _ time.Time) ([]CostByMachine, error) {
	return []CostByMachine{}, nil
}
