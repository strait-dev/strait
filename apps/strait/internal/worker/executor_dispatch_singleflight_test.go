package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

// TestResolveJobForRun_SingleflightDedupes asserts that a burst of concurrent
// resolveJobForRun calls for the same JobID coalesces into exactly one
// store.GetJob call. This prevents the cache-cold stampede that would otherwise
// hammer the DB when many runs for the same job are dispatched simultaneously.
func TestResolveJobForRun_SingleflightDedupes(t *testing.T) {
	t.Parallel()

	const goroutines = 100

	var (
		dbCalls atomic.Int64
		gate    = make(chan struct{})
	)
	mockStore := &mockExecutorStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			// Block until released so all goroutines pile up on the same
			// in-flight call. Without singleflight this would let every
			// goroutine call GetJob once it enters the critical section.
			<-gate
			dbCalls.Add(1)
			return &domain.Job{ID: id, Version: 1}, nil
		},
	}

	e := &Executor{
		store:    mockStore,
		jobCache: newTestJobCache(t, 5*time.Second),
	}

	ctx := context.Background()
	run := &domain.JobRun{ID: "run-x", JobID: "job-x", JobVersion: 1}

	var wg conc.WaitGroup
	var ready sync.WaitGroup
	ready.Add(goroutines)
	start := make(chan struct{})

	results := make([]*domain.Job, goroutines)
	errs := make([]error, goroutines)

	for i := range goroutines {
		wg.Go(func() {
			ready.Done()
			<-start
			job, err := e.resolveJobForRun(ctx, run)
			results[i] = job
			errs[i] = err
		})
	}

	ready.Wait()
	close(start)

	// Give all goroutines a chance to enter the singleflight Do call.
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()
	require.EqualValues(t, 1, dbCalls.
		Load(),
	)

	for i, err := range errs {
		require.NoError(
			t, err)
		require.False(t,
			results[i] == nil ||
				results[i].
					ID != "job-x",
		)

	}
}

// TestResolveJobForRun_DifferentJobIDs_NotDeduped asserts that singleflight
// only coalesces calls with matching keys; distinct JobIDs must each issue
// their own DB call.
func TestResolveJobForRun_DifferentJobIDs_NotDeduped(t *testing.T) {
	t.Parallel()

	const jobs = 25

	var dbCalls atomic.Int64
	mockStore := &mockExecutorStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			dbCalls.Add(1)
			return &domain.Job{ID: id, Version: 1}, nil
		},
	}

	e := &Executor{
		store:    mockStore,
		jobCache: newTestJobCache(t, 5*time.Second),
	}

	ctx := context.Background()

	var wg conc.WaitGroup
	for i := range jobs {
		wg.Go(func() {
			run := &domain.JobRun{
				ID:         "run-" + string(rune('a'+i)),
				JobID:      "job-" + string(rune('a'+i)),
				JobVersion: 1,
			}
			if _, err := e.resolveJobForRun(ctx, run); err != nil {
				assert.Failf(t, "test failure",

					"resolveJobForRun: %v", err)
			}
		})
	}
	wg.Wait()
	require.Equal(t,
		int64(jobs), dbCalls.
			Load())

}

// TestResolveJobForRun_PropagatesError asserts that when the underlying
// store.GetJob fails, the singleflight wrapper propagates the error to every
// coalesced caller and does not poison the cache.
func TestResolveJobForRun_PropagatesError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("get job failed")
	var dbCalls atomic.Int64
	mockStore := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			dbCalls.Add(1)
			return nil, sentinel
		},
	}

	e := &Executor{
		store:    mockStore,
		jobCache: newTestJobCache(t, 5*time.Second),
	}

	ctx := context.Background()
	run := &domain.JobRun{ID: "run-err", JobID: "job-err", JobVersion: 1}

	const goroutines = 10
	var wg conc.WaitGroup
	gotErrs := make([]error, goroutines)
	for i := range goroutines {
		wg.Go(func() {
			_, err := e.resolveJobForRun(ctx, run)
			gotErrs[i] = err
		})
	}
	wg.Wait()

	for _, err := range gotErrs {
		require.Error(t,
			err)
		require.True(t,
			errors.Is(
				err, sentinel,
			))

	}

	// Cache should not be poisoned: a subsequent successful call must hit
	// the DB and populate the cache cleanly.
	mockStore.getJobFn = func(_ context.Context, id string) (*domain.Job, error) {
		dbCalls.Add(1)
		return &domain.Job{ID: id, Version: 1}, nil
	}
	job, err := e.resolveJobForRun(ctx, run)
	require.NoError(
		t, err)
	require.False(t,
		job == nil ||
			job.
				ID != "job-err",
	)

}
