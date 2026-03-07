package domain

import (
	"slices"
)

var validTransitions = map[RunStatus][]RunStatus{
	StatusDelayed:      {StatusQueued, StatusCanceled, StatusExpired},
	StatusQueued:       {StatusDequeued, StatusCanceled, StatusExpired},
	StatusDequeued:     {StatusExecuting, StatusQueued, StatusCanceled, StatusSystemFailed},
	StatusExecuting:    {StatusCompleted, StatusFailed, StatusTimedOut, StatusCrashed, StatusCanceled, StatusWaiting, StatusQueued, StatusSystemFailed, StatusDeadLetter},
	StatusWaiting:      {StatusExecuting, StatusCompleted, StatusFailed, StatusCanceled, StatusTimedOut},
	StatusCompleted:    {},
	StatusFailed:       {},
	StatusTimedOut:     {},
	StatusCrashed:      {},
	StatusSystemFailed: {},
	StatusCanceled:     {},
	StatusExpired:      {},
	StatusDeadLetter:   {StatusQueued},
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

func MustTransition(from, to RunStatus) {
	if err := ValidateTransition(from, to); err != nil {
		panic(err)
	}
}
