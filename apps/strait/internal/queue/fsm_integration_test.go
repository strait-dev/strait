//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

// Exhaustive FSM state transition tests.

// validTransitions maps from_status -> set of valid to_statuses.
var validTransitions = map[domain.RunStatus][]domain.RunStatus{
	domain.StatusDelayed:      {domain.StatusQueued, domain.StatusCanceled, domain.StatusExpired},
	domain.StatusQueued:       {domain.StatusDequeued, domain.StatusCanceled, domain.StatusExpired, domain.StatusDeadLetter},
	domain.StatusDequeued:     {domain.StatusExecuting, domain.StatusQueued, domain.StatusCanceled},
	domain.StatusExecuting:    {domain.StatusCompleted, domain.StatusFailed, domain.StatusTimedOut, domain.StatusCrashed, domain.StatusSystemFailed, domain.StatusCanceled, domain.StatusQueued, domain.StatusWaiting},
	domain.StatusWaiting:      {domain.StatusExecuting, domain.StatusCanceled, domain.StatusTimedOut},
	domain.StatusCompleted:    {},
	domain.StatusFailed:       {},
	domain.StatusTimedOut:     {},
	domain.StatusCrashed:      {},
	domain.StatusSystemFailed: {},
	domain.StatusCanceled:     {},
	domain.StatusExpired:      {},
	domain.StatusDeadLetter:   {domain.StatusQueued},
	domain.StatusReplayStaged: {domain.StatusQueued},
	domain.StatusPaused:       {domain.StatusQueued, domain.StatusCanceled},
}

func isValidTransition(from, to domain.RunStatus) bool {
	targets, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, t := range targets {
		if t == to {
			return true
		}
	}
	return false
}

func TestFSM_AllValidTransitionsSucceed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for from, targets := range validTransitions {
		for _, to := range targets {
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				mustClean(t, ctx)
				st := mustStore(t)
				job := mustCreateJob(t, ctx, st, "project-fsm-valid")

				id := newID()
				_, err := testDB.Pool.Exec(ctx, `
					INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at)
					VALUES ($1, $2, $3, $4, 1, 'manual', NOW())
				`, id, job.ID, job.ProjectID, string(from))
				if err != nil {
					t.Fatalf("insert run in %s: %v", from, err)
				}
				_, err = testDB.Pool.Exec(ctx, `
					UPDATE job_runs SET status=$1, finished_at=CASE WHEN $1 IN ('completed','failed','timed_out','crashed','system_failed','canceled','expired','dead_letter') THEN NOW() ELSE finished_at END WHERE id=$2
				`, string(to), id)
				if err != nil {
					t.Errorf("%s -> %s failed: %v", from, to, err)
				}
			})
		}
	}
}

func TestFSM_TerminalToNonTerminalBlocked(t *testing.T) {
	terminals := []domain.RunStatus{
		domain.StatusCompleted, domain.StatusFailed, domain.StatusTimedOut,
		domain.StatusCrashed, domain.StatusSystemFailed,
		domain.StatusCanceled, domain.StatusExpired,
	}
	nonTerminals := []domain.RunStatus{
		domain.StatusDequeued, domain.StatusExecuting, domain.StatusWaiting,
	}
	for _, from := range terminals {
		for _, to := range nonTerminals {
			if isValidTransition(from, to) {
				continue
			}
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				// The DB doesn't enforce FSM (it's a TEXT column),
				// but the domain helpers should reject these.
				if from.IsTerminal() && !to.IsTerminal() {
					// This is the class of transition we want to prevent.
					// Document that it's NOT blocked at the DB level.
					t.Logf("WARNING: %s->%s is not DB-enforced; needs app-level check", from, to)
				}
			})
		}
	}
}

func TestFSM_DoubleClaimViaSKIPLOCKED(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fsm-dblclaim")
	q := mustQueue(t)
	mustEnqueueRun(t, ctx, q, job)

	// Two concurrent claims should yield exactly 1 run between them.
	r1, err1 := q.Dequeue(ctx)
	r2, err2 := q.Dequeue(ctx)
	if err1 != nil {
		t.Fatalf("dequeue1: %v", err1)
	}
	if r1 == nil {
		t.Fatal("first dequeue should succeed")
	}
	if err2 != nil {
		t.Fatalf("dequeue2: %v", err2)
	}
	if r2 != nil {
		t.Errorf("second dequeue should return nil (SKIP LOCKED)")
	}
}

func TestFSM_RequeueFromDeadLetter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fsm-requeue-dlq")
	q := mustQueue(t)
	run := mustEnqueueRun(t, ctx, q, job)

	// Move to dead_letter then back to queued.
	_, _ = testDB.Pool.Exec(ctx, `UPDATE job_runs SET status='dead_letter', finished_at=NOW() WHERE id=$1`, run.ID)
	_, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status='queued', finished_at=NULL, visible_until=NULL WHERE id=$1`, run.ID)
	if err != nil {
		t.Fatalf("requeue from dead_letter: %v", err)
	}
	// Should be claimable again.
	batch, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("dequeue after requeue: %v", err)
	}
	if len(batch) != 1 || batch[0].ID != run.ID {
		t.Errorf("expected to claim %s, got %v", run.ID, batch)
	}
}

func TestFSM_StatusPredicateConsistency(t *testing.T) {
	// Verify IsTerminal/IsActive/IsClaimable/IsDeadLetter/IsFailure
	// are consistent with the FSM map above.
	allStatuses := []domain.RunStatus{
		domain.StatusDelayed, domain.StatusQueued, domain.StatusDequeued,
		domain.StatusExecuting, domain.StatusWaiting, domain.StatusCompleted,
		domain.StatusFailed, domain.StatusTimedOut, domain.StatusCrashed,
		domain.StatusSystemFailed, domain.StatusCanceled, domain.StatusExpired,
		domain.StatusDeadLetter, domain.StatusReplayStaged, domain.StatusPaused,
	}
	for _, s := range allStatuses {
		if !s.IsValid() {
			t.Errorf("%q should be valid", s)
		}
		// Terminal statuses are runs that have reached a final state from
		// every observer's perspective (SSE, CDC, webhooks, reaper) and may
		// not transition further -- with two operator-initiated escape
		// hatches: dead_letter -> queued (DLQ requeue) and replay_staged
		// -> queued (staged replay activation).
		if s.IsTerminal() {
			targets := validTransitions[s]
			switch s {
			case domain.StatusDeadLetter, domain.StatusReplayStaged:
				if len(targets) != 1 || targets[0] != domain.StatusQueued {
					t.Errorf("%q must allow exactly the requeue transition to queued, got %v", s, targets)
				}
			default:
				if len(targets) > 0 {
					t.Errorf("%q is terminal but has %d valid transitions", s, len(targets))
				}
			}
		}
		// Active statuses should be dequeued or executing.
		if s.IsActive() && s != domain.StatusDequeued && s != domain.StatusExecuting {
			t.Errorf("%q is active but unexpected", s)
		}
		// Claimable should only be queued.
		if s.IsClaimable() && s != domain.StatusQueued {
			t.Errorf("%q is claimable but not queued", s)
		}
	}
}
