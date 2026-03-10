//go:build integration
// +build integration

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
)

// TestEventTriggerLoadCreate measures throughput of creating event triggers.
func TestEventTriggerLoadCreate(t *testing.T) {
	if testStore == nil {
		t.Skip("testStore not initialized — requires DATABASE_URL")
	}

	const count = 500
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < count; i++ {
		trigger := &domain.EventTrigger{
			ID:         fmt.Sprintf("load-create-%d-%d", time.Now().UnixNano(), i),
			EventKey:   fmt.Sprintf("load:create:%d:%d", time.Now().UnixNano(), i),
			ProjectID:  "proj-load-test",
			SourceType: domain.EventSourceJobRun,
			TriggerType: "event",
			Status:     domain.EventTriggerStatusWaiting,
			TimeoutSecs: 3600,
			RequestedAt: time.Now(),
			ExpiresAt:   time.Now().Add(time.Hour),
		}
		if err := testStore.CreateEventTrigger(ctx, trigger); err != nil {
			t.Fatalf("create trigger %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	t.Logf("Created %d triggers in %v (%.0f/sec)", count, elapsed, float64(count)/elapsed.Seconds())
}

// TestEventTriggerLoadSendConcurrent tests concurrent event sends.
func TestEventTriggerLoadSendConcurrent(t *testing.T) {
	if testStore == nil {
		t.Skip("testStore not initialized — requires DATABASE_URL")
	}

	const concurrency = 50
	ctx := context.Background()

	// Pre-create triggers.
	triggers := make([]*domain.EventTrigger, concurrency)
	for i := 0; i < concurrency; i++ {
		triggers[i] = &domain.EventTrigger{
			ID:          fmt.Sprintf("load-send-%d-%d", time.Now().UnixNano(), i),
			EventKey:    fmt.Sprintf("load:send:%d:%d", time.Now().UnixNano(), i),
			ProjectID:   "proj-load-test",
			SourceType:  domain.EventSourceJobRun,
			TriggerType: "event",
			Status:      domain.EventTriggerStatusWaiting,
			TimeoutSecs: 3600,
			RequestedAt: time.Now(),
			ExpiresAt:   time.Now().Add(time.Hour),
		}
		if err := testStore.CreateEventTrigger(ctx, triggers[i]); err != nil {
			t.Fatalf("pre-create trigger %d: %v", i, err)
		}
	}

	var wg sync.WaitGroup
	errs := make([]error, concurrency)
	now := time.Now()
	payload := json.RawMessage(`{"approved":true}`)

	start := time.Now()
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = testStore.UpdateEventTriggerStatus(
				ctx, triggers[idx].ID, domain.EventTriggerStatusReceived, payload, &now, "",
			)
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	failures := 0
	for _, err := range errs {
		if err != nil {
			failures++
		}
	}
	t.Logf("Resolved %d/%d triggers concurrently in %v (%.0f/sec, %d failures)",
		concurrency-failures, concurrency, elapsed, float64(concurrency)/elapsed.Seconds(), failures)

	if failures > 0 {
		t.Errorf("%d/%d concurrent sends failed", failures, concurrency)
	}
}
