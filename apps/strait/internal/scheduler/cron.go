package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/telemetry"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// CronStore is the subset of store operations needed by CronScheduler.
type CronStore interface {
	ListCronJobs(ctx context.Context) ([]domain.Job, error)
	ListCronWorkflows(ctx context.Context) ([]domain.Workflow, error)
	CountRunningWorkflowRuns(ctx context.Context, workflowID string) (int, error)
	CountActiveRunsForJob(ctx context.Context, jobID string) (int, error)
	CancelActiveRunsForJob(ctx context.Context, jobID string, reason string) ([]store.CanceledRun, error)
	CancelChildRunsByParentIDs(ctx context.Context, parentIDs []string, finishedAt time.Time, reason string) (int64, error)
	TryAcquireCronFire(ctx context.Context, projectID, key string, ttl time.Duration) (bool, error)
}

type cronCancelExceptStore interface {
	CancelActiveRunsForJobExcept(ctx context.Context, jobID string, excludeRunID string, reason string) ([]store.CanceledRun, error)
}

type CronAdmissionStore interface {
	GetProjectQuota(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	CountProjectQueuedRuns(ctx context.Context, projectID string) (int, error)
	CountProjectActiveRuns(ctx context.Context, projectID string) (int, error)
	CountRunsForJobSince(ctx context.Context, jobID string, since time.Time) (int, error)
	SumProjectDailyCostMicrousd(ctx context.Context, projectID string, timezone string) (int64, error)
}

type cronAdmissionTransactioner interface {
	WithTx(ctx context.Context, fn func(context.Context, store.DBTX) error) error
}

var (
	errCronProjectQueuedQuotaExceeded    = errors.New("cron project queued quota exceeded")
	errCronProjectExecutingQuotaExceeded = errors.New("cron project executing quota exceeded")
	errCronProjectDailyCostQuotaExceeded = errors.New("cron project daily cost quota exceeded")
	errCronJobRateLimitExceeded          = errors.New("cron job rate limit exceeded")
)

const cronFireDedupeTTL = 25 * time.Hour

type WorkflowTrigger interface {
	TriggerWorkflow(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, stepOverrides []domain.StepOverride, extraTags map[string]string) (*domain.WorkflowRun, error)
}

type CronScheduler struct {
	ctx               context.Context
	cron              *cron.Cron
	store             CronStore
	queue             queue.Queue
	workflowTrigger   WorkflowTrigger
	workflowCallback  WorkflowCallback
	metrics           *telemetry.Metrics
	defaultRunTTLSecs int
	driftMu           sync.RWMutex
	driftSchedules    map[string]cron.Schedule
	scheduleMu        sync.Mutex
	entryIDs          []cron.EntryID
}

// NewCronScheduler creates a new cron-based job and workflow scheduler.
func NewCronScheduler(ctx context.Context, s CronStore, q queue.Queue, workflowTrigger WorkflowTrigger) *CronScheduler {
	return &CronScheduler{
		ctx:             ctx,
		cron:            cron.New(),
		store:           s,
		queue:           q,
		workflowTrigger: workflowTrigger,
		driftSchedules:  make(map[string]cron.Schedule),
	}
}

func (cs *CronScheduler) WithMetrics(m *telemetry.Metrics) *CronScheduler {
	cs.metrics = m
	return cs
}

func (cs *CronScheduler) WithDefaultRunTTLSecs(ttl int) *CronScheduler {
	cs.defaultRunTTLSecs = ttl
	return cs
}

func (cs *CronScheduler) WithWorkflowCallback(wc WorkflowCallback) *CronScheduler {
	cs.workflowCallback = wc
	return cs
}

func (cs *CronScheduler) LoadJobs(ctx context.Context) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "cron.LoadJobs")
	defer span.End()

	jobs, err := cs.store.ListCronJobs(ctx)
	if err != nil {
		return fmt.Errorf("list cron jobs: %w", err)
	}
	workflows, err := cs.store.ListCronWorkflows(ctx)
	if err != nil {
		return fmt.Errorf("list cron workflows: %w", err)
	}

	cs.scheduleMu.Lock()
	defer cs.scheduleMu.Unlock()
	for _, id := range cs.entryIDs {
		cs.cron.Remove(id)
	}
	cs.entryIDs = cs.entryIDs[:0]

	for _, j := range jobs {
		job := j
		expr, tzErr := cronExpressionWithValidatedTimezone(job.Cron, job.Timezone)
		if tzErr != nil {
			slog.Warn("skipping cron job with invalid timezone",
				"job_id", job.ID,
				"project_id", job.ProjectID,
				"timezone", job.Timezone,
				"error", tzErr,
			)
			continue
		}
		id, err := cs.cron.AddFunc(expr, func() {
			cs.triggerJob(cs.ctx, job)
		})
		if err != nil {
			return fmt.Errorf("register cron job %s: %w", job.ID, err)
		}
		cs.entryIDs = append(cs.entryIDs, id)
		cs.cacheDriftSchedule(job.Cron)
	}

	if cs.workflowTrigger != nil {
		for _, wf := range workflows {
			workflow := wf
			expr, tzErr := cronExpressionWithValidatedTimezone(workflow.Cron, workflow.CronTimezone)
			if tzErr != nil {
				slog.Warn("skipping cron workflow with invalid timezone",
					"workflow_id", workflow.ID,
					"project_id", workflow.ProjectID,
					"timezone", workflow.CronTimezone,
					"error", tzErr,
				)
				continue
			}
			id, err := cs.cron.AddFunc(expr, func() {
				cs.triggerWorkflow(cs.ctx, workflow)
			})
			if err != nil {
				return fmt.Errorf("register cron workflow %s: %w", workflow.ID, err)
			}
			cs.entryIDs = append(cs.entryIDs, id)
			cs.cacheDriftSchedule(workflow.Cron)
		}
	}

	slog.Info("cron schedules loaded", "jobs", len(jobs), "workflows", len(workflows))
	return nil
}

func (cs *CronScheduler) triggerJob(ctx context.Context, job domain.Job) {
	cs.withCronFireLock(ctx, job.ProjectID, "job", job.ID, func(lockCtx context.Context, fireKey string) {
		cs.triggerJobLocked(lockCtx, job, fireKey)
	})
}

func (cs *CronScheduler) triggerJobLocked(ctx context.Context, job domain.Job, fireKey string) {
	ctx, span := otel.Tracer("strait").Start(ctx, "cron.TriggerJob")
	defer span.End()

	cs.recordCronDrift(ctx, job.Cron)

	run := domain.JobRun{
		ID:             uuid.Must(uuid.NewV7()).String(),
		JobID:          job.ID,
		ProjectID:      job.ProjectID,
		Tags:           job.Tags,
		TriggeredBy:    domain.TriggerCron,
		JobVersion:     job.Version,
		JobVersionID:   job.VersionID,
		CreatedBy:      "system:cron",
		ExecutionMode:  job.ExecutionMode,
		QueueName:      job.Queue,
		IdempotencyKey: fireKey,
	}

	if job.RunTTLSecs > 0 {
		exp := time.Now().Add(time.Duration(job.RunTTLSecs) * time.Second)
		run.ExpiresAt = &exp
	} else if cs.defaultRunTTLSecs > 0 {
		exp := time.Now().Add(time.Duration(cs.defaultRunTTLSecs) * time.Second)
		run.ExpiresAt = &exp
	}

	switch job.CronOverlapPolicy {
	case domain.OverlapPolicySkip:
		active, err := cs.store.CountActiveRunsForJob(ctx, job.ID)
		if err != nil {
			slog.Error("failed to count active runs for job", "job_id", job.ID, "error", err)
			if cs.metrics != nil {
				cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			}
			return
		}
		if active > 0 {
			slog.Info("skipping cron trigger: job has active runs",
				"job_id", job.ID, "active", active, "policy", "skip")
			if cs.metrics != nil {
				cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "skipped")))
			}
			return
		}

	case domain.OverlapPolicyAllow:
		// Default: always enqueue.

	default:
		// Treat unknown/empty as allow for forward compatibility.
	}

	err := cs.withCronAdmissionGuard(ctx, &job, func(enqueueCtx context.Context, tx store.DBTX) error {
		if tx != nil && cs.queue != nil {
			return cs.queue.EnqueueInTx(enqueueCtx, tx, &run)
		}
		return queue.EnqueueWithRetry(enqueueCtx, cs.queue, &run, queue.DefaultInternalEnqueueRetryConfig())
	})
	if err != nil {
		if errors.Is(err, domain.ErrIdempotencyConflict) {
			if cs.metrics != nil {
				cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "skipped")))
			}
			slog.Info("skipping duplicate cron fire", "job_id", job.ID, "project_id", job.ProjectID, "fire_key", fireKey)
			return
		}
		if isCronAdmissionLimitError(err) {
			if cs.metrics != nil {
				cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "skipped")))
			}
			slog.Info("skipping cron trigger: admission limit exceeded", "job_id", job.ID, "project_id", job.ProjectID, "error", err)
			return
		}
		slog.Error("failed to enqueue cron run", "job_id", job.ID, "project_id", job.ProjectID, "error", err)
		if cs.metrics != nil {
			cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		}
		return
	}

	if job.CronOverlapPolicy == domain.OverlapPolicyCancelRunning {
		canceledRuns, cancelErr := cs.cancelActiveRunsForReplacement(ctx, job.ID, run.ID,
			"canceled by cron overlap policy: cancel_running")
		if cancelErr != nil {
			slog.Error("failed to cancel active runs after cron enqueue", "job_id", job.ID, "error", cancelErr)
			if cs.metrics != nil {
				cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			}
			return
		}
		if len(canceledRuns) > 0 {
			slog.Info("canceled active runs after cron enqueue",
				"job_id", job.ID, "canceled", len(canceledRuns), "policy", "cancel_running")
			cs.processCanceledRuns(ctx, job.ID, canceledRuns)
		}
	}

	if cs.metrics != nil {
		cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "success")))
	}

	slog.Info("cron run enqueued", "job_id", job.ID, "project_id", job.ProjectID, "run_id", run.ID)
}

func (cs *CronScheduler) cancelActiveRunsForReplacement(ctx context.Context, jobID string, replacementRunID string, reason string) ([]store.CanceledRun, error) {
	if s, ok := cs.store.(cronCancelExceptStore); ok {
		return s.CancelActiveRunsForJobExcept(ctx, jobID, replacementRunID, reason)
	}
	return cs.store.CancelActiveRunsForJob(ctx, jobID, reason)
}

func (cs *CronScheduler) withCronAdmissionGuard(ctx context.Context, job *domain.Job, fn func(context.Context, store.DBTX) error) error {
	limits, ok := cs.store.(CronAdmissionStore)
	if !ok {
		return fn(ctx, nil)
	}

	if txer, ok := cs.store.(cronAdmissionTransactioner); ok {
		return txer.WithTx(ctx, func(txCtx context.Context, tx store.DBTX) error {
			if _, err := tx.Exec(txCtx, "SELECT pg_advisory_xact_lock($1)", cronAdmissionLockID(job.ProjectID)); err != nil {
				return fmt.Errorf("acquire cron admission lock: %w", err)
			}
			if err := checkCronAdmissionLimits(txCtx, limits, job); err != nil {
				return err
			}
			return fn(txCtx, tx)
		})
	}

	if err := checkCronAdmissionLimits(ctx, limits, job); err != nil {
		return err
	}
	return fn(ctx, nil)
}

func checkCronAdmissionLimits(ctx context.Context, limits CronAdmissionStore, job *domain.Job) error {
	quota, err := limits.GetProjectQuota(ctx, job.ProjectID)
	if err != nil {
		return fmt.Errorf("get cron project quota: %w", err)
	}
	if err := checkCronProjectQuota(ctx, limits, job.ProjectID, quota); err != nil {
		return err
	}
	return checkCronJobRateLimit(ctx, limits, job)
}

func checkCronProjectQuota(
	ctx context.Context,
	limits CronAdmissionStore,
	projectID string,
	quota *store.ProjectQuota,
) error {
	if quota == nil {
		return nil
	}
	if err := checkCronProjectQueuedQuota(ctx, limits, projectID, quota.MaxQueuedRuns); err != nil {
		return err
	}
	if err := checkCronProjectExecutingQuota(ctx, limits, projectID, quota.MaxExecutingRuns); err != nil {
		return err
	}
	return checkCronProjectDailyCostQuota(ctx, limits, projectID, quota)
}

func checkCronProjectQueuedQuota(ctx context.Context, limits CronAdmissionStore, projectID string, limit int) error {
	if limit <= 0 {
		return nil
	}
	queuedRuns, err := limits.CountProjectQueuedRuns(ctx, projectID)
	if err != nil {
		return fmt.Errorf("evaluate cron project queued quota: %w", err)
	}
	if queuedRuns >= limit {
		return errCronProjectQueuedQuotaExceeded
	}
	return nil
}

func checkCronProjectExecutingQuota(ctx context.Context, limits CronAdmissionStore, projectID string, limit int) error {
	if limit <= 0 {
		return nil
	}
	activeRuns, err := limits.CountProjectActiveRuns(ctx, projectID)
	if err != nil {
		return fmt.Errorf("evaluate cron project active quota: %w", err)
	}
	if activeRuns >= limit {
		return errCronProjectExecutingQuotaExceeded
	}
	return nil
}

func checkCronProjectDailyCostQuota(
	ctx context.Context,
	limits CronAdmissionStore,
	projectID string,
	quota *store.ProjectQuota,
) error {
	if quota.MaxDailyCostMicrousd <= 0 {
		return nil
	}
	tz := quota.Timezone
	if tz == "" {
		tz = "UTC"
	}
	dailyCost, err := limits.SumProjectDailyCostMicrousd(ctx, projectID, tz)
	if err != nil {
		return fmt.Errorf("evaluate cron project daily cost quota: %w", err)
	}
	if dailyCost >= quota.MaxDailyCostMicrousd {
		return errCronProjectDailyCostQuotaExceeded
	}
	return nil
}

func checkCronJobRateLimit(ctx context.Context, limits CronAdmissionStore, job *domain.Job) error {
	if job.RateLimitMax > 0 && job.RateLimitWindowSecs > 0 {
		since := time.Now().Add(-time.Duration(job.RateLimitWindowSecs) * time.Second)
		runCount, countErr := limits.CountRunsForJobSince(ctx, job.ID, since)
		if countErr != nil {
			return fmt.Errorf("evaluate cron job rate limit: %w", countErr)
		}
		if runCount >= job.RateLimitMax {
			return errCronJobRateLimitExceeded
		}
	}
	return nil
}

func isCronAdmissionLimitError(err error) bool {
	return errors.Is(err, errCronProjectQueuedQuotaExceeded) ||
		errors.Is(err, errCronProjectExecutingQuotaExceeded) ||
		errors.Is(err, errCronProjectDailyCostQuotaExceeded) ||
		errors.Is(err, errCronJobRateLimitExceeded)
}

func (cs *CronScheduler) triggerWorkflow(ctx context.Context, workflow domain.Workflow) {
	cs.withCronFireLock(ctx, workflow.ProjectID, "workflow", workflow.ID, func(lockCtx context.Context, fireKey string) {
		cs.triggerWorkflowLocked(lockCtx, workflow, fireKey)
	})
}

func (cs *CronScheduler) triggerWorkflowLocked(ctx context.Context, workflow domain.Workflow, fireKey string) {
	ctx, span := otel.Tracer("strait").Start(ctx, "cron.TriggerWorkflow")
	defer span.End()

	cs.recordCronDrift(ctx, workflow.Cron)

	if workflow.SkipIfRunning {
		running, err := cs.store.CountRunningWorkflowRuns(ctx, workflow.ID)
		if err != nil {
			slog.Error("failed to count running workflow runs", "workflow_id", workflow.ID, "error", err)
			if cs.metrics != nil {
				cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			}
			return
		}
		if running > 0 {
			slog.Info("skipping cron workflow trigger because run is active", "workflow_id", workflow.ID, "running", running)
			if cs.metrics != nil {
				cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "skipped")))
			}
			return
		}
	}

	if _, err := cs.workflowTrigger.TriggerWorkflow(ctx, workflow.ID, workflow.ProjectID, nil, domain.TriggerCron, nil, map[string]string{"cron_fire_key": fireKey}); err != nil {
		slog.Error("failed to trigger cron workflow", "workflow_id", workflow.ID, "project_id", workflow.ProjectID, "error", err)
		if cs.metrics != nil {
			cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		}
		return
	}
	if cs.metrics != nil {
		cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "success")))
	}

	slog.Info("cron workflow triggered", "workflow_id", workflow.ID, "project_id", workflow.ProjectID)
}

// processCanceledRuns handles side effects for runs canceled by the
// cancel_running overlap policy: cancels child runs and notifies the workflow engine.
func (cs *CronScheduler) processCanceledRuns(ctx context.Context, jobID string, runs []store.CanceledRun) {
	parentIDs := make([]string, len(runs))
	for i, cr := range runs {
		parentIDs[i] = cr.ID
	}
	if _, err := cs.store.CancelChildRunsByParentIDs(ctx, parentIDs, time.Now(),
		"parent canceled by cron overlap policy"); err != nil {
		slog.Error("failed to cancel child runs", "job_id", jobID, "error", err)
	}

	if cs.workflowCallback != nil {
		for _, cr := range runs {
			canceledRun := &domain.JobRun{
				ID:                cr.ID,
				JobID:             cr.JobID,
				ProjectID:         cr.ProjectID,
				WorkflowStepRunID: cr.WorkflowStepRunID,
				Status:            domain.StatusCanceled,
				Error:             "canceled by cron overlap policy: cancel_running",
				ExecutionMode:     cr.ExecutionMode,
			}
			if cbErr := cs.workflowCallback.OnJobRunTerminal(ctx, canceledRun); cbErr != nil {
				slog.Error("workflow callback failed on cron cancel", "run_id", cr.ID, "error", cbErr)
			}
		}
	}
}

func cronExpressionWithTimezone(expr, timezone string) string {
	if timezone == "" {
		return expr
	}
	return fmt.Sprintf("CRON_TZ=%s %s", timezone, expr)
}

func cronExpressionWithValidatedTimezone(expr, timezone string) (string, error) {
	if timezone == "" {
		return expr, nil
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return "", fmt.Errorf("invalid cron timezone %q: %w", timezone, err)
	}
	return cronExpressionWithTimezone(expr, timezone), nil
}

func (cs *CronScheduler) withCronFireLock(ctx context.Context, projectID, kind, id string, fn func(context.Context, string)) {
	fireKey := cronFireKey(kind, id, time.Now().UTC())
	acquiredFire, err := cs.store.TryAcquireCronFire(ctx, projectID, fireKey, cronFireDedupeTTL)
	if err != nil {
		slog.Warn("cron fire idempotency claim failed", "kind", kind, "id", id, "project_id", projectID, "fire_key", fireKey, "error", err)
		return
	}
	if !acquiredFire {
		slog.Info("cron fire skipped: durable fire key already claimed", "kind", kind, "id", id, "project_id", projectID, "fire_key", fireKey)
		return
	}

	locker, ok := cs.store.(AdvisoryLocker)
	if !ok {
		fn(ctx, fireKey)
		return
	}

	acquired, err := runWithOptionalAdvisoryLock(ctx, locker, cronFireLockID(fireKey), func(lockCtx context.Context) error {
		fn(lockCtx, fireKey)
		return nil
	})
	if err != nil {
		slog.Warn("cron fire advisory lock failed", "kind", kind, "id", id, "error", err)
		return
	}
	if !acquired {
		slog.Info("cron fire skipped: another scheduler owns fire", "kind", kind, "id", id)
	}
}

func cronFireKey(kind, id string, now time.Time) string {
	const prefix = "cron:"
	const maxInt64Digits = 20
	size := len(prefix) + len(kind) + 1 + len(id) + 1 + maxInt64Digits
	if size <= 96 {
		var buf [96]byte
		out := append(buf[:0], prefix...)
		out = append(out, kind...)
		out = append(out, ':')
		out = append(out, id...)
		out = append(out, ':')
		out = strconv.AppendInt(out, cronFireUnixMinute(now), 10)
		return string(out)
	}
	out := make([]byte, 0, size)
	out = append(out, prefix...)
	out = append(out, kind...)
	out = append(out, ':')
	out = append(out, id...)
	out = append(out, ':')
	out = strconv.AppendInt(out, cronFireUnixMinute(now), 10)
	return string(out)
}

func cronFireUnixMinute(now time.Time) int64 {
	unix := now.Unix()
	return unix - unix%60
}

func cronFireLockID(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64() >> 1)
}

func cronAdmissionLockID(projectID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("trigger-limit:"))
	_, _ = h.Write([]byte(projectID))
	return int64(h.Sum64() >> 1)
}

func (cs *CronScheduler) Start() {
	cs.cron.Start()
}

func (cs *CronScheduler) Stop() context.Context {
	return cs.cron.Stop()
}

// recordCronDrift calculates the delta between the expected cron fire time
// and the actual fire time, and records it as a histogram metric.
func (cs *CronScheduler) recordCronDrift(ctx context.Context, cronExpr string) {
	if cs.metrics == nil || cronExpr == "" {
		return
	}
	schedule := cs.getDriftSchedule(cronExpr)
	if schedule == nil {
		return
	}
	now := time.Now()
	// Walk forward from 2 hours ago to find the most recent expected fire time.
	probe := now.Add(-2 * time.Hour)
	expected := schedule.Next(probe)
	for expected.Before(now) {
		prev := expected
		expected = schedule.Next(expected)
		if expected.After(now) {
			cs.metrics.CronDrift.Record(ctx, now.Sub(prev).Seconds())
			return
		}
	}
}

func (cs *CronScheduler) cacheDriftSchedule(cronExpr string) {
	if cronExpr == "" {
		return
	}
	schedule := parseDriftSchedule(cronExpr)
	if schedule == nil {
		return
	}
	cs.driftMu.Lock()
	cs.driftSchedules[cronExpr] = schedule
	cs.driftMu.Unlock()
}

func (cs *CronScheduler) getDriftSchedule(cronExpr string) cron.Schedule {
	cs.driftMu.RLock()
	schedule := cs.driftSchedules[cronExpr]
	cs.driftMu.RUnlock()
	if schedule != nil {
		return schedule
	}

	schedule = parseDriftSchedule(cronExpr)
	if schedule == nil {
		return nil
	}
	cs.driftMu.Lock()
	if cached := cs.driftSchedules[cronExpr]; cached != nil {
		schedule = cached
	} else {
		cs.driftSchedules[cronExpr] = schedule
	}
	cs.driftMu.Unlock()
	return schedule
}

func parseDriftSchedule(cronExpr string) cron.Schedule {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		return nil
	}
	return schedule
}
