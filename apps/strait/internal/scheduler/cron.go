package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/telemetry"

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
}

type WorkflowTrigger interface {
	TriggerWorkflow(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, stepOverrides []domain.StepOverride, extraTags map[string]string) (*domain.WorkflowRun, error)
}

// MachineStopper can stop a running managed container.
type MachineStopper interface {
	Stop(ctx context.Context, machineID string) error
}

type CronScheduler struct {
	ctx               context.Context
	cron              *cron.Cron
	store             CronStore
	queue             queue.Queue
	workflowTrigger   WorkflowTrigger
	machineStopper    MachineStopper
	workflowCallback  WorkflowCallback
	metrics           *telemetry.Metrics
	defaultRunTTLSecs int
	driftMu           sync.RWMutex
	driftSchedules    map[string]cron.Schedule
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

func (cs *CronScheduler) WithMachineStopper(ms MachineStopper) *CronScheduler {
	cs.machineStopper = ms
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

	for _, j := range jobs {
		job := j
		_, err := cs.cron.AddFunc(job.Cron, func() {
			cs.triggerJob(cs.ctx, job)
		})
		if err != nil {
			return fmt.Errorf("register cron job %s: %w", job.ID, err)
		}
		cs.cacheDriftSchedule(job.Cron)
	}

	if cs.workflowTrigger != nil {
		for _, wf := range workflows {
			workflow := wf
			expr := workflow.Cron
			if workflow.CronTimezone != "" {
				expr = fmt.Sprintf("CRON_TZ=%s %s", workflow.CronTimezone, workflow.Cron)
			}
			_, err := cs.cron.AddFunc(expr, func() {
				cs.triggerWorkflow(cs.ctx, workflow)
			})
			if err != nil {
				return fmt.Errorf("register cron workflow %s: %w", workflow.ID, err)
			}
			cs.cacheDriftSchedule(workflow.Cron)
		}
	}

	slog.Info("cron schedules loaded", "jobs", len(jobs), "workflows", len(workflows))
	return nil
}

func (cs *CronScheduler) triggerJob(ctx context.Context, job domain.Job) {
	ctx, span := otel.Tracer("strait").Start(ctx, "cron.TriggerJob")
	defer span.End()

	cs.recordCronDrift(ctx, job.Cron)

	run := domain.JobRun{
		JobID:        job.ID,
		ProjectID:    job.ProjectID,
		Tags:         job.Tags,
		TriggeredBy:  domain.TriggerCron,
		JobVersion:   job.Version,
		JobVersionID: job.VersionID,
		CreatedBy:    "system:cron",
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

	case domain.OverlapPolicyCancelRunning:
		canceledRuns, err := cs.store.CancelActiveRunsForJob(ctx, job.ID,
			"canceled by cron overlap policy: cancel_running")
		if err != nil {
			slog.Error("failed to cancel active runs for job", "job_id", job.ID, "error", err)
			if cs.metrics != nil {
				cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			}
			return
		}
		if len(canceledRuns) > 0 {
			slog.Info("canceled active runs before cron enqueue",
				"job_id", job.ID, "canceled", len(canceledRuns), "policy", "cancel_running")
			cs.processCanceledRuns(ctx, job.ID, canceledRuns)
		}

	case domain.OverlapPolicyAllow:
		// Default: always enqueue.

	default:
		// Treat unknown/empty as allow for forward compatibility.
	}

	if err := queue.EnqueueWithRetry(ctx, cs.queue, &run, queue.DefaultInternalEnqueueRetryConfig()); err != nil {
		slog.Error("failed to enqueue cron run", "job_id", job.ID, "project_id", job.ProjectID, "error", err)
		if cs.metrics != nil {
			cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		}
		return
	}
	if cs.metrics != nil {
		cs.metrics.CronTriggers.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "success")))
	}

	slog.Info("cron run enqueued", "job_id", job.ID, "project_id", job.ProjectID, "run_id", run.ID)
}

func (cs *CronScheduler) triggerWorkflow(ctx context.Context, workflow domain.Workflow) {
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

	if _, err := cs.workflowTrigger.TriggerWorkflow(ctx, workflow.ID, workflow.ProjectID, nil, domain.TriggerCron, nil, nil); err != nil {
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

// machineStopTimeout caps how long the scheduler waits for a single
// managed container to stop before giving up and moving on.
const machineStopTimeout = 10 * time.Second

// maxConcurrentStops limits the number of concurrent machine stop
// operations during cron cancel to prevent unbounded goroutine spawning.
const maxConcurrentStops = 10

// processCanceledRuns handles side effects for runs canceled by the
// cancel_running overlap policy: stops managed containers, cancels child
// runs, and notifies the workflow engine.
func (cs *CronScheduler) processCanceledRuns(ctx context.Context, jobID string, runs []store.CanceledRun) {
	if cs.machineStopper != nil {
		sem := make(chan struct{}, maxConcurrentStops)
		var wg sync.WaitGroup
		for _, cr := range runs {
			if cr.ExecutionMode == domain.ExecutionModeManaged && cr.MachineID != "" {
				wg.Add(1)
				sem <- struct{}{}
				go func(id, machineID string) { //nolint:gosec // detached context intentional: stop must survive parent cancel
					defer wg.Done()
					defer func() { <-sem }()
					defer func() {
						if r := recover(); r != nil {
							slog.Error("panic stopping container on cron cancel",
								"run_id", id, "machine_id", machineID,
								"panic", fmt.Sprintf("%v", r),
								"stack", string(debug.Stack()),
							)
						}
					}()
					stopCtx, stopCancel := context.WithTimeout(context.Background(), machineStopTimeout)
					defer stopCancel()
					if stopErr := cs.machineStopper.Stop(stopCtx, machineID); stopErr != nil {
						slog.Warn("failed to stop container on cron cancel",
							"run_id", id, "machine_id", machineID, "error", stopErr)
					}
				}(cr.ID, cr.MachineID)
			}
		}
		wg.Wait()
	}

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
			canceledRun := &domain.JobRun{ID: cr.ID, Status: domain.StatusCanceled}
			if cbErr := cs.workflowCallback.OnJobRunTerminal(ctx, canceledRun); cbErr != nil {
				slog.Error("workflow callback failed on cron cancel", "run_id", cr.ID, "error", cbErr)
			}
		}
	}
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
