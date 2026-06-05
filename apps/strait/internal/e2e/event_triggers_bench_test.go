//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// BenchmarkListExpiredEventTriggers seeds N waiting triggers with expired
// timestamps and measures the reaper query performance.
func BenchmarkListExpiredEventTriggers(b *testing.B) {
	ctx := context.Background()

	// Seed 1000 expired waiting triggers.
	for i := range 1000 {
		trigger := &domain.EventTrigger{
			ID:          fmt.Sprintf("bench-exp-%d", i),
			EventKey:    fmt.Sprintf("bench:expired:%d", i),
			ProjectID:   "bench-proj",
			SourceType:  domain.EventSourceWorkflowStep,
			TriggerType: domain.TriggerTypeEvent,
			Status:      domain.EventTriggerStatusWaiting,
			TimeoutSecs: 1,
			RequestedAt: time.Now().Add(-10 * time.Minute),
			ExpiresAt:   time.Now().Add(-5 * time.Minute),
		}
		if err := testStore.CreateEventTrigger(ctx, trigger); err != nil {
			b.Fatalf("seed trigger %d: %v", i, err)
		}
	}

	b.ResetTimer()
	for range b.N {
		triggers, err := testStore.ListExpiredEventTriggers(ctx)
		if err != nil {
			b.Fatalf("ListExpiredEventTriggers: %v", err)
		}
		_ = triggers
	}
}

// BenchmarkListByKeyPrefix seeds triggers with a common prefix and measures
// the prefix LIKE query performance with text_pattern_ops index.
func BenchmarkListByKeyPrefix(b *testing.B) {
	ctx := context.Background()

	// Seed 1000 waiting triggers with common prefix.
	for i := range 1000 {
		trigger := &domain.EventTrigger{
			ID:          fmt.Sprintf("bench-pfx-%d", i),
			EventKey:    fmt.Sprintf("bench:prefix:%d", i),
			ProjectID:   "bench-proj",
			SourceType:  domain.EventSourceWorkflowStep,
			TriggerType: domain.TriggerTypeEvent,
			Status:      domain.EventTriggerStatusWaiting,
			TimeoutSecs: 3600,
			RequestedAt: time.Now(),
			ExpiresAt:   time.Now().Add(1 * time.Hour),
		}
		if err := testStore.CreateEventTrigger(ctx, trigger); err != nil {
			b.Fatalf("seed trigger %d: %v", i, err)
		}
	}

	b.ResetTimer()
	for range b.N {
		triggers, err := testStore.ListEventTriggersByKeyPrefix(ctx, "bench-proj", "bench:prefix:")
		if err != nil {
			b.Fatalf("ListEventTriggersByKeyPrefix: %v", err)
		}
		_ = triggers
	}
}

// TestEventTriggerBench_SeedAndQuery verifies store methods work with bench data.
func TestEventTriggerBench_SeedAndQuery(t *testing.T) {
	ctx := context.Background()

	trigger := &domain.EventTrigger{
		ID:             "bench-verify-1",
		EventKey:       "bench:verify:1",
		ProjectID:      "bench-proj",
		SourceType:     domain.EventSourceWorkflowStep,
		TriggerType:    domain.TriggerTypeEvent,
		Status:         domain.EventTriggerStatusWaiting,
		TimeoutSecs:    1,
		RequestPayload: json.RawMessage(`{"test": true}`),
		RequestedAt:    time.Now().Add(-10 * time.Minute),
		ExpiresAt:      time.Now().Add(-5 * time.Minute),
	}
	require.NoError(t, testStore.
		CreateEventTrigger(ctx, trigger))

	expired, err := testStore.ListExpiredEventTriggers(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, expired)

}
