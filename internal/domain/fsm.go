package domain

import (
	"fmt"
	"slices"
)

var validTransitions = map[RunStatus][]RunStatus{
	StatusDelayed:      {StatusQueued, StatusCanceled, StatusExpired},
	StatusQueued:       {StatusDequeued, StatusCanceled, StatusExpired},
	StatusDequeued:     {StatusExecuting, StatusQueued, StatusCanceled, StatusSystemFailed},
	StatusExecuting:    {StatusCompleted, StatusFailed, StatusTimedOut, StatusCrashed, StatusCanceled, StatusWaiting, StatusQueued, StatusSystemFailed},
	StatusWaiting:      {StatusExecuting, StatusCompleted, StatusFailed, StatusCanceled, StatusTimedOut},
	StatusCompleted:    {},
	StatusFailed:       {},
	StatusTimedOut:     {},
	StatusCrashed:      {},
	StatusSystemFailed: {},
	StatusCanceled:     {},
	StatusExpired:      {},
}

func ValidateTransition(from, to RunStatus) error {
	transitions, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("unknown from status: %s", from)
	}

	if slices.Contains(transitions, to) {
		return nil
	}

	return fmt.Errorf("invalid transition: %s -> %s", from, to)
}

func MustTransition(from, to RunStatus) {
	if err := ValidateTransition(from, to); err != nil {
		panic(err)
	}
}
