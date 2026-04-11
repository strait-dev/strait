package worker

import (
	"context"
	"errors"
	"testing"
)

// Phase 9 unit tests for the DLQ cap enforcer.

type fakeDLQStore struct {
	perJob     map[string]int
	perProject map[string]int
	masked     []string
	err        error
}

func newFakeDLQStore() *fakeDLQStore {
	return &fakeDLQStore{
		perJob:     map[string]int{},
		perProject: map[string]int{},
	}
}

func (f *fakeDLQStore) DLQDepth(_ context.Context, projectID, jobID string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.perJob[projectID+":"+jobID], nil
}
func (f *fakeDLQStore) DLQDepthByProject(_ context.Context, projectID string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.perProject[projectID], nil
}
func (f *fakeDLQStore) MaskOldestDLQRow(_ context.Context, projectID, jobID string) (string, error) {
	id := "victim-" + projectID + ":" + jobID
	f.masked = append(f.masked, id)
	// Simulate the trigger decrementing counters.
	f.perJob[projectID+":"+jobID]--
	f.perProject[projectID]--
	return id, nil
}

func TestDLQCapEnforcer_NoCapProceeds(t *testing.T) {
	e := NewDLQCapEnforcer(newFakeDLQStore(), DLQCapConfig{}, nil)
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	if !proceed || err != nil {
		t.Errorf("expected (true, nil), got (%v, %v)", proceed, err)
	}
}

func TestDLQCapEnforcer_UnderCapProceeds(t *testing.T) {
	s := newFakeDLQStore()
	s.perJob["p:j"] = 5
	s.perProject["p"] = 5
	e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerJob: 10, MaxPerProject: 100, Policy: DLQOverflowReject}, nil)
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	if !proceed || err != nil {
		t.Errorf("expected proceed under cap, got (%v, %v)", proceed, err)
	}
}

func TestDLQCapEnforcer_PerJobRejectAtCap(t *testing.T) {
	s := newFakeDLQStore()
	s.perJob["p:j"] = 10
	e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerJob: 10, Policy: DLQOverflowReject}, nil)
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	if proceed {
		t.Error("expected rejection")
	}
	if !errors.Is(err, ErrDLQOverflow) {
		t.Errorf("err = %v, want ErrDLQOverflow", err)
	}
	if e.OverflowCount() != 1 {
		t.Errorf("overflow count = %d, want 1", e.OverflowCount())
	}
}

func TestDLQCapEnforcer_PerProjectRejectAtCap(t *testing.T) {
	s := newFakeDLQStore()
	s.perProject["p"] = 100
	e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerProject: 100, Policy: DLQOverflowReject}, nil)
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	if proceed || !errors.Is(err, ErrDLQOverflow) {
		t.Errorf("expected reject, got (%v, %v)", proceed, err)
	}
}

func TestDLQCapEnforcer_DropOldestMasksAndProceeds(t *testing.T) {
	s := newFakeDLQStore()
	s.perJob["p:j"] = 10
	e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerJob: 10, Policy: DLQOverflowDropOldest}, nil)
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	if !proceed || err != nil {
		t.Errorf("expected proceed after drop, got (%v, %v)", proceed, err)
	}
	if e.DroppedCount() != 1 {
		t.Errorf("dropped count = %d, want 1", e.DroppedCount())
	}
	if len(s.masked) != 1 {
		t.Errorf("masked = %v, want 1", s.masked)
	}
}

func TestDLQCapEnforcer_StoreErrorFailsOpen(t *testing.T) {
	s := newFakeDLQStore()
	s.err = errors.New("pg down")
	e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerJob: 10, Policy: DLQOverflowReject}, nil)
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	if !proceed {
		t.Error("expected fail-open on store error")
	}
	if err == nil {
		t.Error("expected error propagation")
	}
}

func TestDLQCapEnforcer_NilReceiverSafe(t *testing.T) {
	var e *DLQCapEnforcer
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	if !proceed || err != nil {
		t.Errorf("nil enforcer should proceed, got (%v, %v)", proceed, err)
	}
}

func TestDLQCapEnforcer_DefaultPolicyIsDropOldest(t *testing.T) {
	s := newFakeDLQStore()
	s.perJob["p:j"] = 10
	// Deliberately omit Policy.
	e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerJob: 10}, nil)
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	if !proceed || err != nil {
		t.Errorf("default policy should drop_oldest and proceed, got (%v, %v)", proceed, err)
	}
	if len(s.masked) != 1 {
		t.Errorf("default policy should mask; masked=%v", s.masked)
	}
}

func TestDLQCapEnforcer_InvalidPolicyNormalizes(t *testing.T) {
	s := newFakeDLQStore()
	s.perJob["p:j"] = 10
	e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerJob: 10, Policy: "nonsense"}, nil)
	proceed, _ := e.EnforceBeforeTransition(context.Background(), "p", "j")
	if !proceed {
		t.Error("invalid policy should normalize to drop_oldest")
	}
}

// FuzzDLQDepthBounds feeds random depth/cap pairs to the enforcer and
// asserts the invariant: proceed=true iff depth < cap OR policy is
// drop_oldest.
func FuzzDLQDepthBounds(f *testing.F) {
	f.Add(5, 10)
	f.Add(10, 10)
	f.Add(15, 10)
	f.Fuzz(func(t *testing.T, depth, cap int) {
		if depth < 0 || cap < 0 || depth > 1<<20 || cap > 1<<20 {
			return
		}
		s := newFakeDLQStore()
		s.perJob["p:j"] = depth
		e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerJob: cap, Policy: DLQOverflowReject}, nil)
		proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
		atCap := cap > 0 && depth >= cap
		if atCap {
			if proceed || err == nil {
				t.Errorf("depth=%d cap=%d expected reject, got (%v, %v)", depth, cap, proceed, err)
			}
		} else {
			if !proceed || err != nil {
				t.Errorf("depth=%d cap=%d expected proceed, got (%v, %v)", depth, cap, proceed, err)
			}
		}
	})
}
