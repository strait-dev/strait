package logdrain

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"strait/internal/domain"
)

const (
	defaultBatchSize  = 100
	defaultEventLimit = 1000
)

// DrainStore is the subset of store operations needed by the worker.
type DrainStore interface {
	ListLogDrains(ctx context.Context, projectID string) ([]domain.LogDrain, error)
	ListEvents(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
	ListEnabledLogDrains(ctx context.Context) ([]domain.LogDrain, error)
	ListFinishedRunsSince(ctx context.Context, projectID string, since time.Time, limit int) ([]domain.JobRun, error)
}

// Worker periodically drains logs for finished runs.
type Worker struct {
	store    DrainStore
	service  *Service
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}

	mu          sync.Mutex
	checkpoints map[string]time.Time // drain ID -> last drained timestamp
}

func NewWorker(store DrainStore, service *Service, interval time.Duration) *Worker {
	return &Worker{
		store:       store,
		service:     service,
		interval:    interval,
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
		checkpoints: make(map[string]time.Time),
	}
}

// Run starts the background worker. It blocks until Stop is called.
func (w *Worker) Run(ctx context.Context) {
	defer close(w.done)
	slog.Info("log drain worker started", "interval", w.interval)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

// Stop signals the worker to stop.
func (w *Worker) Stop() {
	close(w.stop)
	<-w.done
}

func (w *Worker) tick(ctx context.Context) {
	if w.store == nil {
		return
	}

	drains, err := w.store.ListEnabledLogDrains(ctx)
	if err != nil {
		slog.Error("log drain worker: list enabled drains", "error", err)
		return
	}

	if len(drains) == 0 {
		return
	}

	for i := range drains {
		if err := ctx.Err(); err != nil {
			return
		}
		w.processDrain(ctx, &drains[i])
	}
}

func (w *Worker) processDrain(ctx context.Context, drain *domain.LogDrain) {
	w.mu.Lock()
	checkpoint := w.checkpoints[drain.ID]
	w.mu.Unlock()

	if checkpoint.IsZero() {
		// First run: start from 1 minute ago to avoid processing the entire history.
		checkpoint = time.Now().Add(-1 * time.Minute)
	}

	runs, err := w.store.ListFinishedRunsSince(ctx, drain.ProjectID, checkpoint, defaultBatchSize)
	if err != nil {
		slog.Error("log drain worker: list finished runs",
			"drain_id", drain.ID,
			"project_id", drain.ProjectID,
			"error", err,
		)
		return
	}

	if len(runs) == 0 {
		return
	}

	var latestFinished time.Time
	for i := range runs {
		if err := ctx.Err(); err != nil {
			return
		}
		run := &runs[i]
		if run.FinishedAt != nil && run.FinishedAt.After(latestFinished) {
			latestFinished = *run.FinishedAt
		}

		events, evErr := w.store.ListEvents(ctx, run.ID, defaultEventLimit, nil)
		if evErr != nil {
			slog.Error("log drain worker: list events",
				"drain_id", drain.ID,
				"run_id", run.ID,
				"error", evErr,
			)
			continue
		}

		if len(events) == 0 {
			continue
		}

		if drainErr := w.service.DrainRunEvents(ctx, drain, events); drainErr != nil {
			slog.Error("log drain worker: drain events",
				"drain_id", drain.ID,
				"run_id", run.ID,
				"endpoint", drain.EndpointURL,
				"error", drainErr,
			)
			// Don't update checkpoint on failure so we retry next tick.
			return
		}

		slog.Debug("log drain worker: drained events",
			"drain_id", drain.ID,
			"run_id", run.ID,
			"event_count", len(events),
		)
	}

	if !latestFinished.IsZero() {
		w.mu.Lock()
		w.checkpoints[drain.ID] = latestFinished
		w.mu.Unlock()
	}
}
