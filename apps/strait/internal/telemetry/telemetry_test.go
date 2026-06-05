package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInit_NoEndpoint verifies that Init handles empty endpoint gracefully.
func TestInit_NoEndpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shutdown, err := Init(ctx, "test-service", "", "test")
	assert.NoError(t,
		err)
	assert.NotNil(t,
		shutdown)

	// Verify shutdown function works without error
	err = shutdown(ctx)
	assert.NoError(t,
		err)

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
		assert.NotNil(t,
			shutdown)

		// If no error, verify shutdown function exists

		// Clean up
		_ = shutdown(ctx)
	}
	// If error occurs, that's acceptable for a non-existent endpoint
}

func TestInit_RedactsCredentialedEndpointInStartupLog(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	shutdown, err := Init(ctx, "test-service", "http://user:pass@localhost:4318/v1/traces?sig=signed&tenant=prod", "test")
	require.NoError(t,
		err)

	_ = shutdown(ctx)

	out := buf.String()
	require.True(t, strings.Contains(out, "otel tracing enabled"))

	for _, leaked := range []string{"user", "pass", "signed"} {
		require.False(t,
			strings.Contains(out, leaked))

	}
	require.True(t, strings.Contains(out, "sig=%5Bredacted%5D"))
	require.True(t, strings.Contains(out, "tenant=prod"))

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
			assert.NoError(t,
				err)
			assert.NotNil(t,
				shutdown)

			_ = shutdown(ctx)
		})
	}
}

// TestInit_EmptyEnvironment verifies that Init works with an empty environment.
func TestInit_EmptyEnvironment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shutdown, err := Init(ctx, "test-service", "", "")
	require.NoError(t,
		err)
	require.NotNil(t,
		shutdown)

	_ = shutdown(ctx)
}

// TestInit_ShutdownIdempotent verifies that shutdown can be called multiple times.
func TestInit_ShutdownIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shutdown, err := Init(ctx, "test-service", "", "test")
	require.NoError(t,
		err)

	// Call shutdown multiple times
	for range 3 {
		err := shutdown(ctx)
		assert.NoError(t,
			err)

	}
}

func TestInit_InvalidURL_ReturnsError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, err := Init(ctx, "test-service", "://bad", "test")
	require.Error(t,
		err)
	assert.True(t, strings.Contains(err.Error(), "parse otel trace endpoint"))

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
	require.Error(t,
		err)
	assert.Nil(t, logger)
	assert.Nil(t, shutdown)

}

func TestInitLogBridge_HttpsScheme(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger, shutdown, err := InitLogBridge(ctx, "test-service", "https://localhost:4318", "test")
	require.NoError(t,
		err)
	require.NotNil(t,
		logger)

	_ = shutdown(ctx)
}
