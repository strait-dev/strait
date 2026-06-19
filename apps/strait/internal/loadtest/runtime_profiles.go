package loadtest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"time"
)

// RuntimeProfileKind names a Go runtime artifact captured by the perf v2
// baseline harness.
type RuntimeProfileKind string

const (
	RuntimeProfileCPU       RuntimeProfileKind = "cpu"
	RuntimeProfileHeap      RuntimeProfileKind = "heap"
	RuntimeProfileGoroutine RuntimeProfileKind = "goroutine"
	RuntimeProfileBlock     RuntimeProfileKind = "block"
	RuntimeProfileMutex     RuntimeProfileKind = "mutex"
	RuntimeProfileTrace     RuntimeProfileKind = "trace"
)

// RuntimeProfileCapture describes one runtime profile capture window.
type RuntimeProfileCapture struct {
	Name        string
	Dir         string
	Kinds       []RuntimeProfileKind
	CPUDuration time.Duration
	Work        func(context.Context) error
}

// CaptureRuntimeProfiles writes requested pprof/trace artifacts and returns
// report entries that can be embedded in PerformanceBaselineReport.
func CaptureRuntimeProfiles(ctx context.Context, capture RuntimeProfileCapture) ([]ProfileArtifact, error) {
	if capture.Name == "" {
		return nil, fmt.Errorf("capture runtime profiles: name is required")
	}
	if capture.Dir == "" {
		return nil, fmt.Errorf("capture runtime profiles: dir is required")
	}
	if len(capture.Kinds) == 0 {
		capture.Kinds = []RuntimeProfileKind{
			RuntimeProfileCPU,
			RuntimeProfileHeap,
			RuntimeProfileGoroutine,
			RuntimeProfileBlock,
			RuntimeProfileMutex,
			RuntimeProfileTrace,
		}
	}
	if capture.CPUDuration <= 0 {
		capture.CPUDuration = 100 * time.Millisecond
	}
	if err := os.MkdirAll(capture.Dir, 0o750); err != nil {
		return nil, fmt.Errorf("create profile dir: %w", err)
	}

	artifacts := make([]ProfileArtifact, 0, len(capture.Kinds))
	for _, kind := range capture.Kinds {
		artifact, err := captureRuntimeProfileKind(ctx, capture, kind)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func captureRuntimeProfileKind(ctx context.Context, capture RuntimeProfileCapture, kind RuntimeProfileKind) (ProfileArtifact, error) {
	path := filepath.Join(capture.Dir, safeProfileFilename(capture.Name, string(kind)))
	file, err := os.Create(path)
	if err != nil {
		return ProfileArtifact{}, fmt.Errorf("create %s profile: %w", kind, err)
	}
	defer file.Close()

	switch kind {
	case RuntimeProfileCPU:
		if err := pprof.StartCPUProfile(file); err != nil {
			return ProfileArtifact{}, fmt.Errorf("start cpu profile: %w", err)
		}
		workErr := runProfileWork(ctx, capture)
		if workErr == nil {
			timer := time.NewTimer(capture.CPUDuration)
			select {
			case <-ctx.Done():
				workErr = ctx.Err()
				timer.Stop()
			case <-timer.C:
			}
		}
		pprof.StopCPUProfile()
		if workErr != nil {
			return ProfileArtifact{}, fmt.Errorf("run cpu profile work: %w", workErr)
		}
	case RuntimeProfileTrace:
		if err := trace.Start(file); err != nil {
			return ProfileArtifact{}, fmt.Errorf("start trace profile: %w", err)
		}
		workErr := runProfileWork(ctx, capture)
		trace.Stop()
		if workErr != nil {
			return ProfileArtifact{}, fmt.Errorf("run trace profile work: %w", workErr)
		}
	case RuntimeProfileHeap:
		runtime.GC()
		if err := pprof.WriteHeapProfile(file); err != nil {
			return ProfileArtifact{}, fmt.Errorf("write heap profile: %w", err)
		}
	case RuntimeProfileGoroutine, RuntimeProfileBlock, RuntimeProfileMutex:
		profile := pprof.Lookup(string(kind))
		if profile == nil {
			return ProfileArtifact{}, fmt.Errorf("profile %q not found", kind)
		}
		if err := profile.WriteTo(file, 0); err != nil {
			return ProfileArtifact{}, fmt.Errorf("write %s profile: %w", kind, err)
		}
	default:
		return ProfileArtifact{}, fmt.Errorf("unsupported profile kind %q", kind)
	}

	return ProfileArtifact{Name: capture.Name, Kind: string(kind), Path: path}, nil
}

func runProfileWork(ctx context.Context, capture RuntimeProfileCapture) error {
	if capture.Work == nil {
		return nil
	}
	return capture.Work(ctx)
}

func safeProfileFilename(name, kind string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = "profile"
	}
	var b strings.Builder
	b.Grow(len(name) + len(kind) + 6)
	for _, r := range name {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
			continue
		}
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	b.WriteByte('.')
	b.WriteString(kind)
	if kind == string(RuntimeProfileTrace) {
		b.WriteString(".out")
	} else {
		b.WriteString(".pprof")
	}
	return b.String()
}
