package clickhouse

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/metric"
)

// ExporterConfig controls the async export worker behavior.
type ExporterConfig struct {
	BatchSize     int           // Max events per batch insert.
	FlushInterval time.Duration // Max time between flushes.
	Enabled       bool          // Feature gate.
}

// RunEventRecord maps to the run_events ClickHouse table.
type RunEventRecord struct {
	EventID   string
	RunID     string
	ProjectID string
	JobID     string
	EventType string
	Level     string
	Message   string
	Metadata  string
	CreatedAt time.Time
}

// RunAnalyticsRecord maps to the run_analytics ClickHouse table.
type RunAnalyticsRecord struct {
	RunID               string
	JobID               string
	ProjectID           string
	Status              string
	ExecutionMode       string
	MachinePreset       string
	Attempt             int
	DurationMs          uint64
	QueueWaitMs         uint64
	CostMicrousd        int64
	ComputeCostMicrousd int64
	TriggeredBy         string
	Tags                string // JSON-encoded tags map
	JobVersionID        string
	CreatedAt           time.Time
	StartedAt           *time.Time
	FinishedAt          *time.Time
}

// ComputeUsageRecord maps to the compute_usage ClickHouse table.
type ComputeUsageRecord struct {
	RunID         string
	ProjectID     string
	MachinePreset string
	MachineID     string
	DurationSecs  float64
	CostMicrousd  int64
	StartedAt     time.Time
	FinishedAt    time.Time
}

// RunUsageEventRecord maps to the run_usage_events ClickHouse table.
type RunUsageEventRecord struct {
	RunID            string
	JobID            string
	ProjectID        string
	Provider         string
	Model            string
	PromptTokens     uint32
	CompletionTokens uint32
	TotalTokens      uint32
	CostMicrousd     int64
	CreatedAt        time.Time
}

// WorkflowApprovalEventRecord maps to the workflow_approval_events ClickHouse table.
type WorkflowApprovalEventRecord struct {
	ApprovalID    string
	WorkflowRunID string
	StepRunID     string
	ProjectID     string
	Status        string
	RequestedAt   time.Time
	ApprovedAt    *time.Time
}

// JobMetadataRecord maps to the job_metadata ClickHouse table.
type JobMetadataRecord struct {
	JobID     string
	ProjectID string
	Slug      string
}

// EventTriggerEventRecord maps to the event_trigger_events ClickHouse table.
type EventTriggerEventRecord struct {
	TriggerID      string
	EventKey       string
	ProjectID      string
	SourceType     string
	Status         string
	TimeoutSecs    uint32
	WaitDurationMs uint64
	CreatedAt      time.Time
	ReceivedAt     *time.Time
}

// WorkflowRunAnalyticsRecord maps to the workflow_run_analytics ClickHouse table.
type WorkflowRunAnalyticsRecord struct {
	WorkflowRunID string
	WorkflowID    string
	ProjectID     string
	Status        string
	TriggeredBy   string
	StepCount     uint16
	DurationMs    uint64
	CreatedAt     time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
}

// WorkflowStepAnalyticsRecord maps to the workflow_step_analytics ClickHouse table.
type WorkflowStepAnalyticsRecord struct {
	StepRunID     string
	WorkflowRunID string
	WorkflowID    string
	ProjectID     string
	StepRef       string
	Status        string
	DurationMs    uint64
	Attempt       uint8
	Error         string
	CreatedAt     time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
}

// WebhookDeliveryEventRecord maps to the webhook_delivery_events ClickHouse table.
type WebhookDeliveryEventRecord struct {
	DeliveryID     string
	RunID          string
	JobID          string
	ProjectID      string
	WebhookURL     string
	Status         string
	Attempts       uint8
	LastStatusCode uint16
	DurationMs     uint64
	EventType      string
	CreatedAt      time.Time
	DeliveredAt    *time.Time
}

// BillingEventRecord maps to the billing_events ClickHouse table.
type BillingEventRecord struct {
	Timestamp time.Time
	OrgID     string
	ProjectID string
	EventType string // "gate_rejected", "spending_limit_hit", "plan_changed", "addon_purchased"
	Feature   string // e.g. "canary_deployments", "large-2x", "http_mode"
	PlanTier  string
	Details   string // JSON blob
}

// maxFlushRetries is the maximum number of consecutive flush failures before
// a batch is dropped to prevent unbounded growth.
const maxFlushRetries = 2

// ExporterMetrics holds optional OTel counters for exporter observability.
type ExporterMetrics struct {
	DroppedRecords metric.Int64Counter
	FlushFailures  metric.Int64Counter
}

// Exporter batches events and periodically flushes them to ClickHouse.
type Exporter struct {
	client  *Client
	config  ExporterConfig
	logger  *slog.Logger
	metrics *ExporterMetrics

	mu                  sync.Mutex
	pending             []any // buffered records
	consecutiveFailures int
	stopping            atomic.Bool
	stopOnce            sync.Once
	stopCh              chan struct{}
	done                chan struct{}
}

// NewExporter creates a new async exporter. Returns nil if client is nil or disabled.
func NewExporter(client *Client, config ExporterConfig, logger *slog.Logger) *Exporter {
	if client == nil || !config.Enabled {
		return nil
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 1000
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 5 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	// done is initialized as a closed channel so that Stop() without Start()
	// does not block forever waiting for a goroutine that was never launched.
	done := make(chan struct{})
	close(done)

	return &Exporter{
		client:  client,
		config:  config,
		logger:  logger,
		pending: make([]any, 0, config.BatchSize),
		stopCh:  make(chan struct{}),
		done:    done,
	}
}

// WithMetrics attaches optional OTel counters to the exporter.
func (e *Exporter) WithMetrics(m *ExporterMetrics) *Exporter {
	if e == nil {
		return nil
	}
	e.metrics = m
	return e
}

// Enqueue adds a record to the export buffer. Safe for concurrent use.
// Returns false if the exporter is stopping.
func (e *Exporter) Enqueue(record any) bool {
	if e == nil || e.stopping.Load() {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.pending = append(e.pending, record)

	// Backpressure: drop oldest if buffer exceeds 10x batch size.
	// Reallocate to release memory held by dropped elements.
	maxBuffer := e.config.BatchSize * 10
	if len(e.pending) > maxBuffer {
		dropped := len(e.pending) - maxBuffer
		kept := make([]any, maxBuffer)
		copy(kept, e.pending[dropped:])
		e.pending = kept
		e.logger.Warn("clickhouse exporter buffer overflow, dropped oldest records", "dropped", dropped)
	}
	return true
}

// Start begins the background flush loop.
func (e *Exporter) Start(ctx context.Context) {
	if e == nil {
		return
	}
	// Replace the pre-closed done channel with a fresh one so Stop() can
	// wait for the goroutine to finish.
	e.done = make(chan struct{})
	go func() { //nolint:gosec // ctx is intentionally captured for the flush loop lifetime.
		defer close(e.done)
		defer func() {
			if r := recover(); r != nil {
				slog.Error("clickhouse exporter panic recovered", "panic", r)
			}
		}()
		ticker := time.NewTicker(e.config.FlushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				e.flush(ctx)
			case <-e.stopCh:
				e.flush(ctx) // Final flush.
				return
			case <-ctx.Done():
				e.flush(context.Background()) // Use background ctx for final flush.
				return
			}
		}
	}()
}

// Stop signals the exporter to flush remaining records and shut down.
// Records enqueued after Stop() returns are silently dropped.
// Safe to call multiple times; subsequent calls are no-ops.
func (e *Exporter) Stop() {
	if e == nil {
		return
	}
	e.stopOnce.Do(func() {
		e.stopping.Store(true)
		close(e.stopCh)
	})
	<-e.done
}

func (e *Exporter) flush(ctx context.Context) {
	e.mu.Lock()
	if len(e.pending) == 0 {
		e.mu.Unlock()
		return
	}
	batch := e.pending
	e.pending = make([]any, 0, e.config.BatchSize)
	e.mu.Unlock()

	if err := e.insertBatch(ctx, batch); err != nil {
		e.logger.Error("clickhouse exporter flush failed", "count", len(batch), "error", err)
		if e.metrics != nil && e.metrics.FlushFailures != nil {
			e.metrics.FlushFailures.Add(ctx, 1)
		}
		e.mu.Lock()
		e.consecutiveFailures++
		if e.consecutiveFailures <= maxFlushRetries {
			maxBuffer := e.config.BatchSize * 10
			combined := append(batch, e.pending...) //nolint:gocritic // intentional prepend of failed batch
			if len(combined) > maxBuffer {
				// Keep the front (failed batch first) and drop newest overflow.
				combined = combined[:maxBuffer]
			}
			e.pending = combined
			e.logger.Warn("clickhouse requeued failed batch", "attempt", e.consecutiveFailures)
		} else {
			e.logger.Error("clickhouse dropping batch after max retries", "dropped", len(batch))
			if e.metrics != nil && e.metrics.DroppedRecords != nil {
				e.metrics.DroppedRecords.Add(ctx, int64(len(batch)))
			}
		}
		e.mu.Unlock()
		return
	}
	e.mu.Lock()
	e.consecutiveFailures = 0
	e.mu.Unlock()
}

// insertBatch writes a batch of records to ClickHouse, grouping by record type.
//
//nolint:gocyclo,cyclop
func (e *Exporter) insertBatch(ctx context.Context, batch []any) error {
	if len(batch) == 0 || e.client == nil {
		return nil
	}

	var events []RunEventRecord
	var analytics []RunAnalyticsRecord
	var usage []ComputeUsageRecord
	var runUsage []RunUsageEventRecord
	var approvals []WorkflowApprovalEventRecord
	var jobMeta []JobMetadataRecord
	var webhookDeliveries []WebhookDeliveryEventRecord
	var workflowRuns []WorkflowRunAnalyticsRecord
	var workflowSteps []WorkflowStepAnalyticsRecord
	var eventTriggers []EventTriggerEventRecord
	var billing []BillingEventRecord
	var agentAnalytics []AgentRunAnalyticsRecord

	for _, rec := range batch {
		switch r := rec.(type) {
		case RunEventRecord:
			events = append(events, r)
		case RunAnalyticsRecord:
			analytics = append(analytics, r)
		case ComputeUsageRecord:
			usage = append(usage, r)
		case RunUsageEventRecord:
			runUsage = append(runUsage, r)
		case WorkflowApprovalEventRecord:
			approvals = append(approvals, r)
		case JobMetadataRecord:
			jobMeta = append(jobMeta, r)
		case WebhookDeliveryEventRecord:
			webhookDeliveries = append(webhookDeliveries, r)
		case WorkflowRunAnalyticsRecord:
			workflowRuns = append(workflowRuns, r)
		case WorkflowStepAnalyticsRecord:
			workflowSteps = append(workflowSteps, r)
		case EventTriggerEventRecord:
			eventTriggers = append(eventTriggers, r)
		case BillingEventRecord:
			billing = append(billing, r)
		case AgentRunAnalyticsRecord:
			agentAnalytics = append(agentAnalytics, r)
		default:
			e.logger.Warn("clickhouse exporter: unknown record type", "type", fmt.Sprintf("%T", rec))
		}
	}

	var errs []error
	if len(events) > 0 {
		if err := e.insertRunEvents(ctx, events); err != nil {
			errs = append(errs, fmt.Errorf("run_events: %w", err))
		}
	}
	if len(analytics) > 0 {
		if err := e.insertRunAnalytics(ctx, analytics); err != nil {
			errs = append(errs, fmt.Errorf("run_analytics: %w", err))
		}
	}
	if len(usage) > 0 {
		if err := e.insertComputeUsage(ctx, usage); err != nil {
			errs = append(errs, fmt.Errorf("compute_usage: %w", err))
		}
	}
	if len(runUsage) > 0 {
		if err := e.insertRunUsageEvents(ctx, runUsage); err != nil {
			errs = append(errs, fmt.Errorf("run_usage_events: %w", err))
		}
	}
	if len(approvals) > 0 {
		if err := e.insertWorkflowApprovalEvents(ctx, approvals); err != nil {
			errs = append(errs, fmt.Errorf("workflow_approval_events: %w", err))
		}
	}
	if len(jobMeta) > 0 {
		if err := e.insertJobMetadata(ctx, jobMeta); err != nil {
			errs = append(errs, fmt.Errorf("job_metadata: %w", err))
		}
	}
	if len(webhookDeliveries) > 0 {
		if err := e.insertWebhookDeliveryEvents(ctx, webhookDeliveries); err != nil {
			errs = append(errs, fmt.Errorf("webhook_delivery_events: %w", err))
		}
	}
	if len(workflowRuns) > 0 {
		if err := e.insertWorkflowRunAnalytics(ctx, workflowRuns); err != nil {
			errs = append(errs, fmt.Errorf("workflow_run_analytics: %w", err))
		}
	}
	if len(workflowSteps) > 0 {
		if err := e.insertWorkflowStepAnalytics(ctx, workflowSteps); err != nil {
			errs = append(errs, fmt.Errorf("workflow_step_analytics: %w", err))
		}
	}
	if len(eventTriggers) > 0 {
		if err := e.insertEventTriggerEvents(ctx, eventTriggers); err != nil {
			errs = append(errs, fmt.Errorf("event_trigger_events: %w", err))
		}
	}
	if len(billing) > 0 {
		if err := e.insertBillingEvents(ctx, billing); err != nil {
			errs = append(errs, fmt.Errorf("billing_events: %w", err))
		}
	}
	if len(agentAnalytics) > 0 {
		if err := e.insertAgentRunAnalytics(ctx, agentAnalytics); err != nil {
			errs = append(errs, fmt.Errorf("agent_run_analytics: %w", err))
		}
	}

	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, err := range errs {
			msgs[i] = err.Error()
		}
		return fmt.Errorf("batch insert errors: %s", strings.Join(msgs, "; "))
	}

	e.logger.Debug("clickhouse exporter flushed batch",
		"events", len(events),
		"analytics", len(analytics),
		"usage", len(usage),
		"run_usage", len(runUsage),
		"approvals", len(approvals),
		"job_metadata", len(jobMeta),
		"webhook_deliveries", len(webhookDeliveries),
		"workflow_runs", len(workflowRuns),
		"workflow_steps", len(workflowSteps),
		"event_triggers", len(eventTriggers),
		"billing_events", len(billing),
		"agent_analytics", len(agentAnalytics),
	)
	return nil
}

func (e *Exporter) insertRunEvents(ctx context.Context, records []RunEventRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO run_events (event_id, run_id, project_id, job_id, event_type, level, message, metadata, created_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*9)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.EventID, r.RunID, r.ProjectID, r.JobID, r.EventType, r.Level, r.Message, r.Metadata, r.CreatedAt)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

func (e *Exporter) insertRunAnalytics(ctx context.Context, records []RunAnalyticsRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO run_analytics (run_id, job_id, project_id, status, execution_mode, machine_preset, attempt, duration_ms, queue_wait_ms, cost_microusd, compute_cost_microusd, triggered_by, tags, job_version_id, created_at, started_at, finished_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*17)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.RunID, r.JobID, r.ProjectID, r.Status, r.ExecutionMode, r.MachinePreset,
			r.Attempt, r.DurationMs, r.QueueWaitMs, r.CostMicrousd, r.ComputeCostMicrousd, r.TriggeredBy,
			r.Tags, r.JobVersionID, r.CreatedAt, r.StartedAt, r.FinishedAt)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

func (e *Exporter) insertComputeUsage(ctx context.Context, records []ComputeUsageRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO compute_usage (run_id, project_id, machine_preset, machine_id, duration_secs, cost_microusd, started_at, finished_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*8)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.RunID, r.ProjectID, r.MachinePreset, r.MachineID, r.DurationSecs, r.CostMicrousd, r.StartedAt, r.FinishedAt)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

func (e *Exporter) insertRunUsageEvents(ctx context.Context, records []RunUsageEventRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO run_usage_events (run_id, job_id, project_id, provider, model, prompt_tokens, completion_tokens, total_tokens, cost_microusd, created_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*10)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.RunID, r.JobID, r.ProjectID, r.Provider, r.Model,
			r.PromptTokens, r.CompletionTokens, r.TotalTokens, r.CostMicrousd, r.CreatedAt)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

func (e *Exporter) insertWorkflowApprovalEvents(ctx context.Context, records []WorkflowApprovalEventRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO workflow_approval_events (approval_id, workflow_run_id, step_run_id, project_id, status, requested_at, approved_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*7)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.ApprovalID, r.WorkflowRunID, r.StepRunID, r.ProjectID,
			r.Status, r.RequestedAt, r.ApprovedAt)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

func (e *Exporter) insertJobMetadata(ctx context.Context, records []JobMetadataRecord) error {
	const row = "(?, ?, ?)"
	query := "INSERT INTO job_metadata (job_id, project_id, slug) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*3)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.JobID, r.ProjectID, r.Slug)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

func (e *Exporter) insertEventTriggerEvents(ctx context.Context, records []EventTriggerEventRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO event_trigger_events (trigger_id, event_key, project_id, source_type, status, timeout_secs, wait_duration_ms, created_at, received_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*9)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.TriggerID, r.EventKey, r.ProjectID, r.SourceType,
			r.Status, r.TimeoutSecs, r.WaitDurationMs, r.CreatedAt, r.ReceivedAt)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

func (e *Exporter) insertWorkflowRunAnalytics(ctx context.Context, records []WorkflowRunAnalyticsRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO workflow_run_analytics (workflow_run_id, workflow_id, project_id, status, triggered_by, step_count, duration_ms, created_at, started_at, finished_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*10)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.WorkflowRunID, r.WorkflowID, r.ProjectID, r.Status, r.TriggeredBy,
			r.StepCount, r.DurationMs, r.CreatedAt, r.StartedAt, r.FinishedAt)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

func (e *Exporter) insertWorkflowStepAnalytics(ctx context.Context, records []WorkflowStepAnalyticsRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO workflow_step_analytics (step_run_id, workflow_run_id, workflow_id, project_id, step_ref, status, duration_ms, attempt, error, created_at, started_at, finished_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*12)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.StepRunID, r.WorkflowRunID, r.WorkflowID, r.ProjectID, r.StepRef,
			r.Status, r.DurationMs, r.Attempt, r.Error, r.CreatedAt, r.StartedAt, r.FinishedAt)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

func (e *Exporter) insertWebhookDeliveryEvents(ctx context.Context, records []WebhookDeliveryEventRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO webhook_delivery_events (delivery_id, run_id, job_id, project_id, webhook_url, status, attempts, last_status_code, duration_ms, event_type, created_at, delivered_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*12)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.DeliveryID, r.RunID, r.JobID, r.ProjectID, r.WebhookURL,
			r.Status, r.Attempts, r.LastStatusCode, r.DurationMs, r.EventType, r.CreatedAt, r.DeliveredAt)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

func (e *Exporter) insertBillingEvents(ctx context.Context, records []BillingEventRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO billing_events (timestamp, org_id, project_id, event_type, feature, plan_tier, details) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*7)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.Timestamp, r.OrgID, r.ProjectID, r.EventType, r.Feature, r.PlanTier, r.Details)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

// PendingCount returns the number of records waiting to be flushed.
func (e *Exporter) PendingCount() int {
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.pending)
}

func (e *Exporter) insertAgentRunAnalytics(ctx context.Context, records []AgentRunAnalyticsRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO agent_run_analytics (run_id, agent_slug, agent_id, project_id, status, duration_ms, model, provider, prompt_tokens, completion_tokens, total_tokens, cost_microusd, tool_call_count, checkpoint_count, error_class, created_at, started_at, finished_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*18)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.RunID, r.AgentSlug, r.AgentID, r.ProjectID, r.Status,
			r.DurationMs, r.Model, r.Provider, r.PromptTokens, r.CompletionTokens,
			r.TotalTokens, r.CostMicrousd, r.ToolCallCount, r.CheckpointCount,
			r.ErrorClass, r.CreatedAt, r.StartedAt, r.FinishedAt)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}
