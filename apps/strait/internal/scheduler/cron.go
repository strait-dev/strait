package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
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
	// overlapDeprecationLogged dedupes the per-job deprecation warning emitted
	// when an explicit singleton config supersedes a legacy cron_overlap_policy.
	overlapDeprecationLogged sync.Map
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

	// Unify the legacy cron_overlap_policy with first-class singleton execution:
	// the singleton lock table is the single mechanism that decides what happens
	// when a cron fire overlaps a still-active run. Explicit singleton config wins;
	// skip -> drop and cancel_running -> replace map onto a per-job constant key.
	eff := domain.EffectiveJobCronSingleton(&job)
	if eff.Configured {
		if eff.LegacyOverridden {
			cs.logOverlapDeprecation(job.ID)
		}
		key, keyErr := resolveCronSingletonKey(eff.KeyExpr)
		if keyErr != nil {
			// A key template that needs payload interpolation cannot resolve for a
			// cron fire (no payload). Rather than silently drop the run, fall back to
			// a plain enqueue and surface the misconfiguration.
			slog.Warn("cron singleton key unresolvable; enqueuing without singleton enforcement",
				"job_id", job.ID, "project_id", job.ProjectID, "error", keyErr)
			if cs.metrics != nil {
				cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "warning")))
			}
			eff.Configured = false
		} else {
			run.SingletonKey = key
		}
	}

	var outcome domain.SingletonOutcome
	var holderID string
	err := cs.withCronAdmissionGuard(ctx, &job, func(enqueueCtx context.Context, tx store.DBTX) error {
		if eff.Configured {
			if tx == nil {
				// The singleton acquire and the run insert must commit together; a
				// non-transactional store (test doubles) cannot guarantee that.
				slog.Warn("cron singleton requires a transactional store; enqueuing without enforcement",
					"job_id", job.ID, "project_id", job.ProjectID)
			} else {
				proceed, oc, hid, serr := store.New(tx).ApplyJobSingletonConflictPolicy(
					enqueueCtx, &run, job.ProjectID, job.ID, run.SingletonKey, eff.OnConflict, eff.MaxQueueDepth, eff.Preempt,
				)
				if serr != nil {
					return serr
				}
				outcome, holderID = oc, hid
				switch {
				case oc == domain.SingletonOutcomeDispatched:
					cs.recordSingletonAcquisition(enqueueCtx)
				case oc == domain.SingletonOutcomeReplaced && eff.OnConflict == domain.SingletonOnConflictQueue:
					cs.recordSingletonPreemption(enqueueCtx)
				default:
					cs.recordSingletonConflict(enqueueCtx, eff.OnConflict)
				}
				if !proceed {
					// dropped / queued_behind / replaced: the policy parked or discarded
					// the run inside this tx. Nothing left to enqueue.
					return nil
				}
				// dispatched: the run acquired the key; enqueue it holding the lock.
			}
		}
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

	// A replace canceled the prior holder inside the policy tx; cancel its child
	// runs and notify the workflow engine, matching the legacy cancel_running side
	// effects. The singleton lock release and successor promotion are handled by
	// the canceled holder's terminal transition (reaper / fast-path).
	if outcome == domain.SingletonOutcomeReplaced && holderID != "" {
		cs.processCanceledRuns(ctx, job.ID, []store.CanceledRun{{
			ID:            holderID,
			JobID:         job.ID,
			ProjectID:     job.ProjectID,
			ExecutionMode: job.ExecutionMode,
		}})
	}

	switch outcome {
	case domain.SingletonOutcomeDropped:
		if cs.metrics != nil {
			cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "skipped")))
		}
		slog.Info("cron run dropped by singleton policy",
			"job_id", job.ID, "project_id", job.ProjectID, "singleton_key", run.SingletonKey)
		return
	case domain.SingletonOutcomeQueuedBehind, domain.SingletonOutcomeReplaced:
		// Both park the newcomer rather than enqueue it: queued_behind waits for the
		// holder to finish, replaced waits for the just-canceled holder to release.
		if cs.metrics != nil {
			cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "success")))
		}
		slog.Info("cron run parked behind singleton holder",
			"job_id", job.ID, "project_id", job.ProjectID, "run_id", run.ID,
			"singleton_key", run.SingletonKey, "outcome", string(outcome))
		return
	case domain.SingletonOutcomeDispatched:
		// Acquired the key (or no singleton): fall through to the enqueue success log.
	}

	if cs.metrics != nil {
		cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "success")))
	}

	slog.Info("cron run enqueued", "job_id", job.ID, "project_id", job.ProjectID, "run_id", run.ID)
}

// resolveCronSingletonKey resolves a singleton key expression for a cron fire,
// which carries no payload. A constant template resolves to itself; a template
// with ${path} interpolation cannot resolve and returns an error so the caller
// can fall back to a plain enqueue.
func resolveCronSingletonKey(keyExpr json.RawMessage) (string, error) {
	expr, err := domain.ParseSingletonKeyExpr(keyExpr)
	if err != nil {
		return "", err
	}
	key, err := domain.ResolveSingletonKey(expr, nil)
	if err != nil {
		return "", err
	}
	if key == "" {
		return "", fmt.Errorf("singleton key resolved to an empty value")
	}
	return key, nil
}

// logOverlapDeprecation warns once per job that an explicit singleton config has
// superseded a legacy cron_overlap_policy set on the same job.
func (cs *CronScheduler) logOverlapDeprecation(jobID string) {
	if _, loaded := cs.overlapDeprecationLogged.LoadOrStore(jobID, struct{}{}); loaded {
		return
	}
	slog.Warn("cron_overlap_policy is ignored because the job has explicit singleton config; "+
		"remove cron_overlap_policy and rely on singleton settings",
		"job_id", jobID)
}

// recordSingletonAcquisition / recordSingletonConflict mirror the API trigger
// metrics so cron-driven singleton activity shows up under the same counters.
func (cs *CronScheduler) recordSingletonAcquisition(ctx context.Context) {
	if cs.metrics == nil || cs.metrics.SingletonAcquisitions == nil {
		return
	}
	cs.metrics.SingletonAcquisitions.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", string(domain.SingletonKindJob)),
	))
}

func (cs *CronScheduler) recordSingletonConflict(ctx context.Context, policy domain.SingletonOnConflict) {
	if cs.metrics == nil || cs.metrics.SingletonConflicts == nil {
		return
	}
	cs.metrics.SingletonConflicts.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", string(domain.SingletonKindJob)),
		attribute.String("policy", string(policy)),
	))
}

func (cs *CronScheduler) recordSingletonPreemption(ctx context.Context) {
	if cs.metrics == nil || cs.metrics.SingletonPreemptions == nil {
		return
	}
	cs.metrics.SingletonPreemptions.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", string(domain.SingletonKindJob)),
	))
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

	// SkipIfRunning is no longer enforced here: the workflow engine maps it (and
	// explicit workflow singleton config) onto the singleton lock table inside
	// TriggerWorkflow, so an overlapping cron fire is dropped there atomically with
	// run creation rather than via a racy pre-check.
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
	return fmt.Sprintf("cron:%s:%s:%d", kind, id, now.Truncate(time.Minute).Unix())
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
