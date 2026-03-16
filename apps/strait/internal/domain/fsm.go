package domain

import (
	"slices"
)

var validTransitions = map[RunStatus][]RunStatus{
	StatusDelayed:      {StatusQueued, StatusCanceled, StatusExpired},
	StatusQueued:       {StatusDequeued, StatusCanceled, StatusExpired},
	StatusDequeued:     {StatusExecuting, StatusQueued, StatusCanceled, StatusSystemFailed},
	StatusExecuting:    {StatusCompleted, StatusFailed, StatusTimedOut, StatusCrashed, StatusCanceled, StatusWaiting, StatusQueued, StatusSystemFailed, StatusDeadLetter, StatusPaused},
	StatusWaiting:      {StatusExecuting, StatusCompleted, StatusFailed, StatusCanceled, StatusTimedOut},
	StatusCompleted:    {},
	StatusFailed:       {},
	StatusTimedOut:     {},
	StatusCrashed:      {},
	StatusSystemFailed: {},
	StatusCanceled:     {},
	StatusExpired:      {},
	StatusDeadLetter:   {StatusQueued, StatusReplayStaged},
	StatusReplayStaged: {StatusQueued, StatusCanceled},
	StatusPaused:       {StatusQueued, StatusCanceled},
}

func ValidateTransition(from, to RunStatus) error {
	transitions, ok := validTransitions[from]
	if !ok {
		return &UnknownStatusError{Status: from}
	}

	if slices.Contains(transitions, to) {
		return nil
	}

	return &TransitionError{From: from, To: to}
}

// Transition validates and returns an error if the transition is invalid.
// Prefer this over MustTransition in production code to avoid panics.
func Transition(from, to RunStatus) error {
	return ValidateTransition(from, to)
}
