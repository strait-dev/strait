package worker

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// goleak.VerifyTestMain is not used because this package includes integration
	// tests (worker_integration_test.go) that create testcontainer instances.
	// Testcontainer goroutines (Reaper, Redis connections, HTTP pool) are detected
	// as leaks by goleak, causing os.Exit(1) which tears down the Redis client
	// mid-flight, failing all subsequent integration tests with "client is closed".
	os.Exit(m.Run())
}
