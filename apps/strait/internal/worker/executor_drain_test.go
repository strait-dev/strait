package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDrainController_RequestRequiresFullBatchHint(t *testing.T) {
	t.Parallel()

	d := newDrainController(nil)
	d.observePoll(10, 9, nil)
	d.request(context.Background())

	select {
	case <-d.wakeChan():
		t.Fatal("partial batch should not request drain")
	default:
	}
	require.False(t, d.hasBacklogHint())

	d.observePoll(10, 10, nil)
	d.request(context.Background())

	select {
	case <-d.wakeChan():
	default:
		t.Fatal("full batch should request drain")
	}
	require.True(t, d.hasBacklogHint())
}

func TestDrainController_EmptyOrErrorPollClearsHint(t *testing.T) {
	t.Parallel()

	d := newDrainController(nil)
	d.observePoll(2, 2, nil)
	require.True(t, d.hasBacklogHint())

	d.observePoll(2, 0, nil)
	require.False(t, d.hasBacklogHint())

	d.observePoll(2, 2, nil)
	require.True(t, d.hasBacklogHint())

	d.observePoll(2, 0, errors.New("dequeue failed"))
	require.False(t, d.hasBacklogHint())

	d.observePoll(2, 2, nil)
	require.True(t, d.hasBacklogHint())

	d.observePoll(0, 2, nil)
	require.False(t, d.hasBacklogHint())
}

func TestDrainController_CoalescesPendingWake(t *testing.T) {
	t.Parallel()

	d := newDrainController(nil)
	d.observePoll(1, 1, nil)
	d.request(context.Background())
	d.request(context.Background())

	select {
	case <-d.wakeChan():
	default:
		t.Fatal("first drain wake was not delivered")
	}

	select {
	case <-d.wakeChan():
		t.Fatal("second drain wake should be coalesced")
	default:
	}
}
