package grpc

import (
	"context"
	"errors"
	"testing"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestDBSyncOnceRegistersSnapshotWorkers(t *testing.T) {
	t.Parallel()

	registry := NewConnectionRegistry()
	require.NoError(t, registry.Register(&ConnectedWorker{
		WorkerID:       "worker-with-queues",
		ProjectID:      "project-1",
		APIKeyID:       "key-1",
		Queues:         []string{"critical", "default"},
		SlotsTotal:     1,
		SlotsAvailable: 1,
		Hostname:       "host-a",
		SDKVersion:     "v1.2.3",
		Status:         "active",
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}))
	require.NoError(t, registry.Register(&ConnectedWorker{
		WorkerID:       "worker-without-queues",
		ProjectID:      "project-1",
		APIKeyID:       "key-2",
		SlotsTotal:     1,
		SlotsAvailable: 1,
		Hostname:       "host-b",
		SDKVersion:     "v2.0.0",
		Status:         "draining",
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}))
	registrar := &recordingWorkerRegistrar{}

	dbSyncOnce(context.Background(), registry, registrar)

	require.Len(t, registrar.workers, 2)
	got := map[string]*domain.Worker{}
	for _, worker := range registrar.workers {
		got[worker.ID] = worker
	}
	require.Equal(t, &domain.Worker{
		ID:        "worker-with-queues",
		ProjectID: "project-1",
		QueueName: "critical",
		Hostname:  "host-a",
		Version:   "v1.2.3",
		Status:    domain.WorkerStatus("active"),
	}, got["worker-with-queues"])
	require.Equal(t, &domain.Worker{
		ID:        "worker-without-queues",
		ProjectID: "project-1",
		QueueName: "",
		Hostname:  "host-b",
		Version:   "v2.0.0",
		Status:    domain.WorkerStatus("draining"),
	}, got["worker-without-queues"])
}

func TestDBSyncOnceContinuesAfterRegisterWorkerError(t *testing.T) {
	t.Parallel()

	registry := NewConnectionRegistry()
	require.NoError(t, registry.Register(&ConnectedWorker{
		WorkerID:       "worker-fails",
		ProjectID:      "project-1",
		APIKeyID:       "key-1",
		SlotsTotal:     1,
		SlotsAvailable: 1,
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}))
	require.NoError(t, registry.Register(&ConnectedWorker{
		WorkerID:       "worker-succeeds",
		ProjectID:      "project-1",
		APIKeyID:       "key-2",
		SlotsTotal:     1,
		SlotsAvailable: 1,
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}))
	registrar := &recordingWorkerRegistrar{
		errByWorkerID: map[string]error{
			"worker-fails": errors.New("register failed"),
		},
	}

	dbSyncOnce(context.Background(), registry, registrar)

	require.Len(t, registrar.workers, 2)
	registered := map[string]bool{}
	for _, worker := range registrar.workers {
		registered[worker.ID] = true
	}
	require.True(t, registered["worker-fails"])
	require.True(t, registered["worker-succeeds"])
}

type recordingWorkerRegistrar struct {
	workers       []*domain.Worker
	errByWorkerID map[string]error
}

func (r *recordingWorkerRegistrar) RegisterWorker(_ context.Context, worker *domain.Worker) error {
	cp := *worker
	r.workers = append(r.workers, &cp)
	if r.errByWorkerID != nil {
		return r.errByWorkerID[worker.ID]
	}
	return nil
}
