package worker

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// TestPool_Shutdown_RespectsContext intentionally leaves goroutines
		// running (simulates stuck work that outlives context-canceled shutdown).
		goleak.IgnoreTopFunction("time.Sleep"),
		goleak.IgnoreTopFunction("sync.runtime_SemacquireWaitGroup"),
		goleak.IgnoreTopFunction("github.com/alitto/pond/v2/internal/future.(*Future).Err"),
		// Otter cache background goroutines (timer + queue processor).
		goleak.IgnoreTopFunction("github.com/maypok86/otter/internal/unixtime.startTimer.func1"),
		goleak.IgnoreAnyFunction("github.com/maypok86/otter/internal/queue.(*Growable[...]).Pop"),
		// Integration test harness goroutines.
		goleak.IgnoreTopFunction("github.com/testcontainers/testcontainers-go.(*Reaper).connect.func1"),
		goleak.IgnoreTopFunction("github.com/jackc/pgx/v5/pgxpool.(*Pool).backgroundHealthCheck"),
	)
}
