//go:build integration

package queue_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"

	"github.com/jackc/pgx/v5"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

func TestNewQueueNotifier(t *testing.T) {
	require.False(t, testDB ==
		nil ||
		testDB.ConnStr ==
			"",
	)

	notifier := queue.NewQueueNotifier(testDB.ConnStr, slog.Default())
	require.NotNil(t, notifier)

	ch := notifier.Wake()
	require.NotNil(t, ch)

}

func TestNewQueueNotifier_NilLogger(t *testing.T) {
	require.False(t, testDB ==
		nil ||
		testDB.ConnStr ==
			"",
	)

	notifier := queue.NewQueueNotifier(testDB.ConnStr, nil)
	require.NotNil(t, notifier)

}

func TestQueueNotifier_WakeReceivesNotification(t *testing.T) {
	require.False(t, testDB ==
		nil ||
		testDB.ConnStr ==
			"",
	)

	notifier := queue.NewQueueNotifier(testDB.ConnStr, slog.Default())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go notifier.Run(ctx)

	// Give the listener time to establish.
	time.Sleep(500 * time.Millisecond)

	// Send a NOTIFY on the queue wake channel via a separate connection.
	conn, err := pgx.Connect(ctx, testDB.ConnStr)
	require.NoError(t, err)

	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, fmt.Sprintf("NOTIFY %s", queue.QueueWakeChannel)); err != nil {
		require.Failf(t, "test failure",

			"NOTIFY error: %v", err)
	}

	select {
	case <-notifier.Wake():
		// Received the wake signal.
	case <-time.After(5 * time.Second):
		require.Fail(t, "timed out waiting for wake signal after NOTIFY")
	}
}

func TestQueueNotifier_RunStopsOnContextCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	require.False(t, testDB ==
		nil ||
		testDB.ConnStr ==
			"",
	)

	notifier := queue.NewQueueNotifier(testDB.ConnStr, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	concWG.Go(func() {
		notifier.Run(ctx)
		close(done)
	})

	// Give the listener time to start.
	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Run exited cleanly.
	case <-time.After(5 * time.Second):
		require.Fail(t, "Run did not stop after context cancellation")
	}
}

func TestQueueNotifier_MultipleWakesCoalesced(t *testing.T) {
	require.False(t, testDB ==
		nil ||
		testDB.ConnStr ==
			"",
	)

	notifier := queue.NewQueueNotifier(testDB.ConnStr, slog.Default())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go notifier.Run(ctx)

	// Give the listener time to establish.
	time.Sleep(500 * time.Millisecond)

	conn, err := pgx.Connect(ctx, testDB.ConnStr)
	require.NoError(t, err)

	defer conn.Close(context.Background())

	// Send multiple NOTIFY signals rapidly without draining the channel.
	for range 10 {
		if _, err := conn.Exec(ctx, fmt.Sprintf("NOTIFY %s", queue.QueueWakeChannel)); err != nil {
			require.Failf(t, "test failure",

				"NOTIFY error: %v", err)
		}
	}

	// Allow some time for notifications to arrive.
	time.Sleep(500 * time.Millisecond)

	// The wake channel has capacity 1, so we should get exactly 1 signal
	// regardless of how many notifications were sent.
	select {
	case <-notifier.Wake():
		// First wake received.
	case <-time.After(3 * time.Second):
		require.Fail(t, "timed out waiting for first wake")
	}

	// The channel should be empty now (coalesced).
	select {
	case <-notifier.Wake():
		// There may be at most one more buffered signal that arrived
		// after we drained the first. A third would indicate no coalescing.
		select {
		case <-notifier.Wake():
			require.Fail(t, "wake channel received more than 2 signals from 10 notifications; coalescing is broken")
		case <-time.After(200 * time.Millisecond):
			// At most 2 signals from 10 notifications -- coalescing works.
		}
	case <-time.After(200 * time.Millisecond):
		// Only 1 signal from 10 notifications -- coalescing works.
	}
}

func TestQueueNotifier_RunReconnectsAfterBadURL(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()

	// Use an invalid database URL to trigger reconnect logic.
	notifier := queue.NewQueueNotifier("postgres://invalid:5432/nope", slog.Default())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan struct{})
	concWG.Go(func() {
		notifier.Run(ctx)
		close(done)
	})

	// Run should eventually exit when context is canceled, even though
	// it keeps failing to connect.
	select {
	case <-done:
		// Exited cleanly after context timeout.
	case <-time.After(5 * time.Second):
		require.Fail(t, "Run did not stop after context cancellation with bad URL")
	}
}

func TestQueueNotifyTrigger_SingleEnqueueEmitsOneWake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)
	job := mustCreateJob(t, ctx, st, "project-notify-single")
	listener := listenQueueWake(t, ctx)
	defer listener.Close(context.Background())

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	require.NoError(t, q.Enqueue(ctx,
		run))
	require.EqualValues(t, 1, countQueueWakeNotifications(t, ctx,
		listener,
	))

}

func TestQueueNotifyTrigger_BatchEnqueueEmitsOneWake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)
	job := mustCreateJob(t, ctx, st, "project-notify-batch")
	listener := listenQueueWake(t, ctx)
	defer listener.Close(context.Background())

	runs := make([]*domain.JobRun, 10)
	for i := range runs {
		runs[i] = &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	}
	inserted, err := q.EnqueueBatch(ctx, runs)
	require.NoError(t, err)
	require.Equal(t, int64(
		len(runs)),
		inserted)
	require.EqualValues(t, 1, countQueueWakeNotifications(t, ctx,
		listener,
	))

}

func TestQueueNotifyTrigger_StatusUpdateQueuedEmitsOneWakePerStatement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)
	job := mustCreateJob(t, ctx, st, "project-notify-update-queued")

	future := time.Now().Add(time.Hour)
	ids := make([]string, 3)
	for i := range ids {
		ids[i] = newID()
		run := &domain.JobRun{
			ID:          ids[i],
			JobID:       job.ID,
			ProjectID:   job.ProjectID,
			ScheduledAt: &future,
		}
		require.NoError(t, q.Enqueue(ctx,
			run))

	}

	listener := listenQueueWake(t, ctx)
	defer listener.Close(context.Background())
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_runs
		SET status = 'queued', scheduled_at = NULL
		WHERE id = ANY($1)
	`, ids); err != nil {
		require.Failf(t, "test failure",

			"update delayed runs to queued: %v", err)
	}
	require.EqualValues(t, 1, countQueueWakeNotifications(t, ctx,
		listener,
	))

}

func TestQueueNotifyTrigger_NonQueuedTransitionEmitsNoWake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)
	job := mustCreateJob(t, ctx, st, "project-notify-nonqueued")
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	require.NoError(t, q.Enqueue(ctx,
		run))

	listener := listenQueueWake(t, ctx)
	defer listener.Close(context.Background())
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE job_runs
		SET status = 'executing', started_at = NOW()
		WHERE id = $1
	`, run.ID); err != nil {
		require.Failf(t, "test failure",

			"update run to executing: %v", err)
	}
	require.EqualValues(t, 0, countQueueWakeNotifications(t, ctx,
		listener,
	))

}

func listenQueueWake(t *testing.T, ctx context.Context) *pgx.Conn {
	t.Helper()
	listener, err := pgx.Connect(ctx, testDB.ConnStr)
	require.NoError(t, err)

	if _, err := listener.Exec(ctx, "LISTEN "+queue.QueueWakeChannel); err != nil {
		_ = listener.Close(context.Background())
		require.Failf(t, "test failure",

			"listen queue wake: %v", err)
	}
	return listener
}

func countQueueWakeNotifications(t *testing.T, ctx context.Context, listener *pgx.Conn) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	count := 0
	for {
		wait := 250 * time.Millisecond
		if count == 0 {
			wait = 2 * time.Second
		}
		if remaining := time.Until(deadline); remaining < wait {
			wait = remaining
		}
		if wait <= 0 {
			return count
		}
		waitCtx, cancel := context.WithTimeout(ctx, wait)
		note, err := listener.WaitForNotification(waitCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return count
			}
			require.Failf(t, "test failure",

				"wait for queue wake notification: %v", err)
		}
		if note != nil && note.Channel == queue.QueueWakeChannel {
			count++
		}
	}
}
