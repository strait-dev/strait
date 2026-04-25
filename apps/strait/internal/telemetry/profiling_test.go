package telemetry

import (
	"runtime"
	"testing"
)

func TestInitProfiling_EmptyEndpoint(t *testing.T) {
	shutdown, err := InitProfiling(ProfilingConfig{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function")
	}
	// Calling shutdown should not panic.
	shutdown()
}

func TestInitProfiling_ShutdownNoPanic(t *testing.T) {
	shutdown, err := InitProfiling(ProfilingConfig{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	// Calling shutdown multiple times should not panic.
	shutdown()
	shutdown()
	shutdown()
}

func TestInitProfiling_ConfigFields(t *testing.T) {
	// Verify that all config fields are accepted without error when endpoint
	// is empty (no actual connection is made).
	shutdown, err := InitProfiling(ProfilingConfig{
		Endpoint:    "",
		AuthToken:   "test-token",
		ServiceName: "test-service",
		Environment: "test",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	shutdown()
}

func TestInitProfiling_NonEmptyEndpoint_SetsProfileRates(t *testing.T) {
	runtime.SetMutexProfileFraction(0)
	runtime.SetBlockProfileRate(0)

	shutdown, err := InitProfiling(ProfilingConfig{
		Endpoint:    "http://localhost:4040",
		ServiceName: "test-service",
		Environment: "test",
	})
	if err != nil {
		t.Fatalf("InitProfiling error = %v", err)
	}
	defer shutdown()

	prevMutex := runtime.SetMutexProfileFraction(0)
	if prevMutex != 100 {
		t.Errorf("expected SetMutexProfileFraction was set to 100, got previous=%d", prevMutex)
	}
	runtime.SetMutexProfileFraction(100)
}

func TestInitProfiling_NonEmptyEndpoint_ReturnsStopFunc(t *testing.T) {
	shutdown, err := InitProfiling(ProfilingConfig{
		Endpoint:    "http://localhost:4040",
		ServiceName: "test-service",
		Environment: "test",
	})
	if err != nil {
		t.Fatalf("InitProfiling error = %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function")
	}
	shutdown()
}
