package scheduler

import (
	"fmt"
	"testing"
	"time"
)

func TestPruneAlertCooldowns_DropsStaleEntries(t *testing.T) {
	t.Parallel()

	r := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, false, nil)

	now := time.Now()
	staleCutoff := now.Add(-25 * time.Hour)
	freshCutoff := now.Add(-5 * time.Minute)

	for i := range 500 {
		r.dlqAlertCooldown[fmt.Sprintf("stale-dlq-%d", i)] = staleCutoff
		r.queueAlertCooldown[fmt.Sprintf("stale-q-%d", i)] = staleCutoff
	}
	for i := range 500 {
		r.dlqAlertCooldown[fmt.Sprintf("fresh-dlq-%d", i)] = freshCutoff
		r.queueAlertCooldown[fmt.Sprintf("fresh-q-%d", i)] = freshCutoff
	}

	if got := len(r.dlqAlertCooldown); got != 1000 {
		t.Fatalf("dlqAlertCooldown pre-prune size = %d, want 1000", got)
	}
	if got := len(r.queueAlertCooldown); got != 1000 {
		t.Fatalf("queueAlertCooldown pre-prune size = %d, want 1000", got)
	}

	r.pruneAlertCooldowns(now)

	if got := len(r.dlqAlertCooldown); got != 500 {
		t.Fatalf("dlqAlertCooldown post-prune size = %d, want 500", got)
	}
	if got := len(r.queueAlertCooldown); got != 500 {
		t.Fatalf("queueAlertCooldown post-prune size = %d, want 500", got)
	}

	for i := range 500 {
		if _, ok := r.dlqAlertCooldown[fmt.Sprintf("stale-dlq-%d", i)]; ok {
			t.Fatalf("stale dlq entry %d should have been pruned", i)
		}
		if _, ok := r.queueAlertCooldown[fmt.Sprintf("stale-q-%d", i)]; ok {
			t.Fatalf("stale queue entry %d should have been pruned", i)
		}
		if _, ok := r.dlqAlertCooldown[fmt.Sprintf("fresh-dlq-%d", i)]; !ok {
			t.Fatalf("fresh dlq entry %d should have survived pruning", i)
		}
		if _, ok := r.queueAlertCooldown[fmt.Sprintf("fresh-q-%d", i)]; !ok {
			t.Fatalf("fresh queue entry %d should have survived pruning", i)
		}
	}
}

func TestPruneAlertCooldowns_DoesNotBlockNewAlerts(t *testing.T) {
	t.Parallel()

	r := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, false, nil)

	now := time.Now()
	r.dlqAlertCooldown["seen-job"] = now.Add(-100 * time.Hour)

	r.pruneAlertCooldowns(now)

	if _, ok := r.dlqAlertCooldown["seen-job"]; ok {
		t.Fatal("expected stale entry to be removed so a new alert can fire")
	}
}
