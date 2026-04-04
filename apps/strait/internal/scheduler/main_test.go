package scheduler

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// Scheduler tests use sleep in background goroutines for timing.
		goleak.IgnoreTopFunction("time.Sleep"),
		// Otter cache background goroutines (timer + queue processor).
		goleak.IgnoreTopFunction("github.com/maypok86/otter/internal/unixtime.startTimer.func1"),
		goleak.IgnoreAnyFunction("github.com/maypok86/otter/internal/queue.(*Growable[...]).Pop"),
		goleak.IgnoreAnyFunction("github.com/maypok86/otter/internal/core.(*Cache[...]).process"),
		// Integration tests use testcontainers (Postgres/Redis) with sync.Once
		// lazy init. The containers are cleaned by the Reaper at process exit,
		// but their background goroutines are still running when goleak checks.
		goleak.IgnoreTopFunction("github.com/testcontainers/testcontainers-go.(*Reaper).connect.func1"),
		goleak.IgnoreTopFunction("github.com/jackc/pgx/v5/pgxpool.(*Pool).backgroundHealthCheck"),
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)
}
