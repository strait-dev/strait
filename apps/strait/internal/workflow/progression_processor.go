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

type batchProgressionEventStore interface {
	MarkWorkflowProgressionEventsProcessed(ctx context.Context, ids []int64) error
	ReleaseWorkflowProgressionEvents(ctx context.Context, ids []int64) error
}

type workflowStepRunBatchLoader interface {
	ListWorkflowStepRunsByIDs(ctx context.Context, ids []string) ([]domain.WorkflowStepRun, error)
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
	grouped := groupProgressionEventsByWorkflow(events)
	for _, workflowEvents := range grouped {
		if err := p.processWorkflowEvents(ctx, workflowEvents); err != nil {
			p.releaseWorkflowEvents(ctx, workflowEvents, err)
			continue
		}
		if err := p.markWorkflowEventsProcessed(ctx, workflowEvents); err != nil {
			return err
		}
	}
	return nil
}

func (p *ProgressionProcessor) processWorkflowEvents(ctx context.Context, events []store.WorkflowProgressionEvent) error {
	if len(events) == 0 {
		return nil
	}
	if p.callback == nil {
		return fmt.Errorf("nil progression callback")
	}

	stepRuns, err := p.loadStepRuns(ctx, events)
	if err != nil {
		return err
	}
	completedRefs := make([]string, 0, len(stepRuns))
	seenRefs := make(map[string]struct{}, len(stepRuns))
	for i := range stepRuns {
		if stepRuns[i].Status != domain.StepCompleted {
			continue
		}
		if _, seen := seenRefs[stepRuns[i].StepRef]; seen {
			continue
		}
		seenRefs[stepRuns[i].StepRef] = struct{}{}
		completedRefs = append(completedRefs, stepRuns[i].StepRef)
	}
	if len(completedRefs) == 0 {
		return nil
	}

	wc, err := p.callback.loadWfCtx(ctx, events[0].WorkflowRunID)
	if err != nil {
		return err
	}
	if err := p.callback.fanInBatchAndStartReadyChildren(ctx, events[0].WorkflowRunID, completedRefs, wc); err != nil {
		return err
	}
	return p.callback.checkWorkflowCompletion(ctx, events[0].WorkflowRunID, wc)
}

func (p *ProgressionProcessor) loadStepRuns(ctx context.Context, events []store.WorkflowProgressionEvent) ([]domain.WorkflowStepRun, error) {
	ids := make([]string, 0, len(events))
	for _, ev := range events {
		if ev.StepRunID != "" {
			ids = append(ids, ev.StepRunID)
		}
	}
	if loader, ok := p.callback.store.(workflowStepRunBatchLoader); ok {
		stepRuns, err := loader.ListWorkflowStepRunsByIDs(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("list workflow step runs by ids: %w", err)
		}
		if len(stepRuns) != len(ids) {
			return nil, fmt.Errorf("workflow progression step run count = %d, want %d", len(stepRuns), len(ids))
		}
		return stepRuns, nil
	}
	stepRuns := make([]domain.WorkflowStepRun, 0, len(events))
	for _, ev := range events {
		stepRun, err := p.callback.store.GetWorkflowStepRun(ctx, ev.StepRunID)
		if err != nil {
			return nil, fmt.Errorf("get workflow step run: %w", err)
		}
		if stepRun == nil {
			return nil, fmt.Errorf("workflow step run not found: %s", ev.StepRunID)
		}
		stepRuns = append(stepRuns, *stepRun)
	}
	return stepRuns, nil
}

func (p *ProgressionProcessor) markWorkflowEventsProcessed(ctx context.Context, events []store.WorkflowProgressionEvent) error {
	ids := progressionEventIDs(events)
	if batchStore, ok := p.store.(batchProgressionEventStore); ok {
		return batchStore.MarkWorkflowProgressionEventsProcessed(ctx, ids)
	}
	for _, id := range ids {
		if err := p.store.MarkWorkflowProgressionEventProcessed(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (p *ProgressionProcessor) releaseWorkflowEvents(ctx context.Context, events []store.WorkflowProgressionEvent, cause error) {
	ids := progressionEventIDs(events)
	for _, ev := range events {
		p.logger.Warn("workflow progression event failed", "event_id", ev.ID, "step_run_id", ev.StepRunID, "error", cause)
	}
	if batchStore, ok := p.store.(batchProgressionEventStore); ok {
		_ = batchStore.ReleaseWorkflowProgressionEvents(ctx, ids)
		return
	}
	for _, id := range ids {
		_ = p.store.ReleaseWorkflowProgressionEvent(ctx, id)
	}
}

func progressionEventIDs(events []store.WorkflowProgressionEvent) []int64 {
	ids := make([]int64, 0, len(events))
	for _, ev := range events {
		ids = append(ids, ev.ID)
	}
	return ids
}

func groupProgressionEventsByWorkflow(events []store.WorkflowProgressionEvent) map[string][]store.WorkflowProgressionEvent {
	if len(events) == 0 {
		return make(map[string][]store.WorkflowProgressionEvent)
	}

	firstWorkflowID := events[0].WorkflowRunID
	allSameWorkflow := true
	for i := 1; i < len(events); i++ {
		if events[i].WorkflowRunID != firstWorkflowID {
			allSameWorkflow = false
			break
		}
	}
	if allSameWorkflow {
		return map[string][]store.WorkflowProgressionEvent{firstWorkflowID: events}
	}

	out := make(map[string][]store.WorkflowProgressionEvent, len(events))
	for _, ev := range events {
		out[ev.WorkflowRunID] = append(out[ev.WorkflowRunID], ev)
	}
	return out
}
