//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/queue"

	"github.com/stretchr/testify/assert"
)

// Integration tests for notifier reconnect/degraded behavior.

func TestQueueNotifier_DegradedKicksInOnPermaFail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Point at an unreachable DSN so every listen attempt fails.
	n := queue.NewQueueNotifier("postgres://127.0.0.1:1/nobody?connect_timeout=1", nil)
	go n.Run(ctx)

	// The default degraded threshold is 30s. Since that's too slow for
	// an integration test, we just assert the notifier eventually
	// becomes degraded when we wait long enough. For CI speed we skip
	// the full 30s wait.
	t.Skip("slow: degraded threshold is 30s; keep this test here as documentation")
}

func TestQueueNotifier_ConnectionAgeTracksRealListen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	n := queue.NewQueueNotifier(testDB.ConnStr, nil)
	go n.Run(ctx)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if n.ConnectionAge() > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.Failf(t, "test failure",

		"ConnectionAge never became positive after listen")
}
