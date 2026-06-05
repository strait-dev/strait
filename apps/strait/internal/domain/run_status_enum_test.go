package domain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunStatus_IsValid(t *testing.T) {
	valid := []RunStatus{
		StatusDelayed, StatusQueued, StatusDequeued, StatusExecuting, StatusWaiting,
		StatusCompleted, StatusFailed, StatusTimedOut, StatusCrashed, StatusSystemFailed,
		StatusCanceled, StatusExpired, StatusDeadLetter, StatusReplayStaged, StatusPaused,
	}
	for _, s := range valid {
		assert.True(t,
			s.IsValid(),
		)

	}
	invalid := []RunStatus{"", "pending", "dlq_overflow", "DEAD_LETTER", "Queued"}
	for _, s := range invalid {
		assert.False(t,
			s.IsValid())

	}
}

func TestRunStatus_IsActive(t *testing.T) {
	active := []RunStatus{StatusDequeued, StatusExecuting}
	for _, s := range active {
		assert.True(t,
			s.IsActive())

	}
	inactive := []RunStatus{StatusQueued, StatusCompleted, StatusDeadLetter, StatusDelayed}
	for _, s := range inactive {
		assert.False(t,
			s.IsActive(),
		)

	}
}

func TestRunStatus_IsClaimable(t *testing.T) {
	assert.True(t,
		StatusQueued.
			IsClaimable())
	assert.False(t,
		StatusDequeued.
			IsClaimable())

}

func TestRunStatus_IsTerminal(t *testing.T) {
	cases := map[RunStatus]bool{
		StatusCompleted:    true,
		StatusFailed:       true,
		StatusTimedOut:     true,
		StatusCrashed:      true,
		StatusSystemFailed: true,
		StatusCanceled:     true,
		StatusExpired:      true,
		// dead_letter is a permanently-failed terminal state. SSE handlers,
		// CDC notifiers, the reaper, and replay all need it to be terminal;
		// IsDeadLetter is available when callers need to distinguish DLQ
		// from normal completion.
		StatusDeadLetter: true,
		StatusQueued:     false,
		StatusExecuting:  false,
		StatusDelayed:    false,
		StatusWaiting:    false,
		StatusDequeued:   false,
		StatusPaused:     false,
	}
	for s, want := range cases {
		assert.Equal(t, want, s.IsTerminal())
	}
}

func TestRunStatus_IsDeadLetter(t *testing.T) {
	assert.True(t,
		StatusDeadLetter.
			IsDeadLetter())
	assert.False(t,
		StatusFailed.
			IsDeadLetter())

}

func TestRunStatus_IsFailure(t *testing.T) {
	failures := []RunStatus{
		StatusFailed, StatusTimedOut, StatusCrashed, StatusSystemFailed, StatusDeadLetter,
	}
	for _, s := range failures {
		assert.True(t,
			s.IsFailure(),
		)

	}
	nonFailures := []RunStatus{StatusCompleted, StatusCanceled, StatusExpired, StatusQueued}
	for _, s := range nonFailures {
		assert.False(t,
			s.IsFailure(),
		)

	}
}

func TestRunStatus_Scan(t *testing.T) {
	cases := []struct {
		name    string
		src     any
		want    RunStatus
		wantErr bool
	}{
		{"string valid", "queued", StatusQueued, false},
		{"bytes valid", []byte("executing"), StatusExecuting, false},
		{"nil", nil, "", false},
		{"string invalid", "pending", "", true},
		{"unknown type", 42, "", true},
		{"empty string", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var s RunStatus
			err := s.Scan(c.src)
			if c.wantErr {
				assert.Error(t,
					err)

				return
			}
			assert.NoError(
				t, err)
			assert.Equal(t,
				c.want, s)

		})
	}
}

func TestRunStatus_Value(t *testing.T) {
	v, err := StatusQueued.Value()
	require.NoError(t, err)
	assert.Equal(t,
		"queued",
		v)

	emptyV, err := RunStatus("").Value()
	assert.False(t,
		err != nil ||

			emptyV != nil)

	_, err = RunStatus("garbage").Value()
	assert.Error(t, err)
}

func TestParseRunStatus(t *testing.T) {
	got, err := ParseRunStatus("queued")
	assert.False(t,
		err != nil ||

			got != StatusQueued)

	_, err = ParseRunStatus("dlq_overflow")
	assert.True(t,
		errors.Is(err,

			ErrUnknownRunStatus))

}

// FuzzRunStatusScan must not panic on any input and must either accept the
// value or return an error.
func FuzzRunStatusScan(f *testing.F) {
	f.Add("queued")
	f.Add("")
	f.Add("DROP TABLE job_runs")
	f.Fuzz(func(t *testing.T, raw string) {
		var s RunStatus
		defer func() {
			if r := recover(); r != nil {
				require.Failf(t, "Scan panicked", "raw=%q panic=%v", raw, r)
			}
		}()
		err := s.Scan(raw)
		assert.False(t,
			err == nil &&

				raw != "" && !s.IsValid())

	})
}
