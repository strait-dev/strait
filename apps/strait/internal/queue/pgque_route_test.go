package queue

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
)

var pgQueRouteBenchmarkSink []string

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

func TestPgQueWorkerRouteKeysCachesWildcardLookup(t *testing.T) {
	ctx := context.Background()
	prefix := pgQueWorkerRouteKey("project-a", "priority", "")
	knownRoutes := []string{
		prefix + "production",
		prefix + "staging",
	}
	queryCount := 0
	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
			queryCount++
			if len(args) != 2 || args[0] != prefix || args[1] != prefix+"%" {
				t.Fatalf("route lookup args = %#v, want prefix lookup", args)
			}
			return &stringRows{values: knownRoutes}, nil
		},
	}
	q := NewPgQueQueue(db, nil, PgQueConfig{})
	refs := []domain.WorkerQueueRef{{
		ProjectID: "project-a",
		QueueName: "priority",
	}}

	first, err := q.workerRouteKeys(ctx, refs)
	if err != nil {
		t.Fatalf("workerRouteKeys first error = %v", err)
	}
	second, err := q.workerRouteKeys(ctx, refs)
	if err != nil {
		t.Fatalf("workerRouteKeys second error = %v", err)
	}

	want := []string{knownRoutes[0], knownRoutes[1], prefix}
	if !slices.Equal(first, want) {
		t.Fatalf("first worker routes = %v, want %v", first, want)
	}
	if !slices.Equal(second, want) {
		t.Fatalf("second worker routes = %v, want %v", second, want)
	}
	if queryCount != 1 {
		t.Fatalf("route lookup count = %d, want 1", queryCount)
	}
}

func TestPgQueEnsureRouteInvalidatesWorkerRouteCache(t *testing.T) {
	ctx := context.Background()
	prefix := pgQueWorkerRouteKey("project-a", "priority", "")
	productionRoute := prefix + "production"
	stagingRoute := prefix + "staging"
	routeSnapshots := [][]string{
		{productionRoute},
		{productionRoute, stagingRoute},
	}
	queryCount := 0
	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			idx := min(queryCount, len(routeSnapshots)-1)
			queryCount++
			return &stringRows{values: routeSnapshots[idx]}, nil
		},
	}
	q := NewPgQueQueue(db, nil, PgQueConfig{})
	refs := []domain.WorkerQueueRef{{
		ProjectID: "project-a",
		QueueName: "priority",
	}}

	first, err := q.workerRouteKeys(ctx, refs)
	if err != nil {
		t.Fatalf("workerRouteKeys first error = %v", err)
	}
	if !slices.Equal(first, []string{productionRoute, prefix}) {
		t.Fatalf("first worker routes = %v", first)
	}

	if err := q.ensureRoute(ctx, db, stagingRoute, pgQueQueueName(stagingRoute)); err != nil {
		t.Fatalf("ensureRoute error = %v", err)
	}

	second, err := q.workerRouteKeys(ctx, refs)
	if err != nil {
		t.Fatalf("workerRouteKeys second error = %v", err)
	}
	want := []string{productionRoute, stagingRoute, prefix}
	if !slices.Equal(second, want) {
		t.Fatalf("second worker routes = %v, want %v", second, want)
	}
	if queryCount != 2 {
		t.Fatalf("route lookup count = %d, want 2", queryCount)
	}
}

func BenchmarkPgQueWorkerRouteKeysWildcardCached(b *testing.B) {
	ctx := context.Background()
	prefix := pgQueWorkerRouteKey("project-a", "priority", "")
	knownRoutes := []string{
		prefix + "production",
		prefix + "staging",
		prefix + "canary",
	}
	queryCount := 0
	allowQuery := true
	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			if !allowQuery {
				b.Fatal("cached worker route lookup queried the database")
			}
			queryCount++
			return &stringRows{values: knownRoutes}, nil
		},
	}
	q := NewPgQueQueue(db, nil, PgQueConfig{})
	refs := []domain.WorkerQueueRef{{
		ProjectID: "project-a",
		QueueName: "priority",
	}}

	if _, err := q.workerRouteKeys(ctx, refs); err != nil {
		b.Fatalf("workerRouteKeys warmup error = %v", err)
	}
	q.routeMu.Lock()
	entry := q.routeCache[prefix]
	entry.expiresAt = time.Now().Add(time.Hour)
	q.routeCache[prefix] = entry
	q.routeMu.Unlock()
	allowQuery = false
	b.ReportAllocs()
	for b.Loop() {
		routes, err := q.workerRouteKeys(ctx, refs)
		if err != nil {
			b.Fatalf("workerRouteKeys cached error = %v", err)
		}
		pgQueRouteBenchmarkSink = routes
	}
	if queryCount != 1 {
		b.Fatalf("route lookup count = %d, want 1", queryCount)
	}
}

func TestPgQueWorkerRouteKeysReloadsExpiredWildcardCache(t *testing.T) {
	ctx := context.Background()
	prefix := pgQueWorkerRouteKey("project-a", "priority", "")
	productionRoute := prefix + "production"
	stagingRoute := prefix + "staging"
	routeSnapshots := [][]string{
		{productionRoute},
		{productionRoute, stagingRoute},
	}
	queryCount := 0
	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			idx := min(queryCount, len(routeSnapshots)-1)
			queryCount++
			return &stringRows{values: routeSnapshots[idx]}, nil
		},
	}
	q := NewPgQueQueue(db, nil, PgQueConfig{})
	refs := []domain.WorkerQueueRef{{
		ProjectID: "project-a",
		QueueName: "priority",
	}}

	first, err := q.workerRouteKeys(ctx, refs)
	if err != nil {
		t.Fatalf("workerRouteKeys first error = %v", err)
	}
	if !slices.Equal(first, []string{productionRoute, prefix}) {
		t.Fatalf("first worker routes = %v", first)
	}

	q.routeMu.Lock()
	entry := q.routeCache[prefix]
	entry.expiresAt = time.Now().Add(-time.Second)
	q.routeCache[prefix] = entry
	q.routeMu.Unlock()

	second, err := q.workerRouteKeys(ctx, refs)
	if err != nil {
		t.Fatalf("workerRouteKeys second error = %v", err)
	}
	want := []string{productionRoute, stagingRoute, prefix}
	if !slices.Equal(second, want) {
		t.Fatalf("second worker routes = %v, want %v", second, want)
	}
	if queryCount != 2 {
		t.Fatalf("route lookup count = %d, want 2", queryCount)
	}
}
