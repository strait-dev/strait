package scheduler

import (
	"context"
	"fmt"
	"log/slog"

	"orchestrator/internal/domain"
	"orchestrator/internal/queue"

	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel"
)

// CronStore is the subset of store operations needed by CronScheduler.
type CronStore interface {
	ListCronJobs(ctx context.Context) ([]domain.Job, error)
}

type CronScheduler struct {
	cron  *cron.Cron
	store CronStore
	queue queue.Queue
}

func NewCronScheduler(s CronStore, q queue.Queue) *CronScheduler {
	return &CronScheduler{
		cron:  cron.New(),
		store: s,
		queue: q,
	}
}

func (cs *CronScheduler) LoadJobs(ctx context.Context) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "cron.LoadJobs")
	defer span.End()

	jobs, err := cs.store.ListCronJobs(ctx)
	if err != nil {
		return fmt.Errorf("list cron jobs: %w", err)
	}

	for _, j := range jobs {
		job := j
		_, err := cs.cron.AddFunc(job.Cron, func() {
			cs.triggerJob(ctx, job)
		})
		if err != nil {
			return fmt.Errorf("register cron job %s: %w", job.ID, err)
		}
	}

	slog.Info("cron jobs loaded", "count", len(jobs))
	return nil
}

func (cs *CronScheduler) triggerJob(ctx context.Context, job domain.Job) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "cron.TriggerJob")
	defer span.End()

	run := domain.JobRun{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		TriggeredBy: domain.TriggerCron,
	}

	if err := cs.queue.Enqueue(ctx, &run); err != nil {
		slog.Error("failed to enqueue cron run", "job_id", job.ID, "project_id", job.ProjectID, "error", err)
		return
	}

	slog.Info("cron run enqueued", "job_id", job.ID, "project_id", job.ProjectID, "run_id", run.ID)
}

func (cs *CronScheduler) Start() {
	cs.cron.Start()
}

func (cs *CronScheduler) Stop() context.Context {
	return cs.cron.Stop()
}
