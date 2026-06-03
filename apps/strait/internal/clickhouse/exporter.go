package clickhouse

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"strait/internal/httputil"

	"github.com/sourcegraph/conc"
	"go.opentelemetry.io/otel/metric"
)

// ExporterConfig controls the async export worker behavior.
type ExporterConfig struct {
	BatchSize      int           // Max events per batch insert.
	FlushInterval  time.Duration // Max time between flushes.
	MaxBufferBytes int           // Max approximate bytes buffered before dropping oldest records.
	Enabled        bool          // Feature gate.
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

const (
	// maxFlushRetries is the maximum number of consecutive flush failures before
	// a batch is dropped to prevent unbounded growth.
	maxFlushRetries = 2

	// defaultMaxBufferBytes caps queued ClickHouse export records by approximate
	// in-memory payload size. The record-count cap alone does not protect the
	// exporter from large event metadata or failure messages.
	defaultMaxBufferBytes = 16 << 20
)

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
	pendingBytes        int
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
	if config.MaxBufferBytes <= 0 {
		config.MaxBufferBytes = defaultMaxBufferBytes
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
// Returns false if the exporter is stopping or the record is too large to
// retain within the byte-size buffer cap.
func (e *Exporter) Enqueue(record any) bool {
	if e == nil || e.stopping.Load() {
		return false
	}
	recordBytes := estimateRecordBytes(record)
	maxBufferBytes := e.maxBufferBytes()
	if recordBytes > maxBufferBytes {
		e.logger.Warn("clickhouse exporter record exceeds byte buffer cap, dropped record", "bytes", recordBytes, "max_bytes", maxBufferBytes)
		if e.metrics != nil && e.metrics.DroppedRecords != nil {
			e.metrics.DroppedRecords.Add(context.Background(), 1)
		}
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.pending = append(e.pending, record)
	e.pendingBytes += recordBytes

	if dropped := e.trimPendingOldestLocked(); dropped > 0 {
		e.logger.Warn(
			"clickhouse exporter buffer overflow, dropped oldest records",
			"dropped", dropped,
			"pending_bytes", e.pendingBytes,
			"max_bytes", e.maxBufferBytes(),
		)
		if e.metrics != nil && e.metrics.DroppedRecords != nil {
			e.metrics.DroppedRecords.Add(context.Background(), int64(dropped))
		}
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
	var wg conc.WaitGroup
	wg.Go(func() {
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
	})
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
	batchBytes := e.pendingBytes
	e.pending = make([]any, 0, e.config.BatchSize)
	e.pendingBytes = 0
	e.mu.Unlock()

	if err := e.insertBatch(ctx, batch); err != nil {
		e.logger.Error("clickhouse exporter flush failed", "count", len(batch), "error", err)
		if e.metrics != nil && e.metrics.FlushFailures != nil {
			e.metrics.FlushFailures.Add(ctx, 1)
		}
		e.mu.Lock()
		e.consecutiveFailures++
		if e.consecutiveFailures <= maxFlushRetries {
			combined := append(batch, e.pending...) //nolint:gocritic // intentional prepend of failed batch
			e.pendingBytes += batchBytes
			e.pending = combined
			dropped := e.trimPendingNewestLocked()
			e.logger.Warn(
				"clickhouse requeued failed batch",
				"attempt", e.consecutiveFailures,
				"dropped", dropped,
				"pending_bytes", e.pendingBytes,
				"max_bytes", e.maxBufferBytes(),
			)
			if dropped > 0 && e.metrics != nil && e.metrics.DroppedRecords != nil {
				e.metrics.DroppedRecords.Add(ctx, int64(dropped))
			}
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

func (e *Exporter) maxBufferRecords() int {
	if e == nil || e.config.BatchSize <= 0 {
		return 1000 * 10
	}
	return e.config.BatchSize * 10
}

func (e *Exporter) maxBufferBytes() int {
	if e == nil || e.config.MaxBufferBytes <= 0 {
		return defaultMaxBufferBytes
	}
	return e.config.MaxBufferBytes
}

func (e *Exporter) trimPendingOldestLocked() int {
	return e.trimPendingLocked(true)
}

func (e *Exporter) trimPendingNewestLocked() int {
	return e.trimPendingLocked(false)
}

func (e *Exporter) trimPendingLocked(dropOldest bool) int {
	maxRecords := e.maxBufferRecords()
	maxBytes := e.maxBufferBytes()
	dropped := 0
	for len(e.pending) > 0 && (len(e.pending) > maxRecords || e.pendingBytes > maxBytes) {
		dropIndex := len(e.pending) - 1
		if dropOldest {
			dropIndex = 0
		}
		e.pendingBytes -= estimateRecordBytes(e.pending[dropIndex])
		if e.pendingBytes < 0 {
			e.pendingBytes = 0
		}
		e.pending = append(e.pending[:dropIndex], e.pending[dropIndex+1:]...)
		dropped++
	}
	if dropped > 0 {
		kept := make([]any, len(e.pending))
		copy(kept, e.pending)
		e.pending = kept
	}
	return dropped
}

func estimateRecordsBytes(records []any) int {
	total := 0
	for _, record := range records {
		total += estimateRecordBytes(record)
	}
	return total
}

func estimateRecordBytes(record any) int {
	return estimateRecordValueBytes(reflect.ValueOf(record)) + 64
}

func estimateRecordValueBytes(v reflect.Value) int {
	if !v.IsValid() {
		return 0
	}
	if v.Type() == reflect.TypeFor[time.Time]() {
		return 24
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return 8
		}
		return 8 + estimateRecordValueBytes(v.Elem())
	case reflect.String:
		return v.Len()
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return 8
	case reflect.Struct:
		size := 0
		for _, field := range v.Fields() {
			size += estimateRecordValueBytes(field)
		}
		return size
	case reflect.Slice, reflect.Array:
		size := 0
		for i := range v.Len() {
			size += estimateRecordValueBytes(v.Index(i))
		}
		return size
	default:
		return 16
	}
}

// insertBatch writes a batch of records to ClickHouse, grouped by record type.
func (e *Exporter) insertBatch(ctx context.Context, batch []any) error {
	if len(batch) == 0 || e.client == nil {
		return nil
	}

	records := exportRecordBatch{}
	for _, rec := range batch {
		records.add(e.logger, rec)
	}

	if err := records.insert(ctx, e); err != nil {
		return err
	}
	records.logFlush(e.logger)
	return nil
}

type exportRecordBatch struct {
	events            []RunEventRecord
	analytics         []RunAnalyticsRecord
	approvals         []WorkflowApprovalEventRecord
	jobMeta           []JobMetadataRecord
	webhookDeliveries []WebhookDeliveryEventRecord
	workflowRuns      []WorkflowRunAnalyticsRecord
	workflowSteps     []WorkflowStepAnalyticsRecord
	eventTriggers     []EventTriggerEventRecord
	billing           []BillingEventRecord
}

func (b *exportRecordBatch) add(logger *slog.Logger, rec any) {
	switch r := rec.(type) {
	case RunEventRecord:
		b.events = append(b.events, sanitizeRunEventRecord(r))
	case RunAnalyticsRecord:
		b.analytics = append(b.analytics, r)
	case WorkflowApprovalEventRecord:
		b.approvals = append(b.approvals, r)
	case JobMetadataRecord:
		b.jobMeta = append(b.jobMeta, r)
	case WebhookDeliveryEventRecord:
		b.webhookDeliveries = append(b.webhookDeliveries, sanitizeWebhookDeliveryEventRecord(r))
	case WorkflowRunAnalyticsRecord:
		b.workflowRuns = append(b.workflowRuns, r)
	case WorkflowStepAnalyticsRecord:
		b.workflowSteps = append(b.workflowSteps, r)
	case EventTriggerEventRecord:
		b.eventTriggers = append(b.eventTriggers, r)
	case BillingEventRecord:
		b.billing = append(b.billing, r)
	default:
		logger.Warn("clickhouse exporter: unknown record type", "type", fmt.Sprintf("%T", rec))
	}
}

func (b *exportRecordBatch) insert(ctx context.Context, e *Exporter) error {
	var errs []error
	errs = appendBatchInsertError(ctx, errs, "run_events", b.events, e.insertRunEvents)
	errs = appendBatchInsertError(ctx, errs, "run_analytics", b.analytics, e.insertRunAnalytics)
	errs = appendBatchInsertError(ctx, errs, "workflow_approval_events", b.approvals, e.insertWorkflowApprovalEvents)
	errs = appendBatchInsertError(ctx, errs, "job_metadata", b.jobMeta, e.insertJobMetadata)
	errs = appendBatchInsertError(ctx, errs, "webhook_delivery_events", b.webhookDeliveries, e.insertWebhookDeliveryEvents)
	errs = appendBatchInsertError(ctx, errs, "workflow_run_analytics", b.workflowRuns, e.insertWorkflowRunAnalytics)
	errs = appendBatchInsertError(ctx, errs, "workflow_step_analytics", b.workflowSteps, e.insertWorkflowStepAnalytics)
	errs = appendBatchInsertError(ctx, errs, "event_trigger_events", b.eventTriggers, e.insertEventTriggerEvents)
	errs = appendBatchInsertError(ctx, errs, "billing_events", b.billing, e.insertBillingEvents)

	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, err := range errs {
			msgs[i] = err.Error()
		}
		return fmt.Errorf("batch insert errors: %s", strings.Join(msgs, "; "))
	}
	return nil
}

func appendBatchInsertError[T any](
	ctx context.Context,
	errs []error,
	table string,
	records []T,
	insert func(context.Context, []T) error,
) []error {
	if len(records) == 0 {
		return errs
	}
	if err := insert(ctx, records); err != nil {
		return append(errs, fmt.Errorf("%s: %w", table, err))
	}
	return errs
}

func (b *exportRecordBatch) logFlush(logger *slog.Logger) {
	logger.Debug("clickhouse exporter flushed batch",
		"events", len(b.events),
		"analytics", len(b.analytics),
		"approvals", len(b.approvals),
		"job_metadata", len(b.jobMeta),
		"webhook_deliveries", len(b.webhookDeliveries),
		"workflow_runs", len(b.workflowRuns),
		"workflow_steps", len(b.workflowSteps),
		"event_triggers", len(b.eventTriggers),
		"billing_events", len(b.billing),
	)
}

func (e *Exporter) insertRunEvents(ctx context.Context, records []RunEventRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO run_events (event_id, run_id, project_id, job_id, event_type, level, message_class, metadata_redacted, created_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*9)

	for i, r := range records {
		r = sanitizeRunEventRecord(r)
		placeholders[i] = row
		args = append(args, r.EventID, r.RunID, r.ProjectID, r.JobID, r.EventType, r.Level, r.Message, r.Metadata, r.CreatedAt)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

func (e *Exporter) insertRunAnalytics(ctx context.Context, records []RunAnalyticsRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO run_analytics " +
		"(run_id, job_id, project_id, status, execution_mode, attempt, duration_ms, queue_wait_ms, " +
		"cost_microusd, compute_cost_microusd, triggered_by, tags, job_version_id, created_at, started_at, finished_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*16)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.RunID, r.JobID, r.ProjectID, r.Status, r.ExecutionMode,
			r.Attempt, r.DurationMs, r.QueueWaitMs, r.CostMicrousd, r.ComputeCostMicrousd, r.TriggeredBy,
			r.Tags, r.JobVersionID, r.CreatedAt, r.StartedAt, r.FinishedAt)
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
	query := "INSERT INTO event_trigger_events " +
		"(trigger_id, event_key, project_id, source_type, status, timeout_secs, wait_duration_ms, created_at, received_at) VALUES "
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
	query := "INSERT INTO workflow_run_analytics " +
		"(workflow_run_id, workflow_id, project_id, status, triggered_by, step_count, duration_ms, created_at, started_at, finished_at) VALUES "
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
	query := "INSERT INTO workflow_step_analytics " +
		"(step_run_id, workflow_run_id, workflow_id, project_id, step_ref, status, duration_ms, attempt, error, created_at, started_at, finished_at) VALUES "
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
	query := "INSERT INTO webhook_delivery_events " +
		"(delivery_id, run_id, job_id, project_id, webhook_host, status, attempts, last_status_code, duration_ms, event_type, created_at, delivered_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*12)

	for i, r := range records {
		r = sanitizeWebhookDeliveryEventRecord(r)
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

func sanitizeRunEventRecord(r RunEventRecord) RunEventRecord {
	r.Message = safeRunEventAnalyticsMessage(r)
	if strings.TrimSpace(r.Metadata) != "" {
		r.Metadata = "{}"
	}
	return r
}

func safeRunEventAnalyticsMessage(r RunEventRecord) string {
	if strings.EqualFold(strings.TrimSpace(r.Level), "error") {
		return safeRunFailureReason(r.Message)
	}
	switch normalizedAnalyticsLabel(r.EventType) {
	case "run_started", "run_completed", "run_failed", "run_timed_out", "run_canceled", "run_retrying", "run_snoozed":
		return normalizedAnalyticsLabel(r.EventType)
	}
	switch normalizedAnalyticsLabel(r.Level) {
	case "debug", "info", "warn", "warning", "error":
		return "level_" + normalizedAnalyticsLabel(r.Level)
	default:
		return "run_event"
	}
}

func normalizedAnalyticsLabel(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	if label == "" || len(label) > 64 {
		return ""
	}
	for _, r := range label {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return ""
	}
	return label
}

func sanitizeWebhookDeliveryEventRecord(r WebhookDeliveryEventRecord) WebhookDeliveryEventRecord {
	r.WebhookURL = httputil.RedactURLForLog(r.WebhookURL)
	return r
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

// PendingSnapshot returns a copy of queued records for diagnostics and tests.
func (e *Exporter) PendingSnapshot() []any {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]any, len(e.pending))
	copy(out, e.pending)
	return out
}
