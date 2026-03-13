package logdrain

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/domain"
)

// DrainStore is the subset of store operations needed by the worker.
type DrainStore interface {
	ListLogDrains(ctx context.Context, projectID string) ([]domain.LogDrain, error)
	ListEvents(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
}

// Worker periodically drains logs for finished runs.
type Worker struct {
	store    DrainStore
	service  *Service
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
}

func NewWorker(store DrainStore, service *Service, interval time.Duration) *Worker {
	return &Worker{
		store:    store,
		service:  service,
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
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
			// Worker tick - actual draining logic will be wired later
			// when we have a way to track which runs need draining.
		}
	}
}

// Stop signals the worker to stop.
func (w *Worker) Stop() {
	close(w.stop)
	<-w.done
}
