package domain

import (
	"errors"
	"testing"
)

func TestRunStatus_IsValid(t *testing.T) {
	valid := []RunStatus{
		StatusDelayed, StatusQueued, StatusDequeued, StatusExecuting, StatusWaiting,
		StatusCompleted, StatusFailed, StatusTimedOut, StatusCrashed, StatusSystemFailed,
		StatusCanceled, StatusExpired, StatusDeadLetter, StatusReplayStaged, StatusPaused,
	}
	for _, s := range valid {
		if !s.IsValid() {
			t.Errorf("%q should be valid", s)
		}
	}
	invalid := []RunStatus{"", "pending", "dlq_overflow", "DEAD_LETTER", "Queued"}
	for _, s := range invalid {
		if s.IsValid() {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestRunStatus_IsActive(t *testing.T) {
	active := []RunStatus{StatusDequeued, StatusExecuting}
	for _, s := range active {
		if !s.IsActive() {
			t.Errorf("%q should be active", s)
		}
	}
	inactive := []RunStatus{StatusQueued, StatusCompleted, StatusDeadLetter, StatusDelayed}
	for _, s := range inactive {
		if s.IsActive() {
			t.Errorf("%q should not be active", s)
		}
	}
}

func TestRunStatus_IsClaimable(t *testing.T) {
	if !StatusQueued.IsClaimable() {
		t.Error("queued should be claimable")
	}
	if StatusDequeued.IsClaimable() {
		t.Error("dequeued should not be claimable")
	}
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
		if got := s.IsTerminal(); got != want {
			t.Errorf("%q IsTerminal = %v, want %v", s, got, want)
		}
	}
}

func TestRunStatus_IsDeadLetter(t *testing.T) {
	if !StatusDeadLetter.IsDeadLetter() {
		t.Error("dead_letter should be dead letter")
	}
	if StatusFailed.IsDeadLetter() {
		t.Error("failed is not dead_letter")
	}
}

func TestRunStatus_IsFailure(t *testing.T) {
	failures := []RunStatus{
		StatusFailed, StatusTimedOut, StatusCrashed, StatusSystemFailed, StatusDeadLetter,
	}
	for _, s := range failures {
		if !s.IsFailure() {
			t.Errorf("%q should be failure", s)
		}
	}
	nonFailures := []RunStatus{StatusCompleted, StatusCanceled, StatusExpired, StatusQueued}
	for _, s := range nonFailures {
		if s.IsFailure() {
			t.Errorf("%q should not be failure", s)
		}
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
				if err == nil {
					t.Errorf("want error, got %q", s)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if s != c.want {
				t.Errorf("got %q, want %q", s, c.want)
			}
		})
	}
}

func TestRunStatus_Value(t *testing.T) {
	v, err := StatusQueued.Value()
	if err != nil {
		t.Fatalf("value: %v", err)
	}
	if v != "queued" {
		t.Errorf("value = %v, want queued", v)
	}
	emptyV, err := RunStatus("").Value()
	if err != nil || emptyV != nil {
		t.Errorf("empty status should be nil, got %v %v", emptyV, err)
	}
	if _, err := RunStatus("garbage").Value(); err == nil {
		t.Error("garbage should error")
	}
}

func TestParseRunStatus(t *testing.T) {
	got, err := ParseRunStatus("queued")
	if err != nil || got != StatusQueued {
		t.Errorf("got %q %v", got, err)
	}
	_, err = ParseRunStatus("dlq_overflow")
	if !errors.Is(err, ErrUnknownRunStatus) {
		t.Errorf("want ErrUnknownRunStatus, got %v", err)
	}
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
				t.Fatalf("Scan panicked on %q: %v", raw, r)
			}
		}()
		err := s.Scan(raw)
		if err == nil && raw != "" && !s.IsValid() {
			t.Errorf("Scan accepted invalid %q → %q", raw, s)
		}
	})
}
