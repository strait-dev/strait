//go:build loadtest

package loadtest

import (
	"context"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
)

func TestStartTrackedLoadtestTriggerBoundsAndDrains(t *testing.T) {
	var wg conc.WaitGroup
	slots := make(chan struct{}, 1)
	release := make(chan struct{})
	started := make(chan struct{}, 1)

	ok := startTrackedLoadtestTrigger(context.Background(), &wg, slots, func(context.Context) error {
		started <- struct{}{}
		<-release
		return nil
	}, nil, nil)
	if !ok {
		t.Fatal("expected first trigger to start")
	}
	<-started

	secondReturned := make(chan struct{})
	go func() {
		_ = startTrackedLoadtestTrigger(context.Background(), &wg, slots, func(context.Context) error {
			started <- struct{}{}
			return nil
		}, nil, nil)
		close(secondReturned)
	}()

	select {
	case <-secondReturned:
		t.Fatal("second trigger should wait for an in-flight slot")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	wg.Wait()

	select {
	case <-secondReturned:
	case <-time.After(time.Second):
		t.Fatal("second trigger did not start after the first drained")
	}
	wg.Wait()
}
