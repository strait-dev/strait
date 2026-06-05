package telemetry

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitProfiling_EmptyEndpoint(t *testing.T) {
	shutdown, err := InitProfiling(ProfilingConfig{})
	require.NoError(t,
		err)
	require.NotNil(t, shutdown)

	// Calling shutdown should not panic.
	shutdown()
}

func TestInitProfiling_ShutdownNoPanic(t *testing.T) {
	shutdown, err := InitProfiling(ProfilingConfig{})
	require.NoError(t,
		err)

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
	require.NoError(t,
		err)

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
	require.NoError(t,
		err)

	defer shutdown()

	prevMutex := runtime.SetMutexProfileFraction(0)
	assert.EqualValues(t, 100,
		prevMutex,
	)

	runtime.SetMutexProfileFraction(100)
}

func TestInitProfiling_NonEmptyEndpoint_ReturnsStopFunc(t *testing.T) {
	shutdown, err := InitProfiling(ProfilingConfig{
		Endpoint:    "http://localhost:4040",
		ServiceName: "test-service",
		Environment: "test",
	})
	require.NoError(t,
		err)
	require.NotNil(t, shutdown)

	shutdown()
}
