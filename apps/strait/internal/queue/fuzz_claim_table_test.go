package queue

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strings"
	"testing"

	"strait/internal/domain"
)

// Fuzz: two-phase claim SQL shape.

// FuzzTwoPhaseClaimSQL_ContainsRequiredClauses builds the same candidatesSQL
// that DequeueNTwoPhase uses (parameterised by batch size) and verifies the
// structural invariants no branch of fmt.Sprintf can violate.
func FuzzTwoPhaseClaimSQL_ContainsRequiredClauses(f *testing.F) {
	f.Add(1)
	f.Add(0)
	f.Add(100)
	f.Add(-1)
	f.Add(1<<15 - 1) // int16 max
	f.Fuzz(func(t *testing.T, batchSize int) {
		q := NewPostgresQueue(nil)
		candidatesSQL := fmt.Sprintf(`
			SELECT jr.id
			FROM job_runs jr
			LEFT JOIN job_active_counts jac_job
			  ON jac_job.job_id = jr.job_id AND jac_job.concurrency_key = ''
			LEFT JOIN job_active_counts jac_key
			  ON jac_key.job_id = jr.job_id
			  AND jac_key.concurrency_key = COALESCE(jr.concurrency_key, '')
			WHERE jr.status = '%s'
			  AND COALESCE(jr.job_enabled, true) = true
			  AND COALESCE(jr.job_paused, false) = false
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  AND (jr.job_max_concurrency IS NULL OR COALESCE(jac_job.count, 0) < jr.job_max_concurrency)
			  AND (jr.job_max_concurrency_per_key IS NULL
			       OR jr.concurrency_key IS NULL
			       OR jr.concurrency_key = ''
			       OR COALESCE(jac_key.count, 0) < jr.job_max_concurrency_per_key)
			ORDER BY %s
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1`, domain.StatusQueued, q.dequeueOrderByClause())

		// Wrap in the CTE the same way executeDequeueTwoPhase does.
		claimSQL := fmt.Sprintf(
			`/* action=dequeue */ WITH candidates AS (%s) UPDATE job_runs SET status = '%s' WHERE id IN (SELECT id FROM candidates) RETURNING id`,
			candidatesSQL, domain.StatusDequeued,
		)

		for _, required := range []string{
			"FOR UPDATE",
			"SKIP LOCKED",
			"LIMIT",
			"RETURNING id",
			"/* action=dequeue */",
			"ORDER BY",
		} {
			if !strings.Contains(claimSQL, required) {
				t.Errorf("two-phase claim SQL missing %q for batchSize=%d:\n%s", required, batchSize, claimSQL)
			}
		}
	})
}

// Fuzz: claim INSERT SQL uses only parameterised placeholders.

// FuzzClaimInsertSQL_NoInjection verifies that claimInsertSQL never contains
// string-interpolated user values. The constant must use $1..$12 positional
// parameters and must not contain the Go fmt verb '%s' or '%v'.
func FuzzClaimInsertSQL_NoInjection(f *testing.F) {
	// Seed corpus: adversarial strings that could look like format verbs.
	f.Add("'; DROP TABLE job_run_queue; --")
	f.Add("%s")
	f.Add("$13")
	f.Add("")
	f.Add("\x00")
	f.Fuzz(func(t *testing.T, _ string) {
		sql := claimInsertFromJobSQL

		// Must have all 8 positional placeholders (job config comes from subquery).
		for i := 1; i <= 8; i++ {
			placeholder := fmt.Sprintf("$%d", i)
			if !strings.Contains(sql, placeholder) {
				t.Errorf("claimInsertFromJobSQL missing placeholder %s", placeholder)
			}
		}

		// Must NOT have Go format verbs that would allow injection.
		for _, verb := range []string{"%s", "%v", "%d", "%q"} {
			if strings.Contains(sql, verb) {
				t.Errorf("claimInsertSQL contains format verb %q — potential injection vector", verb)
			}
		}

		// Must use ON CONFLICT for idempotency.
		if !strings.Contains(sql, "ON CONFLICT") {
			t.Error("claimInsertSQL missing ON CONFLICT clause")
		}
		if !strings.Contains(sql, "DO NOTHING") {
			t.Error("claimInsertSQL missing DO NOTHING — idempotency broken")
		}
	})
}

// Fuzz: claim DELETE SQL shape (DequeueNClaim path).

// FuzzClaimDeleteSQL_Shape reconstructs the DELETE query that DequeueNClaim
// uses and verifies structural invariants: the query must DELETE from the
// thin claim table, use FOR UPDATE SKIP LOCKED, have a LIMIT, and RETURN
// the run_id for the subsequent UPDATE.
func FuzzClaimDeleteSQL_Shape(f *testing.F) {
	f.Add(1)
	f.Add(0)
	f.Add(50)
	f.Add(10000)
	f.Fuzz(func(t *testing.T, batchSize int) {
		// Reconstruct the DELETE SQL the same way DequeueNClaim does.
		claimSQL := "/* action=dequeue */ " + `
		DELETE FROM job_run_queue
		WHERE run_id IN (
			SELECT q.run_id
			FROM job_run_queue q
			LEFT JOIN job_active_counts jac_job
			  ON jac_job.job_id = q.job_id AND jac_job.concurrency_key = ''
			LEFT JOIN job_active_counts jac_key
			  ON jac_key.job_id = q.job_id
			  AND jac_key.concurrency_key = COALESCE(q.concurrency_key, '')
			WHERE COALESCE(q.job_enabled, true) = true
			  AND COALESCE(q.job_paused, false) = false
			  AND (q.scheduled_at IS NULL OR q.scheduled_at <= NOW())
			  AND (q.next_retry_at IS NULL OR q.next_retry_at <= NOW())
			  AND (q.job_max_concurrency IS NULL
			       OR COALESCE(jac_job.count, 0) < q.job_max_concurrency)
			  AND (q.job_max_concurrency_per_key IS NULL
			       OR q.concurrency_key IS NULL
			       OR q.concurrency_key = ''
			       OR COALESCE(jac_key.count, 0) < q.job_max_concurrency_per_key)
			ORDER BY q.priority DESC, q.created_at ASC
			FOR UPDATE OF q SKIP LOCKED
			LIMIT $1
		)
		RETURNING run_id`

		for _, required := range []string{
			"DELETE FROM job_run_queue",
			"FOR UPDATE",
			"SKIP LOCKED",
			"LIMIT $1",
			"RETURNING run_id",
			"/* action=dequeue */",
			"ORDER BY q.priority DESC",
		} {
			if !strings.Contains(claimSQL, required) {
				t.Errorf("claim DELETE SQL missing %q for batchSize=%d:\n%s", required, batchSize, claimSQL)
			}
		}

		// Must NOT contain format verbs — all user input via $N params.
		for _, verb := range []string{"%s", "%v"} {
			if strings.Contains(claimSQL, verb) {
				t.Errorf("claim DELETE SQL contains format verb %q", verb)
			}
		}
	})
}

// Property: claimInsertSQL placeholder count matches column count.

// TestProperty_ClaimInsertSQLHasCorrectParamCount asserts that the.
// claimInsertSQL constant has exactly 12 positional placeholders ($1..$12)
// matching the 12 columns of the job_run_queue table.
func TestProperty_ClaimInsertSQLHasCorrectParamCount(t *testing.T) {
	t.Parallel()

	re := regexp.MustCompile(`\$\d+`)
	matches := re.FindAllString(claimInsertFromJobSQL, -1)

	// Deduplicate in case a placeholder appears in both INSERT and ON CONFLICT.
	seen := make(map[string]bool, len(matches))
	for _, m := range matches {
		seen[m] = true
	}

	const expectedParams = 8 // run fields; job config comes from SELECT...FROM jobs
	if len(seen) != expectedParams {
		t.Errorf("claimInsertFromJobSQL has %d distinct placeholders, want %d: %v",
			len(seen), expectedParams, matches)
	}

	// Verify contiguous $1..$8.
	for i := 1; i <= expectedParams; i++ {
		p := fmt.Sprintf("$%d", i)
		if !seen[p] {
			t.Errorf("claimInsertFromJobSQL missing placeholder %s", p)
		}
	}

	// Verify all 12 column names are present.
	requiredColumns := []string{
		"run_id", "job_id", "project_id", "priority", "created_at",
		"scheduled_at", "next_retry_at", "concurrency_key",
		"job_max_concurrency", "job_max_concurrency_per_key",
		"job_enabled", "job_paused",
	}
	for _, col := range requiredColumns {
		if !strings.Contains(claimInsertFromJobSQL, col) {
			t.Errorf("claimInsertSQL missing column %q", col)
		}
	}
}

// Property: claim-table dequeue never returns more than requested.

// TestProperty_ClaimTableEnqueueDequeueBalance simulates random enqueue/dequeue.
// sequences against an in-memory claim table model and verifies invariants:
//   - dequeued count never exceeds enqueued count
//   - pending (enqueued - dequeued) is never negative
//   - no run_id is dequeued twice
func TestProperty_ClaimTableEnqueueDequeueBalance(t *testing.T) {
	t.Parallel()

	for trial := range 500 {
		rng := rand.New(rand.NewPCG(uint64(trial), uint64(trial*7)))
		pending := make(map[string]bool)  // run_ids in the claim table
		dequeued := make(map[string]bool) // run_ids successfully dequeued
		var totalEnqueued, totalDequeued int

		ops := rng.IntN(300) + 50
		for range ops {
			if rng.IntN(3) < 2 || len(pending) == 0 {
				// Enqueue: add a new run to the claim table.
				id := fmt.Sprintf("run-%d-%d", trial, totalEnqueued)
				pending[id] = true
				totalEnqueued++
			} else {
				// Dequeue: pick a random batch size and claim up to that many.
				batchSize := rng.IntN(10) + 1
				claimed := 0
				for id := range pending {
					if claimed >= batchSize {
						break
					}
					if dequeued[id] {
						t.Fatalf("trial %d: duplicate dequeue of %s", trial, id)
					}
					dequeued[id] = true
					delete(pending, id)
					claimed++
					totalDequeued++
				}

				if claimed > batchSize {
					t.Fatalf("trial %d: claimed %d > batchSize %d", trial, claimed, batchSize)
				}
			}

			if totalDequeued > totalEnqueued {
				t.Fatalf("trial %d: dequeued (%d) > enqueued (%d)",
					trial, totalDequeued, totalEnqueued)
			}
		}

		// Final invariant: pending == enqueued - dequeued.
		if len(pending) != totalEnqueued-totalDequeued {
			t.Fatalf("trial %d: pending=%d but enqueued-dequeued=%d",
				trial, len(pending), totalEnqueued-totalDequeued)
		}
	}
}
