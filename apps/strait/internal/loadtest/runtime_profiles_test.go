package loadtest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
	if err != nil {
		t.Fatalf("CaptureRuntimeProfiles() error = %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("artifacts len = %d, want 2", len(artifacts))
	}

	for _, artifact := range artifacts {
		if artifact.Name != "Trigger Storm" {
			t.Fatalf("artifact name = %q, want Trigger Storm", artifact.Name)
		}
		if artifact.Kind == "" {
			t.Fatal("artifact kind is empty")
		}
		if filepath.Dir(artifact.Path) != dir {
			t.Fatalf("artifact path = %q, want under %q", artifact.Path, dir)
		}
		info, statErr := os.Stat(artifact.Path)
		if statErr != nil {
			t.Fatalf("stat artifact %q: %v", artifact.Path, statErr)
		}
		if info.Size() == 0 {
			t.Fatalf("artifact %q is empty", artifact.Path)
		}
	}
}

func TestCaptureRuntimeProfiles_ValidatesInput(t *testing.T) {
	t.Parallel()

	if _, err := CaptureRuntimeProfiles(context.Background(), RuntimeProfileCapture{}); err == nil {
		t.Fatal("CaptureRuntimeProfiles() error = nil, want name validation")
	}
	if _, err := CaptureRuntimeProfiles(context.Background(), RuntimeProfileCapture{Name: "x"}); err == nil {
		t.Fatal("CaptureRuntimeProfiles() error = nil, want dir validation")
	}
	if _, err := CaptureRuntimeProfiles(context.Background(), RuntimeProfileCapture{
		Name:  "x",
		Dir:   t.TempDir(),
		Kinds: []RuntimeProfileKind{"unknown"},
	}); err == nil {
		t.Fatal("CaptureRuntimeProfiles() error = nil, want unsupported profile kind")
	}
}

func TestSafeProfileFilename(t *testing.T) {
	t.Parallel()

	if got := safeProfileFilename("Trigger Storm / Core API", string(RuntimeProfileCPU)); got != "trigger-storm---core-api.cpu.pprof" {
		t.Fatalf("safeProfileFilename() = %q", got)
	}
	if got := safeProfileFilename("trace", string(RuntimeProfileTrace)); got != "trace.trace.out" {
		t.Fatalf("safeProfileFilename(trace) = %q", got)
	}
}
