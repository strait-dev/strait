package domain

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTransition_AllValidTransitions(t *testing.T) {
	t.Parallel()
	for from, toStatuses := range validTransitions {
		for _, to := range toStatuses {
			t.Run(fmt.Sprintf("%s->%s", from, to), func(t *testing.T) {
				assert.NoError(t, ValidateTransition(from, to))

			})
		}
	}
}

func TestValidateTransition_InvalidTransitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		from RunStatus
		to   RunStatus
	}{
		{StatusCompleted, StatusExecuting},
		{StatusFailed, StatusExecuting},
		{StatusQueued, StatusCompleted},
		{StatusDelayed, StatusExecuting},
		{StatusExecuting, StatusDelayed},
		{StatusWaiting, StatusDequeued},
		{StatusCanceled, StatusQueued},
		{StatusExpired, StatusQueued},
		{StatusSystemFailed, StatusCompleted},
		{StatusTimedOut, StatusWaiting},
		{StatusDequeued, StatusCompleted},
		{RunStatus("unknown"), StatusQueued},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s->%s", tc.from, tc.to), func(t *testing.T) {
			assert.Error(t, ValidateTransition(tc.from, tc.to))

		})
	}
}

func TestTerminalStatesHaveNoValidTransitions(t *testing.T) {
	t.Parallel()
	// dead_letter is terminal from the perspective of automatic state
	// progression (SSE handlers, CDC notifiers, the reaper, replay
	// idempotency). It still has FSM out-edges for OPERATOR-INITIATED
	// replay via dead_letter -> queued or dead_letter -> replay_staged.
	// Allow that pair only; every other terminal status must be a sink.
	deadLetterAllowed := map[RunStatus]struct{}{
		StatusQueued:       {},
		StatusReplayStaged: {},
	}
	for _, status := range TerminalStatuses() {
		t.Run(string(status), func(t *testing.T) {
			transitions, ok := validTransitions[status]
			assert.True(
				t, ok)

			if status == StatusDeadLetter {
				for _, to := range transitions {
					assert.Contains(t, deadLetterAllowed, to)
				}
				return
			}
			assert.Len(t,
				transitions,

				0)

		})
	}
}

func TestRunStatusIsTerminal_AllStatuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status   RunStatus
		expected bool
	}{
		{StatusDelayed, false},
		{StatusQueued, false},
		{StatusDequeued, false},
		{StatusExecuting, false},
		{StatusWaiting, false},
		{StatusCompleted, true},
		{StatusFailed, true},
		{StatusTimedOut, true},
		{StatusCrashed, true},
		{StatusSystemFailed, true},
		{StatusCanceled, true},
		{StatusExpired, true},
		{StatusDeadLetter, true},
		{StatusReplayStaged, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.status.IsTerminal())
		})
	}
}

func TestAllStatusesCoveredByTransitionsMap(t *testing.T) {
	t.Parallel()
	allStatuses := []RunStatus{
		StatusDelayed,
		StatusQueued,
		StatusDequeued,
		StatusExecuting,
		StatusWaiting,
		StatusCompleted,
		StatusFailed,
		StatusTimedOut,
		StatusCrashed,
		StatusSystemFailed,
		StatusCanceled,
		StatusExpired,
		StatusDeadLetter,
		StatusReplayStaged,
		StatusPaused,
	}

	for _, status := range allStatuses {
		t.Run(string(status), func(t *testing.T) {
			assert.Contains(t, validTransitions, status)
		})
	}
	require.Len(
		t, validTransitions,

		len(allStatuses))

}

func TestRunStatusIsValid(t *testing.T) {
	t.Parallel()
	require.True(t, StatusQueued.
		IsValid())
	require.False(t, RunStatus("not-valid").IsValid())

}

func TestValidateTransition_DeadLetterTransitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		from    RunStatus
		to      RunStatus
		wantErr bool
	}{
		{name: "executing to dead_letter is valid", from: StatusExecuting, to: StatusDeadLetter},
		{name: "dead_letter to queued is valid", from: StatusDeadLetter, to: StatusQueued},
		{name: "dead_letter to replay_staged is valid", from: StatusDeadLetter, to: StatusReplayStaged},
		{name: "replay_staged to queued is valid", from: StatusReplayStaged, to: StatusQueued},
		{name: "replay_staged to canceled is valid", from: StatusReplayStaged, to: StatusCanceled},
		{name: "dead_letter to completed is invalid", from: StatusDeadLetter, to: StatusCompleted, wantErr: true},
		{name: "replay_staged to completed is invalid", from: StatusReplayStaged, to: StatusCompleted, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTransition(tc.from, tc.to)
			require.False(t, tc.wantErr &&
				err == nil)
			require.False(t, !tc.wantErr &&
				err != nil)

		})
	}
}

func FuzzFSMTransition(f *testing.F) {
	f.Add("queued", "dequeued")
	f.Add("executing", "completed")
	f.Add("executing", "failed")
	f.Add("nonsense", "also_nonsense")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, from, to string) {
		_ = ValidateTransition(RunStatus(from), RunStatus(to))
	})
}

func BenchmarkFSMTransition(b *testing.B) {
	for b.Loop() {
		_ = ValidateTransition(StatusExecuting, StatusCompleted)
	}
}
