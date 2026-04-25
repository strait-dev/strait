package scheduler

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSchedulerStop_ReturnsAfterComponentTimeout(t *testing.T) {
	store := &mockSchedulerStore{
		cron:   &mockCronStore{},
		poller: &mockPollerStore{},
		reaper: &mockReaperStore{},
		index:  &mockIndexMaintenanceStore{},
	}
	s := New(context.Background(), testSchedulerConfig(), store, &mockQueue{}, nil, nil)
	s.componentShutdownTimeout = 50 * time.Millisecond

	blocked := make(chan struct{})
	s.tracker.track(&s.wg, "stuck_component", func() {
		<-blocked
	})

	start := time.Now()
	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Stop did not return after the configured timeout")
	}

	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("Stop elapsed=%v, want bounded return well under 250ms", elapsed)
	}

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
	s.tracker.track(&s.wg, "clean_component", func() {
		<-release
	})
	time.AfterFunc(35*time.Millisecond, func() {
		close(release)
	})

	start := time.Now()
	s.Stop()
	elapsed := time.Since(start)

	if elapsed < 20*time.Millisecond {
		t.Fatalf("Stop elapsed=%v, want it to wait for the component to exit cleanly", elapsed)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("Stop elapsed=%v, want clean completion before timeout", elapsed)
	}
	if strings.Contains(logs.String(), "scheduler component exceeded shutdown deadline") {
		t.Fatalf("unexpected shutdown timeout log: %s", logs.String())
	}
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
	s.tracker.track(&s.wg, "stuck_a", func() { <-blocked })
	s.tracker.track(&s.wg, "stuck_b", func() { <-blocked })

	s.Stop()

	output := logs.String()
	if !strings.Contains(output, "component=stuck_a") {
		t.Fatalf("expected timeout log for stuck_a, got %s", output)
	}
	if !strings.Contains(output, "component=stuck_b") {
		t.Fatalf("expected timeout log for stuck_b, got %s", output)
	}
	if !strings.Contains(output, "timed_out_components=2") {
		t.Fatalf("expected timed_out_components=2 in logs, got %s", output)
	}

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
