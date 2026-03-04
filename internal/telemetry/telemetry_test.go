package telemetry

import (
	"context"
	"testing"
)

// TestInit_NoEndpoint verifies that Init handles empty endpoint gracefully.
func TestInit_NoEndpoint(t *testing.T) {
	ctx := context.Background()
	shutdown, err := Init(ctx, "test-service", "")

	if err != nil {
		t.Errorf("Init with empty endpoint returned error: %v", err)
	}

	if shutdown == nil {
		t.Error("Init returned nil shutdown function")
	}

	// Verify shutdown function works without error
	err = shutdown(ctx)
	if err != nil {
		t.Errorf("shutdown() returned error: %v", err)
	}
}

// TestInit_WithEndpoint verifies that Init can be called with a valid endpoint.
// Note: This test does not actually connect to an OTLP endpoint.
func TestInit_WithEndpoint(t *testing.T) {
	ctx := context.Background()
	// Use a localhost endpoint that won't actually connect
	shutdown, err := Init(ctx, "test-service", "http://localhost:4318")

	// We expect an error because the endpoint doesn't exist, but the function
	// should attempt to create the exporter
	if err == nil {
		// If no error, verify shutdown function exists
		if shutdown == nil {
			t.Error("Init returned nil shutdown function")
		}
		// Clean up
		_ = shutdown(ctx)
	}
	// If error occurs, that's acceptable for a non-existent endpoint
}

// TestInit_ServiceName verifies that Init accepts a service name.
func TestInit_ServiceName(t *testing.T) {
	ctx := context.Background()
	serviceNames := []string{
		"orchestrator",
		"test-service",
		"my-app",
	}

	for _, name := range serviceNames {
		t.Run(name, func(t *testing.T) {
			shutdown, err := Init(ctx, name, "")
			if err != nil {
				t.Errorf("Init with service name %q returned error: %v", name, err)
			}
			if shutdown == nil {
				t.Error("Init returned nil shutdown function")
			}
			_ = shutdown(ctx)
		})
	}
}

// TestInit_ShutdownIdempotent verifies that shutdown can be called multiple times.
func TestInit_ShutdownIdempotent(t *testing.T) {
	ctx := context.Background()
	shutdown, err := Init(ctx, "test-service", "")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Call shutdown multiple times
	for i := 0; i < 3; i++ {
		err := shutdown(ctx)
		if err != nil {
			t.Errorf("shutdown call %d returned error: %v", i+1, err)
		}
	}
}
