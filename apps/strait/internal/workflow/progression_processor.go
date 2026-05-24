package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

type ProgressionEventStore interface {
	ClaimWorkflowProgressionEvents(ctx context.Context, limit int) ([]store.WorkflowProgressionEvent, error)
	MarkWorkflowProgressionEventProcessed(ctx context.Context, id int64) error
	ReleaseWorkflowProgressionEvent(ctx context.Context, id int64) error
}

type ProgressionProcessor struct {
	store    ProgressionEventStore
	callback *StepCallback
	interval time.Duration
	limit    int
	logger   *slog.Logger
}

type ProgressionProcessorConfig struct {
	Interval time.Duration
	Limit    int
	Logger   *slog.Logger
}

func NewProgressionProcessor(store ProgressionEventStore, callback *StepCallback, cfg ProgressionProcessorConfig) *ProgressionProcessor {
	p := &ProgressionProcessor{
		store:    store,
		callback: callback,
		interval: cfg.Interval,
		limit:    cfg.Limit,
		logger:   cfg.Logger,
	}
	if p.interval <= 0 {
		p.interval = 100 * time.Millisecond
	}
	if p.limit <= 0 {
		p.limit = 100
	}
	if p.logger == nil {
		p.logger = slog.Default()
	}
	return p
}

func (p *ProgressionProcessor) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	_ = p.ProcessOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = p.ProcessOnce(ctx)
		}
	}
}

func (p *ProgressionProcessor) ProcessOnce(ctx context.Context) error {
	events, err := p.store.ClaimWorkflowProgressionEvents(ctx, p.limit)
	if err != nil {
		return err
	}
	for _, ev := range events {
		if err := p.processEvent(ctx, ev); err != nil {
			p.logger.Warn("workflow progression event failed", "event_id", ev.ID, "step_run_id", ev.StepRunID, "error", err)
			_ = p.store.ReleaseWorkflowProgressionEvent(ctx, ev.ID)
			continue
		}
		if err := p.store.MarkWorkflowProgressionEventProcessed(ctx, ev.ID); err != nil {
			return err
		}
	}
	return nil
}

func (p *ProgressionProcessor) processEvent(ctx context.Context, ev store.WorkflowProgressionEvent) error {
	if p.callback == nil {
		return fmt.Errorf("nil progression callback")
	}
	stepRun, err := p.callback.store.GetWorkflowStepRun(ctx, ev.StepRunID)
	if err != nil {
		return fmt.Errorf("get workflow step run: %w", err)
	}
	if stepRun == nil {
		return fmt.Errorf("workflow step run not found: %s", ev.StepRunID)
	}
	if stepRun.Status != domain.StepCompleted {
		return nil
	}
	wc, err := p.callback.loadWfCtx(ctx, stepRun.WorkflowRunID)
	if err != nil {
		return err
	}
	if err := p.callback.fanInAndStartReadyChildren(ctx, stepRun, wc); err != nil {
		return err
	}
	return p.callback.checkWorkflowCompletion(ctx, stepRun.WorkflowRunID, wc)
}

func groupProgressionEventsByWorkflow(events []store.WorkflowProgressionEvent) map[string][]store.WorkflowProgressionEvent {
	out := make(map[string][]store.WorkflowProgressionEvent)
	for _, ev := range events {
		out[ev.WorkflowRunID] = append(out[ev.WorkflowRunID], ev)
	}
	return out
}
