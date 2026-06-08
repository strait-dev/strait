package domain

var validTransitions = map[RunStatus][]RunStatus{
	StatusDelayed:      {StatusQueued, StatusCanceled, StatusExpired},
	StatusQueued:       {StatusDequeued, StatusExecuting, StatusCanceled, StatusExpired},
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
	switch from {
	case StatusDelayed:
		if to == StatusQueued || to == StatusCanceled || to == StatusExpired {
			return nil
		}
	case StatusQueued:
		if to == StatusDequeued || to == StatusExecuting || to == StatusCanceled || to == StatusExpired {
			return nil
		}
	case StatusDequeued:
		if to == StatusExecuting || to == StatusQueued || to == StatusCanceled || to == StatusSystemFailed {
			return nil
		}
	case StatusExecuting:
		switch to {
		case StatusCompleted, StatusFailed, StatusTimedOut, StatusCrashed, StatusCanceled, StatusWaiting, StatusQueued, StatusSystemFailed, StatusDeadLetter, StatusPaused:
			return nil
		}
	case StatusWaiting:
		if to == StatusExecuting || to == StatusCompleted || to == StatusFailed || to == StatusCanceled || to == StatusTimedOut {
			return nil
		}
	case StatusCompleted, StatusFailed, StatusTimedOut, StatusCrashed, StatusSystemFailed, StatusCanceled, StatusExpired:
	case StatusDeadLetter:
		if to == StatusQueued || to == StatusReplayStaged {
			return nil
		}
	case StatusReplayStaged:
		if to == StatusQueued || to == StatusCanceled {
			return nil
		}
	case StatusPaused:
		if to == StatusQueued || to == StatusCanceled {
			return nil
		}
	default:
		return &UnknownStatusError{Status: from}
	}
	return &TransitionError{From: from, To: to}
}

// Transition validates and returns an error if the transition is invalid.
// Prefer this over MustTransition in production code to avoid panics.
func Transition(from, to RunStatus) error {
	return ValidateTransition(from, to)
}
