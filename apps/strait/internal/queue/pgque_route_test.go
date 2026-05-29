package queue

import (
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestPgQueQueueNameDeterministicAndNotifySafe(t *testing.T) {
	routeKey := pgQueWorkerRouteKey(strings.Repeat("project", 20), strings.Repeat("queue", 20), strings.Repeat("env", 20))

	first := pgQueQueueName(routeKey)
	second := pgQueQueueName(routeKey)
	if first != second {
		t.Fatalf("queue name changed: %q != %q", first, second)
	}
	if len(first) > 57 {
		t.Fatalf("queue name length = %d, want <= 57", len(first))
	}
	if !strings.HasPrefix(first, pgQueQueuePrefix) {
		t.Fatalf("queue name = %q, want prefix %q", first, pgQueQueuePrefix)
	}
}

func TestPgQueRouteKeyForRun(t *testing.T) {
	httpRun := &domain.JobRun{ProjectID: "project-a", ExecutionMode: domain.ExecutionModeHTTP}
	if got := pgQueRouteKeyForRun(httpRun); got != pgQueHTTPRouteKey {
		t.Fatalf("http route = %q, want %q", got, pgQueHTTPRouteKey)
	}

	workerRun := &domain.JobRun{ProjectID: "project-a", ExecutionMode: domain.ExecutionModeWorker, QueueName: "critical"}
	if got := pgQueRouteKeyForRun(workerRun); got != "worker:project-a:critical:" {
		t.Fatalf("worker route = %q", got)
	}
}
