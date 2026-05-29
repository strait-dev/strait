//go:build integration

package queue_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
)

type queueEngineBehaviorCase struct {
	name         string
	build        func(t *testing.T) queue.Queue
	afterEnqueue func(t *testing.T, ctx context.Context, q queue.Queue)
}

func queueEngineBehaviorCases() []queueEngineBehaviorCase {
	return []queueEngineBehaviorCase{
		{
			name: "legacy",
			build: func(t *testing.T) queue.Queue {
				t.Helper()
				return queue.NewPostgresQueue(testDB.Pool)
			},
		},
		{
			name: "batchlog",
			build: func(t *testing.T) queue.Queue {
				t.Helper()
				return queue.NewBatchlogQueue(testDB.Pool, queue.NewPostgresQueue(testDB.Pool), queue.BatchlogConfig{
					TickInterval:  10 * time.Millisecond,
					LeaseDuration: time.Second,
					LeaseOwner:    "behavior-" + newID(),
				})
			},
			afterEnqueue: func(t *testing.T, ctx context.Context, q queue.Queue) {
				t.Helper()
				bq, ok := q.(*queue.BatchlogQueue)
				if !ok {
					t.Fatalf("queue = %T, want *BatchlogQueue", q)
				}
				if _, err := bq.SealDueBatches(ctx); err != nil {
					t.Fatalf("SealDueBatches: %v", err)
				}
			},
		},
	}
}

func TestQueueEngineBehavior_PriorityAndProjectIsolation(t *testing.T) {
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
				if err := q.Enqueue(ctx, run); err != nil {
					t.Fatalf("Enqueue(%s): %v", run.ID, err)
				}
			}
			if tc.afterEnqueue != nil {
				tc.afterEnqueue(t, ctx, q)
			}
			first, err := q.DequeueN(ctx, 1)
			if err != nil {
				t.Fatalf("DequeueN priority: %v", err)
			}
			if len(first) != 1 || first[0].ID != high.ID {
				t.Fatalf("priority dequeue = %+v, want high priority run %s", first, high.ID)
			}

			mustClean(t, ctx)
			jobA := mustCreateJob(t, ctx, st, "project-behavior-a-"+tc.name)
			jobB := mustCreateJob(t, ctx, st, "project-behavior-b-"+tc.name)
			projectRunA := &domain.JobRun{ID: newID(), JobID: jobA.ID, ProjectID: jobA.ProjectID, Priority: 1}
			projectRunB := &domain.JobRun{ID: newID(), JobID: jobB.ID, ProjectID: jobB.ProjectID, Priority: 10}
			for _, run := range []*domain.JobRun{projectRunA, projectRunB} {
				if err := q.Enqueue(ctx, run); err != nil {
					t.Fatalf("Enqueue(%s): %v", run.ID, err)
				}
			}
			if tc.afterEnqueue != nil {
				tc.afterEnqueue(t, ctx, q)
			}

			projectRuns, err := q.DequeueNByProject(ctx, 2, jobA.ProjectID)
			if err != nil {
				t.Fatalf("DequeueNByProject: %v", err)
			}
			if len(projectRuns) != 1 {
				t.Fatalf("DequeueNByProject len = %d, want 1", len(projectRuns))
			}
			for _, run := range projectRuns {
				if run.ProjectID != jobA.ProjectID {
					t.Fatalf("project run project_id = %q, want %q", run.ProjectID, jobA.ProjectID)
				}
			}
		})
	}
}

func TestQueueEngineBehavior_ConcurrentClaimsAreUnique(t *testing.T) {
	for _, tc := range queueEngineBehaviorCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			mustClean(t, ctx)
			st := mustStore(t)
			job := mustCreateJob(t, ctx, st, "project-behavior-concurrent-"+tc.name)
			q := tc.build(t)

			for range 50 {
				if err := q.Enqueue(ctx, &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}); err != nil {
					t.Fatalf("Enqueue: %v", err)
				}
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
							errCh <- errDuplicateClaim{runID: run.ID}
							return
						}
					}
				}()
			}
			wg.Wait()
			close(errCh)
			for err := range errCh {
				t.Fatalf("concurrent dequeue: %v", err)
			}
		})
	}
}
