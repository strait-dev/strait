package loadtest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
)

type blockingAuditStore struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingAuditStore() *blockingAuditStore {
	return &blockingAuditStore{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (s *blockingAuditStore) CreateAuditEvent(context.Context, *domain.AuditEvent) error {
	s.once.Do(func() { close(s.started) })
	<-s.release
	return nil
}

func (s *blockingAuditStore) CreateAuditEventDeadletter(context.Context, *domain.AuditEvent, string, int) error {
	return nil
}

func testAuditEvent(i int) *domain.AuditEvent {
	return &domain.AuditEvent{
		ProjectID:    "proj-load",
		ActorID:      "actor-load",
		ActorType:    "user",
		Action:       "job.triggered",
		ResourceType: "job",
		ResourceID:   fmt.Sprintf("job-%d", i),
	}
}

func TestAuditEmitHarness_WaitDrainWaitsForInFlightEvent(t *testing.T) {
	t.Parallel()

	store := newBlockingAuditStore()
	h := NewAuditEmitHarness(store, nil, AuditEmitHarnessConfig{BufferSize: 1})
	h.Start()
	t.Cleanup(h.Stop)

	h.Emit(testAuditEvent(1))
	select {
	case <-store.started:
	case <-time.After(time.Second):
		t.Fatal("drainer did not start processing event")
	}
	if h.WaitDrain(20 * time.Millisecond) {
		t.Fatal("WaitDrain returned true while final event was still in flight")
	}

	close(store.release)
	if !h.WaitDrain(time.Second) {
		t.Fatal("WaitDrain did not return after in-flight event completed")
	}
}

func TestAuditEmitHarness_ConcurrentEmitAndStopDoesNotPanic(t *testing.T) {
	t.Parallel()

	store := NewMemoryAuditStore()
	h := NewAuditEmitHarness(store, nil, AuditEmitHarnessConfig{BufferSize: 8})
	h.Start()

	panicCh := make(chan any, 32)
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicCh <- r
				}
			}()
			h.Emit(testAuditEvent(i))
		}(i)
	}
	h.Stop()
	wg.Wait()
	close(panicCh)

	if panicValue, ok := <-panicCh; ok {
		t.Fatalf("Emit panicked during concurrent Stop: %v", panicValue)
	}
}
