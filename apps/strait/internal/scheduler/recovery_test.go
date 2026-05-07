package scheduler

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/sourcegraph/conc"
)

func TestSafeGo_NoPanic_RunsNormally(t *testing.T) {
	// Not parallel: mutates package-level exitFunc.
	origExit := exitFunc
	exitFunc = func(code int) {
		t.Fatalf("exitFunc should not be called, got code %d", code)
	}
	defer func() { exitFunc = origExit }()

	var ran bool
	var wg conc.WaitGroup
	safeGo(&wg, "no-panic", func() {
		ran = true
	})
	wg.Wait()

	if !ran {
		t.Fatal("expected function to run")
	}
}

func TestSafeGoWithContext_AddsSchedulerBreadcrumb(t *testing.T) {
	t.Parallel()

	hub := sentry.NewHub(nil, sentry.NewScope())
	ctx := sentry.SetHubOnContext(context.Background(), hub)

	var wg conc.WaitGroup
	safeGoWithContext(ctx, &wg, "breadcrumb-component", func() {})
	wg.Wait()

	event := hub.Scope().ApplyToEvent(&sentry.Event{}, nil, nil)
	if event == nil || len(event.Breadcrumbs) != 1 {
		t.Fatalf("breadcrumbs = %v, want one breadcrumb", event)
	}
	if got := event.Breadcrumbs[0].Category; got != "scheduler.component" {
		t.Fatalf("breadcrumb category = %q, want scheduler.component", got)
	}
}

func TestSafeGo_Panic_CallsExit(t *testing.T) {
	// Not parallel: mutates package-level exitFunc.
	var exitCode atomic.Int32
	exitCode.Store(-1)
	origExit := exitFunc
	exitFunc = func(code int) {
		exitCode.Store(int32(code))
	}
	defer func() { exitFunc = origExit }()

	var wg conc.WaitGroup
	safeGo(&wg, "crash-component", func() {
		panic("something broke")
	})
	wg.Wait()

	if exitCode.Load() != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode.Load())
	}
}

func TestSafeGo_Panic_NilValue(t *testing.T) {
	// Not parallel: mutates package-level exitFunc.
	var called atomic.Bool
	origExit := exitFunc
	exitFunc = func(_ int) {
		called.Store(true)
	}
	defer func() { exitFunc = origExit }()

	var wg conc.WaitGroup
	safeGo(&wg, "nil-panic", func() {
		panic(nil)
	})
	wg.Wait()

	// panic(nil) is still caught by recover() in Go 1.21+; in older Go it returns nil.
	// Either way, exitFunc should be called because the deferred recover fires.
	// Note: In Go 1.21+ panic(nil) wraps into a *runtime.PanicNilError.
	if !called.Load() {
		t.Fatal("expected exitFunc to be called on panic(nil)")
	}
}
