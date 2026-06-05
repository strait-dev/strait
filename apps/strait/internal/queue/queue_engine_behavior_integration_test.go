//go:build integration

package queue_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"

	"github.com/stretchr/testify/require"
)

type queueEngineBehaviorCase struct {
	name         string
	build        func(t *testing.T) queue.Queue
	afterEnqueue func(t *testing.T, ctx context.Context, q queue.Queue)
}

type duplicateClaimError struct {
	runID string
}

func (e duplicateClaimError) Error() string {
	return fmt.Sprintf("duplicate claim for run %s", e.runID)
}

func queueEngineBehaviorCases() []queueEngineBehaviorCase {
	return []queueEngineBehaviorCase{
		{
			name: "pgque",
			build: func(t *testing.T) queue.Queue {
				t.Helper()
				return queue.NewPgQueQueue(testDB.Pool, queue.NewPostgresRunWriter(testDB.Pool), queue.PgQueConfig{
					TickInterval:  10 * time.Millisecond,
					ConsumerName:  "behavior-" + newID(),
					ReceiveWindow: 100,
				})
			},
			afterEnqueue: func(t *testing.T, ctx context.Context, q queue.Queue) {
				t.Helper()
				pq, ok := q.(*queue.PgQueQueue)
				require.True(t, ok)
				require.NoError(t, pq.ForceTick(ctx,
					"http"),
				)

			},
		},
	}
}

func TestPgQueBehavior_PriorityAndProjectIsolation(t *testing.T) {
	for _, tc := range queueEngineBehaviorCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			mustClean(t, ctx)
			st := mustStore(t)
			priorityJob := mustCreateJob(t, ctx, st, "project-behavior-priority-"+tc.name)
			q := tc.build(t)

			low := &domain.JobRun{ID: newID(), JobID: priorityJob.ID, ProjectID: priorityJob.ProjectID, Priority: 1}
			high := &domain.JobRun{ID: newID(), JobID: priorityJob.ID, ProjectID: priorityJob.ProjectID, Priority: 9}
			for _, run := range []*domain.JobRun{low, high} {
				require.NoError(t, q.Enqueue(ctx,
					run))

			}
			if tc.afterEnqueue != nil {
				tc.afterEnqueue(t, ctx, q)
			}
			first, err := q.DequeueN(ctx, 1)
			require.NoError(t, err)
			require.False(t, len(first) != 1 ||
				first[0].
					ID !=
					high.ID)

			mustClean(t, ctx)
			q = tc.build(t)
			jobA := mustCreateJob(t, ctx, st, "project-behavior-a-"+tc.name)
			jobB := mustCreateJob(t, ctx, st, "project-behavior-b-"+tc.name)
			projectRunA := &domain.JobRun{ID: newID(), JobID: jobA.ID, ProjectID: jobA.ProjectID, Priority: 1}
			projectRunB := &domain.JobRun{ID: newID(), JobID: jobB.ID, ProjectID: jobB.ProjectID, Priority: 10}
			for _, run := range []*domain.JobRun{projectRunA, projectRunB} {
				require.NoError(t, q.Enqueue(ctx,
					run))

			}
			if tc.afterEnqueue != nil {
				tc.afterEnqueue(t, ctx, q)
			}

			projectRuns, err := q.DequeueNByProject(ctx, 2, jobA.ProjectID)
			require.NoError(t, err)
			require.Len(t, projectRuns,

				1)

			for _, run := range projectRuns {
				require.Equal(t, jobA.ProjectID,

					run.ProjectID,
				)

			}
		})
	}
}

func TestPgQueBehavior_ConcurrentClaimsAreUnique(t *testing.T) {
	for _, tc := range queueEngineBehaviorCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			mustClean(t, ctx)
			st := mustStore(t)
			job := mustCreateJob(t, ctx, st, "project-behavior-concurrent-"+tc.name)
			q := tc.build(t)

			for range 50 {
				require.NoError(t, q.Enqueue(ctx,
					&domain.JobRun{ID: newID(), JobID: job.
						ID, ProjectID: job.ProjectID}))

			}
			if tc.afterEnqueue != nil {
				tc.afterEnqueue(t, ctx, q)
			}

			seen := sync.Map{}
			errCh := make(chan error, 5)
			var wg sync.WaitGroup
			for range 5 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					runs, err := q.DequeueN(ctx, 10)
					if err != nil {
						errCh <- err
						return
					}
					for _, run := range runs {
						if _, loaded := seen.LoadOrStore(run.ID, true); loaded {
							errCh <- duplicateClaimError{runID: run.ID}
							return
						}
					}
				}()
			}
			wg.Wait()
			close(errCh)
			for err := range errCh {
				require.Failf(t, "test failure",

					"concurrent dequeue: %v", err)
			}
		})
	}
}
