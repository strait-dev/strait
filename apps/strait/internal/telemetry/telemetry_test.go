package telemetry

import (
	"context"
	"strings"
	"testing"
)

// TestInit_NoEndpoint verifies that Init handles empty endpoint gracefully.
func TestInit_NoEndpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shutdown, err := Init(ctx, "test-service", "", "test")

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
	t.Parallel()
	ctx := context.Background()
	// Use a localhost endpoint that won't actually connect
	shutdown, err := Init(ctx, "test-service", "http://localhost:4318", "test")

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
	t.Parallel()
	ctx := context.Background()
	serviceNames := []string{
		"strait",
		"test-service",
		"my-app",
	}

	for _, name := range serviceNames {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			shutdown, err := Init(ctx, name, "", "test")
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

// TestInit_EmptyEnvironment verifies that Init works with an empty environment.
func TestInit_EmptyEnvironment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shutdown, err := Init(ctx, "test-service", "", "")
	if err != nil {
		t.Fatalf("Init with empty environment returned error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Init returned nil shutdown function")
	}
	_ = shutdown(ctx)
}

// TestInit_ShutdownIdempotent verifies that shutdown can be called multiple times.
func TestInit_ShutdownIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shutdown, err := Init(ctx, "test-service", "", "test")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Call shutdown multiple times
	for i := range 3 {
		err := shutdown(ctx)
		if err != nil {
			t.Errorf("shutdown call %d returned error: %v", i+1, err)
		}
	}
}

func TestInit_InvalidURL_ReturnsError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, err := Init(ctx, "test-service", "://bad", "test")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
	if !strings.Contains(err.Error(), "parse otel trace endpoint") {
		t.Errorf("error = %q, want 'parse otel trace endpoint'", err.Error())
	}
}

func TestInit_HttpsScheme(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shutdown, err := Init(ctx, "test-service", "https://localhost:4318", "test")
	if err == nil && shutdown != nil {
		_ = shutdown(ctx)
	}
}

func TestInit_WithEnvironment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shutdown, err := Init(ctx, "test-service", "http://localhost:4318", "production")
	if err == nil && shutdown != nil {
		_ = shutdown(ctx)
	}
}

func TestInitLogBridge_InvalidURL_ReturnsError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger, shutdown, err := InitLogBridge(ctx, "test-service", "://bad", "test")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
	if logger != nil {
		t.Error("expected nil logger on error")
	}
	if shutdown != nil {
		t.Error("expected nil shutdown on error")
	}
}

func TestInitLogBridge_HttpsScheme(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger, shutdown, err := InitLogBridge(ctx, "test-service", "https://localhost:4318", "test")
	if err != nil {
		t.Fatalf("InitLogBridge() error = %v", err)
	}
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	_ = shutdown(ctx)
}
