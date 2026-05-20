package grpc

import (
	"testing"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
)

func TestConnectedWorkerIDs_IncludesAllConnectedWorkers(t *testing.T) {
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

	ids := connectedWorkerIDs(registry)
	if len(ids) != 2 {
		t.Fatalf("connectedWorkerIDs = %#v, want two connected workers", ids)
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if !got["worker-active"] || !got["worker-draining"] {
		t.Fatalf("connectedWorkerIDs = %#v, want active and draining workers", ids)
	}
}
