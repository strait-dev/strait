package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func runWithOptionalAdvisoryLock(ctx context.Context, locker AdvisoryLocker, lockID int64, fn func(context.Context) error) (acquired bool, err error) {
	if fn == nil {
		return false, fmt.Errorf("advisory lock %d: fn is nil", lockID)
	}
	if locker == nil {
		return true, fn(ctx)
	}
	if runner, ok := locker.(AdvisoryLockRunner); ok {
		return runner.RunWithAdvisoryLock(ctx, lockID, fn)
	}

	acquired, err = locker.TryAdvisoryLock(ctx, lockID)
	if err != nil || !acquired {
		return acquired, err
	}

	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if releaseErr := locker.ReleaseAdvisoryLock(releaseCtx, lockID); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release advisory lock %d: %w", lockID, releaseErr))
		}
	}()

	return true, fn(ctx)
}
