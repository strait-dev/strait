package grpc

import (
	"testing"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/store"
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
	if err := registry.Register(active); err != nil {
		t.Fatalf("register active: %v", err)
	}
	if err := registry.Register(draining); err != nil {
		t.Fatalf("register draining: %v", err)
	}

	refs := connectedWorkerRefs(registry)
	if len(refs) != 2 {
		t.Fatalf("connectedWorkerRefs = %#v, want two connected workers", refs)
	}
	got := map[store.ActiveWorkerRef]bool{}
	for _, ref := range refs {
		got[ref] = true
	}
	if !got[store.ActiveWorkerRef{WorkerID: "worker-active", ProjectID: "project-1"}] ||
		!got[store.ActiveWorkerRef{WorkerID: "worker-draining", ProjectID: "project-1"}] {
		t.Fatalf("connectedWorkerRefs = %#v, want active and draining workers", refs)
	}
}
