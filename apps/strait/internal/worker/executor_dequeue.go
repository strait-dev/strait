package worker

import (
	"context"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
)

// workerQueueDequeuer claims worker-mode runs for specific queues.
type workerQueueDequeuer interface {
	DequeueNForWorkerQueues(ctx context.Context, n int, queues []domain.WorkerQueueRef) ([]domain.JobRun, error)
}

// QueueSnapshotter returns the environment-qualified queues that have active
// workers connected to this replica. Implemented by grpc.ConnectionRegistry via
// an adapter to avoid a circular import.
type QueueSnapshotter interface {
	SnapshotWorkerQueues() []domain.WorkerQueueRef
}

type workerQueueAvailabilitySnapshotter interface {
	SnapshotWorkerQueueAvailability() domain.WorkerQueueAvailability
}

// dequeueRuns fetches up to capacity runs from the queue.
// In fair-share mode it round-robins across partitions; otherwise it performs
// a two-pass dequeue: HTTP-eligible runs first, then worker-eligible runs.
func (e *Executor) dequeueRuns(ctx context.Context, capacity int) ([]domain.JobRun, int, error) {
	if len(e.partitionCycle) > 0 {
		runs, err := e.dequeueAcrossPartitions(ctx, capacity)
		return runs, capacity, err
	}

	// Pass 1: HTTP-eligible runs (any replica can dispatch these).
	claimed := newClaimedRunBatch(capacity)
	runs, err := e.queue.DequeueN(ctx, claimed.remaining())
	if err != nil {
		return nil, capacity, err
	}
	claimed.append(runs)

	// Pass 2: Worker-eligible runs - only attempt if this replica has
	// connected workers and capacity remains after the HTTP pass.
	workerRequested := e.appendWorkerRuns(ctx, &claimed)
	requested := capacity
	if workerRequested >= 0 {
		requested = len(runs) + workerRequested
	}
	return claimed.runs, requested, nil
}

// appendWorkerRuns dequeues worker-mode runs and appends them to runs when
// connected workers are available and remaining capacity allows it.
func (e *Executor) appendWorkerRuns(ctx context.Context, claimed *claimedRunBatch) int {
	if e.queueSnapshotter == nil {
		return -1
	}
	workerLimit := claimed.remaining()
	var workerQueues []domain.WorkerQueueRef
	if snapshotter, ok := e.queueSnapshotter.(workerQueueAvailabilitySnapshotter); ok {
		availability := snapshotter.SnapshotWorkerQueueAvailability()
		workerQueues = availability.Queues
		workerLimit = min(workerLimit, availability.SlotsAvailable)
	} else {
		workerQueues = e.queueSnapshotter.SnapshotWorkerQueues()
	}
	if len(workerQueues) == 0 {
		return 0
	}
	wq, ok := e.queue.(workerQueueDequeuer)
	if !ok || claimed.full() || workerLimit <= 0 {
		return 0
	}
	workerRuns, wErr := wq.DequeueNForWorkerQueues(ctx, workerLimit, workerQueues)
	if wErr != nil {
		// Log but don't block the HTTP pass result.
		e.logger.Warn("worker dequeue failed", "error", wErr)
		return workerLimit
	}
	claimed.append(workerRuns)
	return workerLimit
}

func (e *Executor) dequeueAcrossPartitions(ctx context.Context, capacity int) ([]domain.JobRun, error) {
	claimed := newClaimedRunBatch(capacity)
	if capacity <= 0 || len(e.partitionCycle) == 0 {
		return claimed.runs, nil
	}

	iterations := len(e.partitionCycle)
	qm, _ := queue.Metrics()
	for i := 0; i < iterations && !claimed.full(); i++ {
		partition := e.partitionCycle[e.nextPartition%len(e.partitionCycle)]
		e.nextPartition = (e.nextPartition + 1) % len(e.partitionCycle)

		partStart := time.Now()
		runs, err := e.queue.DequeueNByProject(ctx, claimed.remaining(), partition)
		if qm != nil {
			// Avoid attaching partition/project_id as a label here;
			// in fair-share mode that would explode Prometheus cardinality.
			qm.PartitionDequeueLag.Record(ctx, time.Since(partStart).Seconds())
		}
		if err != nil {
			return nil, err
		}
		if len(runs) == 0 {
			continue
		}

		claimed.append(runs)
	}

	return claimed.runs, nil
}

type claimedRunBatch struct {
	runs     []domain.JobRun
	capacity int
}

func newClaimedRunBatch(capacity int) claimedRunBatch {
	if capacity < 0 {
		capacity = 0
	}
	return claimedRunBatch{
		runs:     make([]domain.JobRun, 0, capacity),
		capacity: capacity,
	}
}

func (b *claimedRunBatch) append(runs []domain.JobRun) {
	b.runs = append(b.runs, runs...)
}

func (b *claimedRunBatch) full() bool {
	return b.remaining() <= 0
}

func (b *claimedRunBatch) remaining() int {
	return max(b.capacity-len(b.runs), 0)
}
