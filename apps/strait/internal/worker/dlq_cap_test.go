package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Unit tests for the DLQ cap enforcer.

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
	assert.False(t,
		!proceed ||
			err != nil,
	)

}

func TestDLQCapEnforcer_UnderCapProceeds(t *testing.T) {
	s := newFakeDLQStore()
	s.perJob["p:j"] = 5
	s.perProject["p"] = 5
	e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerJob: 10, MaxPerProject: 100, Policy: DLQOverflowReject}, nil)
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	assert.False(t,
		!proceed ||
			err != nil,
	)

}

func TestDLQCapEnforcer_PerJobRejectAtCap(t *testing.T) {
	s := newFakeDLQStore()
	s.perJob["p:j"] = 10
	e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerJob: 10, Policy: DLQOverflowReject}, nil)
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	assert.False(t,
		proceed)
	assert.True(t, errors.Is(err,
		ErrDLQOverflow,
	))
	assert.EqualValues(t, 1, e.OverflowCount())

}

func TestDLQCapEnforcer_PerProjectRejectAtCap(t *testing.T) {
	s := newFakeDLQStore()
	s.perProject["p"] = 100
	e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerProject: 100, Policy: DLQOverflowReject}, nil)
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	assert.False(t,
		proceed ||
			!errors.Is(err,

				ErrDLQOverflow))

}

func TestDLQCapEnforcer_DropOldestMasksAndProceeds(t *testing.T) {
	s := newFakeDLQStore()
	s.perJob["p:j"] = 10
	e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerJob: 10, Policy: DLQOverflowDropOldest}, nil)
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	assert.False(t,
		!proceed ||
			err != nil,
	)
	assert.EqualValues(t, 1, e.DroppedCount())
	assert.Len(t, s.
		masked, 1)

}

func TestDLQCapEnforcer_StoreErrorFailsOpen(t *testing.T) {
	s := newFakeDLQStore()
	s.err = errors.New("pg down")
	e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerJob: 10, Policy: DLQOverflowReject}, nil)
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	assert.True(t, proceed)
	assert.Error(t,
		err)

}

func TestDLQCapEnforcer_NilReceiverSafe(t *testing.T) {
	var e *DLQCapEnforcer
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	assert.False(t,
		!proceed ||
			err != nil,
	)

}

func TestDLQCapEnforcer_DefaultPolicyIsDropOldest(t *testing.T) {
	s := newFakeDLQStore()
	s.perJob["p:j"] = 10
	// Deliberately omit Policy.
	e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerJob: 10}, nil)
	proceed, err := e.EnforceBeforeTransition(context.Background(), "p", "j")
	assert.False(t,
		!proceed ||
			err != nil,
	)
	assert.Len(t, s.
		masked, 1)

}

func TestDLQCapEnforcer_InvalidPolicyNormalizes(t *testing.T) {
	s := newFakeDLQStore()
	s.perJob["p:j"] = 10
	e := NewDLQCapEnforcer(s, DLQCapConfig{MaxPerJob: 10, Policy: "nonsense"}, nil)
	proceed, _ := e.EnforceBeforeTransition(context.Background(), "p", "j")
	assert.True(t, proceed)

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
			assert.False(t,
				proceed ||
					err == nil,
			)

		} else {
			assert.False(t,
				!proceed ||
					err != nil,
			)

		}
	})
}
