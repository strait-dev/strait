package scheduler

import (
	"context"
	"errors"
	"fmt"
)

func runWithOptionalAdvisoryLock(ctx context.Context, locker AdvisoryLocker, lockID int64, fn func(context.Context) error) (bool, error) {
	if fn == nil {
		return false, fmt.Errorf("advisory lock %d: fn is nil", lockID)
	}
	if locker == nil {
		return true, fn(ctx)
	}
	if runner, ok := locker.(AdvisoryLockRunner); ok {
		return runner.RunWithAdvisoryLock(ctx, lockID, fn)
	}

	acquired, err := locker.TryAdvisoryLock(ctx, lockID)
	if err != nil || !acquired {
		return acquired, err
	}

	runErr := fn(ctx)
	if releaseErr := locker.ReleaseAdvisoryLock(ctx, lockID); releaseErr != nil {
		return true, errors.Join(runErr, fmt.Errorf("release advisory lock %d: %w", lockID, releaseErr))
	}
	return true, runErr
}
