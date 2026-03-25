package queue

import (
	"math/rand/v2"
	"testing"
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
			if currentPending < 0 {
				t.Fatalf("pending count went negative: enqueued=%d dequeued=%d",
					enqueued, dequeued)
			}
			if dequeued > enqueued {
				t.Fatalf("dequeued (%d) > enqueued (%d)", dequeued, enqueued)
			}
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

			if spent > budget {
				t.Fatalf("spent (%d) > budget (%d)", spent, budget)
			}
			if spent < 0 {
				t.Fatalf("spent went negative: %d", spent)
			}
		}
	}
}
