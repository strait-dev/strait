package logdrain

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"strait/internal/domain"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	defaultBatchSize  = 100
	defaultEventLimit = 1000
	maxRunRetries     = 3
)

// drainCursor is a composite cursor using both timestamp and run ID
// to avoid skipping runs that share the same finished_at timestamp.
type drainCursor struct {
	FinishedAt time.Time
	RunID      string
}

// DrainStore is the subset of store operations needed by the worker.
type DrainStore interface {
	ListLogDrains(ctx context.Context, projectID string) ([]domain.LogDrain, error)
	ListEventsAsc(ctx context.Context, runID string, limit int, afterTime *time.Time, afterID string) ([]domain.RunEvent, error)
	ListEnabledLogDrains(ctx context.Context) ([]domain.LogDrain, error)
	ListFinishedRunsSince(ctx context.Context, projectID string, since time.Time, sinceRunID string, limit int) ([]domain.JobRun, error)
}

// Worker periodically drains logs for finished runs.
type Worker struct {
	store    DrainStore
	service  *Service
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}

	mu            sync.Mutex
	checkpoints   map[string]drainCursor // drain ID -> composite cursor
	failCounts    map[string]int         // "drainID:runID" -> consecutive failure count
	eventsCounter metric.Int64Counter
}

func NewWorker(store DrainStore, service *Service, interval time.Duration) *Worker {
	return &Worker{
		store:       store,
		service:     service,
		interval:    interval,
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
		checkpoints: make(map[string]drainCursor),
		failCounts:  make(map[string]int),
	}
}

// WithEventsCounter attaches an OTel counter for tracking log drain event outcomes.
func (w *Worker) WithEventsCounter(c metric.Int64Counter) *Worker {
	w.eventsCounter = c
	return w
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
	cursor := w.checkpoints[drain.ID]
	w.mu.Unlock()

	// Zero-value cursor is valid: the SQL query `finished_at > time.Time{}`
	// matches everything, and ORDER BY ... ASC LIMIT efficiently scans from earliest.
	// This ensures no data is lost on restart.

	for {
		runs, err := w.store.ListFinishedRunsSince(ctx, drain.ProjectID, cursor.FinishedAt, cursor.RunID, defaultBatchSize)
		if err != nil {
			slog.Error("log drain worker: list finished runs",
				"drain_id", drain.ID,
				"project_id", drain.ProjectID,
				"error", err,
			)
			return
		}

		if len(runs) == 0 {
			break
		}

		for i := range runs {
			if err := ctx.Err(); err != nil {
				return
			}
			run := &runs[i]

			failKey := drain.ID + ":" + run.ID
			w.mu.Lock()
			failures := w.failCounts[failKey]
			w.mu.Unlock()

			if failures >= maxRunRetries {
				slog.Error("log drain worker: skipping poison run after max retries",
					"drain_id", drain.ID,
					"run_id", run.ID,
					"retries", failures,
				)
				w.mu.Lock()
				delete(w.failCounts, failKey)
				w.mu.Unlock()
				// Advance past the poison run.
				if run.FinishedAt != nil {
					cursor = advanceCursor(cursor, *run.FinishedAt, run.ID)
				}
				continue
			}

			events, evErr := w.fetchAllEvents(ctx, run.ID)
			if evErr != nil {
				slog.Error("log drain worker: list events",
					"drain_id", drain.ID,
					"run_id", run.ID,
					"error", evErr,
				)
				// Do NOT advance cursor past this run.
				continue
			}

			if len(events) == 0 {
				// Safe to advance — no events to lose.
				if run.FinishedAt != nil {
					cursor = advanceCursor(cursor, *run.FinishedAt, run.ID)
				}
				continue
			}

			if drainErr := w.service.DrainRunEvents(ctx, drain, events); drainErr != nil {
				slog.Error("log drain worker: drain events",
					"drain_id", drain.ID,
					"run_id", run.ID,
					"endpoint", drain.EndpointURL,
					"error", drainErr,
				)
				if w.eventsCounter != nil {
					w.eventsCounter.Add(ctx, int64(len(events)), metric.WithAttributes(attribute.String("status", "error")))
				}
				w.mu.Lock()
				w.failCounts[failKey]++
				w.mu.Unlock()
				// Continue to next run instead of returning.
				continue
			}

			if w.eventsCounter != nil {
				w.eventsCounter.Add(ctx, int64(len(events)), metric.WithAttributes(attribute.String("status", "success")))
			}

			// Success — advance cursor and clear fail count.
			w.mu.Lock()
			delete(w.failCounts, failKey)
			w.mu.Unlock()

			if run.FinishedAt != nil {
				cursor = advanceCursor(cursor, *run.FinishedAt, run.ID)
			}

			slog.Debug("log drain worker: drained events",
				"drain_id", drain.ID,
				"run_id", run.ID,
				"event_count", len(events),
			)
		}

		// Persist cursor progress after each page.
		w.mu.Lock()
		w.checkpoints[drain.ID] = cursor
		w.mu.Unlock()

		if len(runs) < defaultBatchSize {
			break // last page
		}
	}
}

// fetchAllEvents paginates forward through all events for a run using a
// composite cursor (created_at, id) to avoid skipping events with duplicate timestamps.
func (w *Worker) fetchAllEvents(ctx context.Context, runID string) ([]domain.RunEvent, error) {
	var all []domain.RunEvent
	var afterTime *time.Time
	var afterID string
	for {
		batch, err := w.store.ListEventsAsc(ctx, runID, defaultEventLimit, afterTime, afterID)
		if err != nil {
			return nil, err
		}
		if len(batch) < defaultEventLimit {
			if len(all) == 0 {
				return batch, nil
			}
			all = append(all, batch...)
			break
		}
		if len(all) == 0 {
			all = make([]domain.RunEvent, 0, len(batch)+defaultEventLimit)
		}
		all = append(all, batch...)
		last := batch[len(batch)-1]
		afterTime = &last.CreatedAt
		afterID = last.ID
	}
	return all, nil
}

// advanceCursor returns a new cursor that is at least as far as (finishedAt, runID).
func advanceCursor(cur drainCursor, finishedAt time.Time, runID string) drainCursor {
	if finishedAt.After(cur.FinishedAt) || (finishedAt.Equal(cur.FinishedAt) && runID > cur.RunID) {
		return drainCursor{FinishedAt: finishedAt, RunID: runID}
	}
	return cur
}
