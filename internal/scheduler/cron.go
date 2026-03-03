package scheduler

import (
	"context"
	"fmt"
	"log/slog"

	"orchestrator/internal/domain"
	"orchestrator/internal/queue"
	"orchestrator/internal/store"

	"github.com/robfig/cron/v3"
)

type CronScheduler struct {
	cron  *cron.Cron
	store store.Store
	queue queue.Queue
}

func NewCronScheduler(s store.Store, q queue.Queue) *CronScheduler {
	return &CronScheduler{
		cron:  cron.New(),
		store: s,
		queue: q,
	}
}

func (cs *CronScheduler) LoadJobs(ctx context.Context) error {
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
