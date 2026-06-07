package worker

import (
	"context"
	"time"

	"github.com/sourcegraph/conc"
)

// doPoll wraps a single poll cycle with proper WaitGroup tracking via
// defer so that pollWG.Done is always called even if poll panics.
func (e *Executor) doPoll(ctx context.Context) {
	e.pollWG.Add(1)
	e.pollInFlight.Add(1)
	defer e.pollWG.Done()
	defer e.pollInFlight.Add(-1)
	e.poll(ctx)
}

// Run starts the heartbeat manager, event loop, and polling loop. Blocks until ctx is canceled.
func (e *Executor) Run(ctx context.Context) {
	e.runStarted.Store(true)

	// Create a child context that cancels when either the parent context
	// is canceled or Shutdown closes e.stop, so all background goroutines
	// (heartbeat, pool pruner) exit promptly in both cases.
	// runCancel must fire before pollWG.Wait or shutdown can deadlock.
	runCtx, runCancel := context.WithCancel(ctx) //nolint:gosec,nolintlint

	defer func() {
		runCancel() // Cancel context first so heartbeat and other goroutines exit.
		close(e.done)
		// Wait for in-flight polls and tracked goroutines to finish
		// emitting events, then close the event channel so the event
		// loop goroutine exits cleanly.
		e.pollWG.Wait()
		close(e.eventCh)
	}()

	e.logger.Info("executor started", "poll_interval", e.pollInterval)

	e.bgWG.Go(func() {
		e.heartbeat.Run(runCtx)
	})
	e.bgWG.Go(e.runEventLoop)

	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()

	var degradedCh <-chan struct{}
	if e.degraded != nil {
		degradedCh = e.degraded.Degraded()
	}
	inDegradedMode := false

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("executor stopping")
			return
		case <-e.stop:
			e.logger.Info("executor stopping")
			return
		case _, ok := <-e.wake:
			if !ok {
				e.wake = nil
				continue
			}
			if inDegradedMode {
				ticker.Reset(e.pollInterval)
				inDegradedMode = false
				if e.degraded != nil {
					degradedCh = e.degraded.Degraded()
				}
				e.logger.Info("executor restored normal poll interval after wake reconnect")
			}
			e.doPoll(ctx)
		case <-e.drainWake:
			e.doPoll(ctx)
		case <-degradedCh:
			ticker.Reset(e.degradedPollInterval)
			inDegradedMode = true
			degradedCh = nil
			e.logger.Warn("executor entering degraded mode: fast polling engaged",
				"degraded_poll_interval", e.degradedPollInterval,
			)
			e.doPoll(ctx)
		case <-ticker.C:
			e.doPoll(ctx)
		}
	}
}

func (e *Executor) Shutdown(ctx context.Context) error {
	e.stopOnce.Do(func() {
		close(e.stop)
	})

	if !e.runStarted.Load() {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.done:
	}

	e.pollWG.Wait()

	callbackDone := make(chan struct{})
	var callbackWaitWG conc.WaitGroup
	callbackWaitWG.Go(func() {
		e.callbackWG.Wait()
		close(callbackDone)
	})

	callbackTimeout := time.NewTimer(10 * time.Second)
	defer callbackTimeout.Stop()
	select {
	case <-callbackDone:
	case <-callbackTimeout.C:
		e.logger.Warn("timed out waiting for in-flight workflow callbacks")
	case <-ctx.Done():
		return ctx.Err()
	}

	// Wait for any in-flight Stripe usage event goroutines.
	stripeDone := make(chan struct{})
	var stripeWaitWG conc.WaitGroup
	stripeWaitWG.Go(func() {
		e.stripeUsageWG.Wait()
		close(stripeDone)
	})

	stripeTimeout := time.NewTimer(10 * time.Second)
	defer stripeTimeout.Stop()
	select {
	case <-stripeDone:
	case <-stripeTimeout.C:
		e.logger.Warn("timed out waiting for in-flight stripe usage events")
	case <-ctx.Done():
		return ctx.Err()
	}

	bgDone := make(chan struct{})
	var bgWaitWG conc.WaitGroup
	bgWaitWG.Go(func() {
		e.bgWG.Wait()
		close(bgDone)
	})

	bgTimeout := time.NewTimer(10 * time.Second)
	defer bgTimeout.Stop()
	select {
	case <-bgDone:
	case <-bgTimeout.C:
		e.logger.Warn("timed out waiting for executor background goroutines")
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}
