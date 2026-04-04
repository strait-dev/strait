package cdc

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// goleak.VerifyTestMain is intentionally not used here because integration
	// tests in this package create testcontainer instances (Redis, Postgres)
	// whose background goroutines (Reaper, HTTP connections, connection pools)
	// outlive the test suite and cause false-positive leak detections.
	os.Exit(m.Run())
}
