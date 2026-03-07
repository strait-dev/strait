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
	)
}
