package scheduler

import (
	"testing"
	"time"
)

func TestWithReaperAdvisoryLocker_WiresReaper(t *testing.T) {
	t.Parallel()

	reaper := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, true, nil)
	sched := &Scheduler{reaper: reaper}
	locker := &mockAdvisoryLocker{}

	WithReaperAdvisoryLocker(locker)(sched)

	if sched.reaper.advisoryLocker != locker {
		t.Fatal("reaper advisory locker was not wired")
	}
}
