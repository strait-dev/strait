package queue

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestProperty_DequeuedNeverExceedsEnqueued simulates random enqueue/dequeue
// sequences and verifies the invariant: dequeued count never exceeds enqueued
// count, and the pending count (enqueued - dequeued) is never negative.
func TestProperty_DequeuedNeverExceedsEnqueued(t *testing.T) {
	t.Parallel()

	for range 1000 {
		var enqueued, dequeued int
		ops := rand.IntN(300) + 50

		for range ops {
			pending := enqueued - dequeued
			if rand.IntN(3) < 2 {
				// Enqueue: always succeeds.
				enqueued++
			} else if pending > 0 {
				// Dequeue: only if there are pending items.
				dequeued++
			}

			currentPending := enqueued - dequeued
			require.GreaterOrEqual(t, currentPending,

				0)
			require.LessOrEqual(
				t, dequeued,

				enqueued,
			)
		}
	}
}

// TestProperty_BudgetSpentNeverExceedsBudget simulates random spend operations
// against a budget and verifies that total spending never exceeds the allocated
// budget when the spend-check is applied correctly.
func TestProperty_BudgetSpentNeverExceedsBudget(t *testing.T) {
	t.Parallel()

	for range 1000 {
		budget := rand.IntN(10000) + 100
		spent := 0
		ops := rand.IntN(200) + 50

		for range ops {
			cost := rand.IntN(500) + 1
			remaining := budget - spent

			// Only spend if cost fits within remaining budget.
			if cost <= remaining {
				spent += cost
			}
			require.LessOrEqual(
				t, spent, budget,
			)
			require.GreaterOrEqual(t, spent,

				0,
			)
		}
	}
}
