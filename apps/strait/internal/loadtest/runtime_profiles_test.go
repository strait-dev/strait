package loadtest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
		require.NotEqual(t,
			"",
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

func TestSafeProfileFilename(t *testing.T) {
	t.Parallel()

	require.Equal(t, "trigger-storm---core-api.cpu.pprof", safeProfileFilename("Trigger Storm / Core API", string(RuntimeProfileCPU)))
	require.Equal(t, "trace.trace.out", safeProfileFilename("trace", string(RuntimeProfileTrace)))
}
