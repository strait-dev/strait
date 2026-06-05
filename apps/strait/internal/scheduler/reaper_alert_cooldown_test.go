package scheduler

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	require.Len(t, r.dlqAlertCooldown, 1000)
	require.Len(t, r.queueAlertCooldown, 1000)

	r.pruneAlertCooldowns(now)
	require.Len(t, r.dlqAlertCooldown, 500)
	require.Len(t, r.queueAlertCooldown, 500)

	for i := range 500 {
		if _, ok := r.dlqAlertCooldown[fmt.Sprintf("stale-dlq-%d", i)]; ok {
			require.Failf(t, "test failure",

				"stale dlq entry %d should have been pruned", i)
		}
		if _, ok := r.queueAlertCooldown[fmt.Sprintf("stale-q-%d", i)]; ok {
			require.Failf(t, "test failure",

				"stale queue entry %d should have been pruned", i)
		}
		if _, ok := r.dlqAlertCooldown[fmt.Sprintf("fresh-dlq-%d", i)]; !ok {
			require.Failf(t, "test failure",

				"fresh dlq entry %d should have survived pruning", i)
		}
		if _, ok := r.queueAlertCooldown[fmt.Sprintf("fresh-q-%d", i)]; !ok {
			require.Failf(t, "test failure",

				"fresh queue entry %d should have survived pruning", i)
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
		require.Fail(t,

			"expected stale entry to be removed so a new alert can fire")
	}
}
