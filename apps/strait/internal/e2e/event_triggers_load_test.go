//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventTriggerLoadCreate measures throughput of creating event triggers.
func TestEventTriggerLoadCreate(t *testing.T) {
	if testStore == nil {
		t.Skip("testStore not initialized — requires DATABASE_URL")
	}

	const count = 500
	ctx := context.Background()

	start := time.Now()
	for i := range count {
		trigger := &domain.EventTrigger{
			ID:          fmt.Sprintf("load-create-%d-%d", time.Now().UnixNano(), i),
			EventKey:    fmt.Sprintf("load:create:%d:%d", time.Now().UnixNano(), i),
			ProjectID:   "proj-load-test",
			SourceType:  domain.EventSourceJobRun,
			TriggerType: "event",
			Status:      domain.EventTriggerStatusWaiting,
			TimeoutSecs: 3600,
			RequestedAt: time.Now(),
			ExpiresAt:   time.Now().Add(time.Hour),
		}
		require.NoError(t, testStore.
			CreateEventTrigger(ctx, trigger))

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
	for i := range concurrency {
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
		require.NoError(t, testStore.
			CreateEventTrigger(ctx, triggers[i]))

	}

	var wg conc.WaitGroup
	errs := make([]error, concurrency)
	now := time.Now()
	payload := json.RawMessage(`{"approved":true}`)

	start := time.Now()
	for i := range concurrency {
		idx := i
		wg.Go(func() {
			errs[idx] = testStore.UpdateEventTriggerStatus(
				ctx, triggers[idx].ID, domain.EventTriggerStatusReceived, payload, &now, "",
			)
		})
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
	assert.LessOrEqual(t,

		failures,
		0)

}
