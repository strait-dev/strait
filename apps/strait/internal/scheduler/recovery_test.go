package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

func TestSafeGo_NoPanic_RunsNormally(t *testing.T) {
	// Not parallel: mutates package-level exitFunc.
	origExit := exitFunc
	exitFunc = func(code int) {
		require.Failf(t, "test failure",

			"exitFunc should not be called, got code %d", code)
	}
	defer func() { exitFunc = origExit }()

	var ran bool
	var wg conc.WaitGroup
	safeGo(&wg, "no-panic", func() {
		ran = true
	})
	wg.Wait()
	require.True(t, ran)
}

func TestSafeGoWithContext_AddsSchedulerBreadcrumb(t *testing.T) {
	t.Parallel()

	hub := sentry.NewHub(nil, sentry.NewScope())
	ctx := sentry.SetHubOnContext(context.Background(), hub)

	var wg conc.WaitGroup
	safeGoWithContext(ctx, sentrySchedulerMetadata{mode: "all", region: "iad", version: "test-version"}, &wg, "breadcrumb-component", func(context.Context) {})
	wg.Wait()

	event := hub.Scope().ApplyToEvent(&sentry.Event{}, nil, nil)
	require.False(t, event ==
		nil ||
		len(event.Breadcrumbs) !=
			1)
	require.Equal(t, "scheduler.component",

		event.
			Breadcrumbs[0].Category,
	)
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
	require.Len(t, got,
		2)
	require.Equal(t, "strait-scheduler-clean-component",

		got[0].MonitorSlug,
	)
	require.Equal(t, sentry.
		CheckInStatusInProgress,

		got[0].
			Status)
	require.Equal(t, id,
		got[1].ID,
	)
	require.Equal(t, sentry.
		CheckInStatusOK,
		got[1].Status)
	require.GreaterOrEqual(t, got[1].Duration, time.Duration(0))
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
	require.NotNil(t, gotHub)
	require.Equal(t, "component",

		gotCheckIn.component,
	)
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
	require.True(t, ran)
	require.Len(t, got,
		2)
	require.Equal(t, "strait-reaper-cycle",

		got[0].checkIn.
			MonitorSlug,
	)
	require.False(t, got[0].config ==
		nil || got[0].config.
		CheckInMargin !=
		2 || got[0].config.MaxRuntime !=
		2)
	require.Equal(t, id,
		got[1].checkIn.
			ID)
	require.Equal(t, sentry.
		CheckInStatusOK,
		got[1].checkIn.
			Status)
}

func TestRunSchedulerCycleCheckInWithError_CapturesFailedCycle(t *testing.T) {
	// Not parallel: mutates package-level captureSchedulerCheckIn.
	origCapture := captureSchedulerCheckIn
	defer func() { captureSchedulerCheckIn = origCapture }()

	var got []sentry.CheckIn
	id := sentry.EventID("cycle-check-in-id")
	captureSchedulerCheckIn = func(checkIn *sentry.CheckIn, _ *sentry.MonitorConfig) *sentry.EventID {
		got = append(got, *checkIn)
		return &id
	}

	ctx := context.WithValue(context.Background(), schedulerCheckInContextKey{}, schedulerCheckInContext{
		meta: sentrySchedulerMetadata{
			checkInsEnabled:      true,
			checkInMonitorPrefix: "strait",
		},
		component: "outbox_flusher",
	})
	err := runSchedulerCycleCheckInWithError(ctx, time.Minute, func() error {
		return errors.New("flush failed")
	})
	require.Error(t, err)
	require.Len(t, got,
		2)
	require.Equal(t, sentry.
		CheckInStatusError,

		got[1].Status,
	)
	require.Equal(t, "strait-outbox-flusher-cycle",

		got[1].
			MonitorSlug,
	)
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
	require.Len(t, got,
		2)
	require.Equal(t, sentry.
		CheckInStatusError,

		got[1].Status,
	)
	require.LessOrEqual(t, got[1].
		Duration, time.
		Minute)
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
	require.NotNil(t, event)

	wantTags := map[string]string{
		"subsystem": "scheduler",
		"mode":      "all",
		"region":    "iad",
		"version":   "test-version",
		"operation": "poller",
	}
	for key, want := range wantTags {
		require.Equal(t, want,
			event.
				Tags[key])
	}
	require.Equal(t, "poller",
		event.
			Contexts["scheduler.component"]["component"])
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
	require.EqualValues(t, 1,
		exitCode.
			Load())
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
	require.True(t, called.
		Load(),
	)

	// panic(nil) is still caught by recover() in Go 1.21+; in older Go it returns nil.
	// Either way, exitFunc should be called because the deferred recover fires.
	// Note: In Go 1.21+ panic(nil) wraps into a *runtime.PanicNilError.
}
