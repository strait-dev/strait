package grpc

import (
	"testing"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestConnectedWorkerRefs_IncludesAllConnectedWorkers(t *testing.T) {
	registry := NewConnectionRegistry()
	active := &ConnectedWorker{
		WorkerID:       "worker-active",
		ProjectID:      "project-1",
		APIKeyID:       "key-active",
		Queues:         []string{"default"},
		SlotsTotal:     1,
		SlotsAvailable: 1,
		Status:         "active",
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}
	draining := &ConnectedWorker{
		WorkerID:       "worker-draining",
		ProjectID:      "project-1",
		APIKeyID:       "key-draining",
		Queues:         []string{"default"},
		SlotsTotal:     1,
		SlotsAvailable: 1,
		Status:         "draining",
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}
	require.NoError(t, registry.Register(active))
	require.NoError(t, registry.Register(draining))

	refs := connectedWorkerRefs(registry)
	require.Len(t,

		refs, 2)

	got := map[store.ActiveWorkerRef]bool{}
	for _, ref := range refs {
		got[ref] = true
	}
	require.False(
		t,
		!got[store.ActiveWorkerRef{WorkerID: "worker-active",
			ProjectID: "project-1",
		}] ||
			!got[store.ActiveWorkerRef{WorkerID: "worker-draining", ProjectID: "project-1"}])

}
