package loadtest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCaptureRuntimeProfiles_WritesRequestedArtifacts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	artifacts, err := CaptureRuntimeProfiles(context.Background(), RuntimeProfileCapture{
		Name:  "Trigger Storm",
		Dir:   dir,
		Kinds: []RuntimeProfileKind{RuntimeProfileHeap, RuntimeProfileGoroutine},
		Work: func(context.Context) error {
			_ = make([]byte, 1024)
			return nil
		},
	})
	require.NoError(t,
		err)
	require.Len(t, artifacts,

		2)

	for _, artifact := range artifacts {
		require.Equal(t, "Trigger Storm",

			artifact.
				Name)
		require.NotEmpty(t,
			artifact.Kind)
		require.Equal(t, dir,
			filepath.
				Dir(artifact.
					Path))

		info, statErr := os.Stat(artifact.Path)
		require.NoError(t,
			statErr,
		)
		require.NotEqual(t,
			0, info.
				Size())
	}
}

func TestCaptureRuntimeProfiles_DefaultKinds(t *testing.T) {
	artifacts, err := CaptureRuntimeProfiles(context.Background(), RuntimeProfileCapture{
		Name:        "default profiles",
		Dir:         t.TempDir(),
		CPUDuration: time.Nanosecond,
	})
	require.NoError(t, err)
	require.Len(t, artifacts, 6)

	kinds := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		kinds[artifact.Kind] = struct{}{}
		info, statErr := os.Stat(artifact.Path)
		require.NoError(t, statErr)
		require.NotZero(t, info.Size())
	}

	for _, kind := range []RuntimeProfileKind{
		RuntimeProfileCPU,
		RuntimeProfileHeap,
		RuntimeProfileGoroutine,
		RuntimeProfileBlock,
		RuntimeProfileMutex,
		RuntimeProfileTrace,
	} {
		require.Contains(t, kinds, string(kind))
	}
}

func TestCaptureRuntimeProfiles_CPUWorkError(t *testing.T) {
	workErr := errors.New("load generator failed")
	_, err := CaptureRuntimeProfiles(context.Background(), RuntimeProfileCapture{
		Name:        "cpu failure",
		Dir:         t.TempDir(),
		Kinds:       []RuntimeProfileKind{RuntimeProfileCPU},
		CPUDuration: time.Hour,
		Work: func(context.Context) error {
			return workErr
		},
	})
	require.ErrorIs(t, err, workErr)
	require.ErrorContains(t, err, "run cpu profile work")
}

func TestCaptureRuntimeProfiles_CPUContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := CaptureRuntimeProfiles(ctx, RuntimeProfileCapture{
		Name:        "cpu canceled",
		Dir:         t.TempDir(),
		Kinds:       []RuntimeProfileKind{RuntimeProfileCPU},
		CPUDuration: time.Hour,
	})
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorContains(t, err, "run cpu profile work")
}

func TestCaptureRuntimeProfiles_TraceWorkError(t *testing.T) {
	workErr := errors.New("trace workload failed")
	_, err := CaptureRuntimeProfiles(context.Background(), RuntimeProfileCapture{
		Name:  "trace failure",
		Dir:   t.TempDir(),
		Kinds: []RuntimeProfileKind{RuntimeProfileTrace},
		Work: func(context.Context) error {
			return workErr
		},
	})
	require.ErrorIs(t, err, workErr)
	require.ErrorContains(t, err, "run trace profile work")
}

func TestCaptureRuntimeProfiles_ValidatesInput(t *testing.T) {
	t.Parallel()

	_, err := CaptureRuntimeProfiles(context.Background(), RuntimeProfileCapture{})
	require.Error(t, err)
	_, err = CaptureRuntimeProfiles(context.Background(), RuntimeProfileCapture{Name: "x"})
	require.Error(t, err)
	if _, err := CaptureRuntimeProfiles(context.Background(), RuntimeProfileCapture{
		Name:  "x",
		Dir:   t.TempDir(),
		Kinds: []RuntimeProfileKind{"unknown"},
	}); err == nil {
		require.Fail(t, "CaptureRuntimeProfiles() error = nil, want unsupported profile kind")
	}
}

func TestRunProfileWork_NilWork(t *testing.T) {
	t.Parallel()

	require.NoError(t, runProfileWork(context.Background(), RuntimeProfileCapture{}))
}

func TestSafeProfileFilename(t *testing.T) {
	t.Parallel()

	require.Equal(t, "trigger-storm---core-api.cpu.pprof", safeProfileFilename("Trigger Storm / Core API", string(RuntimeProfileCPU)))
	require.Equal(t, "profile.heap.pprof", safeProfileFilename("  ", string(RuntimeProfileHeap)))
	require.Equal(t, "az09---.heap.pprof", safeProfileFilename("az09`{/", string(RuntimeProfileHeap)))
	require.Equal(t, "run-123.heap.pprof", safeProfileFilename("Run 123", string(RuntimeProfileHeap)))
	require.Equal(t, "trace.trace.out", safeProfileFilename("trace", string(RuntimeProfileTrace)))
}
