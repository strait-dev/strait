package telemetry

import (
	"fmt"
	"runtime"

	"github.com/grafana/pyroscope-go"
)

// ProfilingConfig holds settings for continuous profiling via Pyroscope.
type ProfilingConfig struct {
	Endpoint    string
	AuthToken   string
	ServiceName string
	Environment string
}

// InitProfiling starts continuous profiling with Pyroscope. If Endpoint is empty,
// profiling is disabled and a no-op shutdown function is returned.
func InitProfiling(cfg ProfilingConfig) (shutdown func(), err error) {
	if cfg.Endpoint == "" {
		return func() {}, nil
	}

	EnableRuntimeProfiling()

	profiler, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: cfg.ServiceName,
		ServerAddress:   cfg.Endpoint,
		AuthToken:       cfg.AuthToken,
		Tags:            map[string]string{"environment": cfg.Environment},
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("start pyroscope profiler: %w", err)
	}

	return func() {
		_ = profiler.Stop()
	}, nil
}

// EnableRuntimeProfiling enables mutex and block profiling for pprof consumers.
func EnableRuntimeProfiling() {
	// Use moderate sampling rates to limit production overhead (~0.5% CPU).
	// Rate of 5 would record nearly every event; 100000 samples ~1 in 100k ns events.
	runtime.SetMutexProfileFraction(100)
	runtime.SetBlockProfileRate(100000)
}
