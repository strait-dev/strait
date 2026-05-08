package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

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
	safeGoWithContext(ctx, sentrySchedulerMetadata{mode: "all", region: "iad", version: "test-version"}, &wg, "breadcrumb-component", func(context.Context) {})
	wg.Wait()

	event := hub.Scope().ApplyToEvent(&sentry.Event{}, nil, nil)
	if event == nil || len(event.Breadcrumbs) != 1 {
		t.Fatalf("breadcrumbs = %v, want one breadcrumb", event)
	}
	if got := event.Breadcrumbs[0].Category; got != "scheduler.component" {
		t.Fatalf("breadcrumb category = %q, want scheduler.component", got)
	}
}

func TestSafeGoWithContext_CapturesSchedulerCheckIns(t *testing.T) {
	// Not parallel: mutates package-level captureSchedulerCheckIn.
	origCapture := captureSchedulerCheckIn
	defer func() { captureSchedulerCheckIn = origCapture }()

	var got []sentry.CheckIn
	id := sentry.EventID("check-in-id")
	captureSchedulerCheckIn = func(checkIn *sentry.CheckIn, _ *sentry.MonitorConfig) *sentry.EventID {
		got = append(got, *checkIn)
		return &id
	}

	var wg conc.WaitGroup
	safeGoWithContext(context.Background(), sentrySchedulerMetadata{
		checkInsEnabled:      true,
		checkInMonitorPrefix: "Strait Scheduler",
	}, &wg, "Clean Component", func(context.Context) {})
	wg.Wait()

	if len(got) != 2 {
		t.Fatalf("check-ins = %d, want 2", len(got))
	}
	if got[0].MonitorSlug != "strait-scheduler-clean-component" {
		t.Fatalf("monitor slug = %q, want sanitized slug", got[0].MonitorSlug)
	}
	if got[0].Status != sentry.CheckInStatusInProgress {
		t.Fatalf("start status = %q, want in_progress", got[0].Status)
	}
	if got[1].ID != id {
		t.Fatalf("finish check-in id = %q, want %q", got[1].ID, id)
	}
	if got[1].Status != sentry.CheckInStatusOK {
		t.Fatalf("finish status = %q, want ok", got[1].Status)
	}
	if got[1].Duration < 0 {
		t.Fatalf("duration = %v, want non-negative", got[1].Duration)
	}
}

func TestSafeGoWithContext_PassesSentryContextToComponent(t *testing.T) {
	t.Parallel()

	hub := sentry.NewHub(nil, sentry.NewScope())
	ctx := sentry.SetHubOnContext(context.Background(), hub)

	var wg conc.WaitGroup
	var gotHub *sentry.Hub
	var gotCheckIn schedulerCheckInContext
	safeGoWithContext(ctx, sentrySchedulerMetadata{
		checkInsEnabled:      true,
		checkInMonitorPrefix: "strait",
	}, &wg, "component", func(componentCtx context.Context) {
		gotHub = sentry.GetHubFromContext(componentCtx)
		gotCheckIn, _ = componentCtx.Value(schedulerCheckInContextKey{}).(schedulerCheckInContext)
	})
	wg.Wait()

	if gotHub == nil {
		t.Fatal("component context is missing sentry hub")
	}
	if gotCheckIn.component != "component" {
		t.Fatalf("component check-in context = %q, want component", gotCheckIn.component)
	}
}

func TestRunSchedulerCycleCheckIn_CapturesConfiguredCycle(t *testing.T) {
	// Not parallel: mutates package-level captureSchedulerCheckIn.
	origCapture := captureSchedulerCheckIn
	defer func() { captureSchedulerCheckIn = origCapture }()

	type captured struct {
		checkIn sentry.CheckIn
		config  *sentry.MonitorConfig
	}
	var got []captured
	id := sentry.EventID("cycle-check-in-id")
	captureSchedulerCheckIn = func(checkIn *sentry.CheckIn, cfg *sentry.MonitorConfig) *sentry.EventID {
		got = append(got, captured{checkIn: *checkIn, config: cfg})
		return &id
	}

	ctx := context.WithValue(context.Background(), schedulerCheckInContextKey{}, schedulerCheckInContext{
		meta: sentrySchedulerMetadata{
			checkInsEnabled:      true,
			checkInMonitorPrefix: "strait",
		},
		component: "reaper",
	})
	ran := false
	runSchedulerCycleCheckIn(ctx, 90*time.Second, func() {
		ran = true
	})

	if !ran {
		t.Fatal("cycle body did not run")
	}
	if len(got) != 2 {
		t.Fatalf("check-ins = %d, want 2", len(got))
	}
	if got[0].checkIn.MonitorSlug != "strait-reaper-cycle" {
		t.Fatalf("monitor slug = %q, want strait-reaper-cycle", got[0].checkIn.MonitorSlug)
	}
	if got[0].config == nil || got[0].config.CheckInMargin != 2 || got[0].config.MaxRuntime != 2 {
		t.Fatalf("monitor config = %#v, want 2 minute interval config", got[0].config)
	}
	if got[1].checkIn.ID != id {
		t.Fatalf("finish id = %q, want %q", got[1].checkIn.ID, id)
	}
	if got[1].checkIn.Status != sentry.CheckInStatusOK {
		t.Fatalf("finish status = %q, want ok", got[1].checkIn.Status)
	}
}

func TestSafeGoWithContext_CapturesErrorCheckInOnPanic(t *testing.T) {
	// Not parallel: mutates package-level captureSchedulerCheckIn and exitFunc.
	origCapture := captureSchedulerCheckIn
	origExit := exitFunc
	defer func() {
		captureSchedulerCheckIn = origCapture
		exitFunc = origExit
	}()

	var got []sentry.CheckIn
	id := sentry.EventID("panic-check-in-id")
	captureSchedulerCheckIn = func(checkIn *sentry.CheckIn, _ *sentry.MonitorConfig) *sentry.EventID {
		got = append(got, *checkIn)
		return &id
	}
	exitFunc = func(int) {}

	var wg conc.WaitGroup
	safeGoWithContext(context.Background(), sentrySchedulerMetadata{
		checkInsEnabled:      true,
		checkInMonitorPrefix: "strait",
	}, &wg, "panic_component", func(context.Context) {
		panic("boom")
	})
	wg.Wait()

	if len(got) != 2 {
		t.Fatalf("check-ins = %d, want 2", len(got))
	}
	if got[1].Status != sentry.CheckInStatusError {
		t.Fatalf("finish status = %q, want error", got[1].Status)
	}
	if got[1].Duration > time.Minute {
		t.Fatalf("duration = %v, want bounded test duration", got[1].Duration)
	}
}

func TestApplySchedulerSentryScopeAddsRuntimeTags(t *testing.T) {
	t.Parallel()

	scope := sentry.NewScope()
	applySchedulerSentryScope(scope, sentrySchedulerMetadata{
		mode:    "all",
		region:  "iad",
		version: "test-version",
	}, "poller", "boom")
	event := scope.ApplyToEvent(&sentry.Event{}, nil, nil)
	if event == nil {
		t.Fatal("expected event")
	}
	wantTags := map[string]string{
		"subsystem": "scheduler",
		"mode":      "all",
		"region":    "iad",
		"version":   "test-version",
		"operation": "poller",
	}
	for key, want := range wantTags {
		if got := event.Tags[key]; got != want {
			t.Fatalf("tag %s = %q, want %q", key, got, want)
		}
	}
	if got := event.Contexts["scheduler.component"]["component"]; got != "poller" {
		t.Fatalf("scheduler component context = %v, want poller", got)
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
