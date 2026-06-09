package worker

import (
	"context"
	"sync/atomic"

	"strait/internal/queue"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	executorPollTriggerDrain    = "drain"
	executorPollTriggerExternal = "external_wake"
	executorPollTriggerDegraded = "degraded"
	executorPollTriggerTicker   = "ticker"
)

type drainController struct {
	wake        chan struct{}
	backlogHint atomic.Bool
	metrics     *queue.QueueMetrics
}

func newDrainController(metrics *queue.QueueMetrics) *drainController {
	return &drainController{
		wake:    make(chan struct{}, 1),
		metrics: metrics,
	}
}

func (d *drainController) wakeChan() <-chan struct{} {
	if d == nil {
		return nil
	}
	return d.wake
}

func (d *drainController) observePoll(requested, claimed int, err error) {
	if d == nil {
		return
	}
	if err != nil {
		d.backlogHint.Store(false)
		return
	}
	if claimed == 0 {
		d.backlogHint.Store(false)
		return
	}
	if requested <= 0 {
		d.backlogHint.Store(false)
		return
	}
	d.backlogHint.Store(claimed >= requested)
}

func (d *drainController) clear() {
	if d == nil {
		return
	}
	d.backlogHint.Store(false)
}

func (d *drainController) request(ctx context.Context) {
	if d == nil || !d.backlogHint.Load() {
		return
	}
	if d.metrics != nil {
		d.metrics.ExecutorDrainWakeRequested.Add(ctx, 1)
	}
	select {
	case d.wake <- struct{}{}:
		if d.metrics != nil {
			d.metrics.ExecutorDrainWakeDelivered.Add(ctx, 1)
		}
	default:
		if d.metrics != nil {
			d.metrics.ExecutorDrainWakeCoalesced.Add(ctx, 1)
		}
	}
}

func (d *drainController) hasBacklogHint() bool {
	return d != nil && d.backlogHint.Load()
}

func recordExecutorPoll(ctx context.Context, metrics *queue.QueueMetrics, trigger string) {
	if metrics == nil {
		return
	}
	metrics.ExecutorPolls.Add(ctx, 1, metric.WithAttributes(attribute.String("trigger", trigger)))
}
