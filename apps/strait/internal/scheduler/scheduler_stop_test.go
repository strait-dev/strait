package scheduler

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

func TestSchedulerStop_ReturnsAfterComponentTimeout(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	store := &mockSchedulerStore{
		cron:   &mockCronStore{},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
		index:  &mockIndexMaintenanceStore{},
	}
	s := New(context.Background(), testSchedulerConfig(), store, &mockQueue{}, nil, nil)
	s.componentShutdownTimeout = 50 * time.Millisecond

	blocked := make(chan struct{})
	s.tracker.track(context.Background(), &s.wg, "stuck_component", func(context.Context) {
		<-blocked
	})

	start := time.Now()
	done := make(chan struct{})
	concWG.Go(func() {
		s.Stop()
		close(done)
	})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		require.Fail(t, "Stop did not return after the configured timeout")
	}
	require.LessOrEqual(t, time.
		Since(start),
		250*time.
			Millisecond,
	)

	close(blocked)
	s.wg.Wait()
}

func TestSchedulerStop_NoTimeoutWhenComponentsExitCleanly(t *testing.T) {
	store := &mockSchedulerStore{
		cron:   &mockCronStore{},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
		index:  &mockIndexMaintenanceStore{},
	}
	s := New(context.Background(), testSchedulerConfig(), store, &mockQueue{}, nil, nil)
	s.componentShutdownTimeout = 200 * time.Millisecond

	var logs bytes.Buffer
	restore := setDefaultTestLogger(&logs)
	defer restore()

	release := make(chan struct{})
	s.tracker.track(context.Background(), &s.wg, "clean_component", func(context.Context) {
		<-release
	})
	time.AfterFunc(35*time.Millisecond, func() {
		close(release)
	})

	start := time.Now()
	s.Stop()
	elapsed := time.Since(start)
	require.GreaterOrEqual(t, elapsed,
		20*
			time.Millisecond,
	)
	require.LessOrEqual(t, elapsed,
		250*time.
			Millisecond,
	)
	require.False(t,
		strings.Contains(logs.String(), "scheduler component exceeded shutdown deadline"))

}

func TestSchedulerStop_ReportsTimedOutComponentCount(t *testing.T) {
	store := &mockSchedulerStore{
		cron:   &mockCronStore{},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
		index:  &mockIndexMaintenanceStore{},
	}
	s := New(context.Background(), testSchedulerConfig(), store, &mockQueue{}, nil, nil)
	s.componentShutdownTimeout = 40 * time.Millisecond

	var logs bytes.Buffer
	restore := setDefaultTestLogger(&logs)
	defer restore()

	blocked := make(chan struct{})
	s.tracker.track(context.Background(), &s.wg, "stuck_a", func(context.Context) { <-blocked })
	s.tracker.track(context.Background(), &s.wg, "stuck_b", func(context.Context) { <-blocked })

	s.Stop()

	output := logs.String()
	require.True(t, strings.Contains(output,
		"component=stuck_a",
	))
	require.True(t, strings.Contains(output,
		"component=stuck_b",
	))
	require.True(t, strings.Contains(output,
		"timed_out_components=2",
	))

	close(blocked)
	s.wg.Wait()
}

func setDefaultTestLogger(buf *bytes.Buffer) func() {
	prev := slog.Default()
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
	return func() {
		slog.SetDefault(prev)
	}
}
