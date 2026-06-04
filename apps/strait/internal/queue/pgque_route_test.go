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

var pgQueQueueNameBenchmarkSink string
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

	if got := pgQueQueueName(pgQueHTTPRouteKey); got != "stq_e0603c499aae47eb89343ad0ef3178e0" {
		t.Fatalf("http queue name = %q", got)
	}
	workerRoute := pgQueWorkerRouteKey("project-a", "critical", "prod")
	if got := pgQueQueueName(workerRoute); got != "stq_27d44f587337af384a66c080216b17d5" {
		t.Fatalf("worker queue name = %q", got)
	}
}

func BenchmarkPgQueQueueName(b *testing.B) {
	routeKey := pgQueWorkerRouteKey(
		strings.Repeat("project", 20),
		strings.Repeat("queue", 20),
		strings.Repeat("env", 20),
	)

	b.ReportAllocs()
	for b.Loop() {
		pgQueQueueNameBenchmarkSink = pgQueQueueName(routeKey)
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

func TestPgQueEnsureRouteInvalidatesExactWorkerRefCache(t *testing.T) {
	ctx := context.Background()
	routeKey := pgQueWorkerRouteKey("project-a", "priority", "production")
	db := &mockDBTX{}
	q := NewPgQueQueue(db, nil, PgQueConfig{})
	refs := []domain.WorkerQueueRef{{
		ProjectID:     "project-a",
		QueueName:     "priority",
		EnvironmentID: "production",
	}}

	first, err := q.workerRouteKeys(ctx, refs)
	if err != nil {
		t.Fatalf("workerRouteKeys first error = %v", err)
	}
	if !slices.Equal(first, []string{routeKey}) {
		t.Fatalf("first worker routes = %v, want %v", first, []string{routeKey})
	}

	refCacheKey := refs[0]
	refCacheKey.QueueName = runQueueName(refCacheKey.QueueName)
	q.routeMu.Lock()
	if _, ok := q.routeRefCache[refCacheKey]; !ok {
		q.routeMu.Unlock()
		t.Fatal("worker ref cache was not populated")
	}
	q.routeMu.Unlock()

	if err := q.ensureRoute(ctx, db, routeKey, pgQueQueueName(routeKey)); err != nil {
		t.Fatalf("ensureRoute error = %v", err)
	}

	q.routeMu.Lock()
	_, ok := q.routeRefCache[refCacheKey]
	q.routeMu.Unlock()
	if ok {
		t.Fatal("worker ref cache was not invalidated")
	}
}

func TestPgQueWorkerRouteKeysCachesMultiRefLookups(t *testing.T) {
	ctx := context.Background()
	prefixA := pgQueWorkerRouteKey("project-a", "priority", "")
	prefixB := pgQueWorkerRouteKey("project-b", "default", "")
	knownRoutes := map[string][]string{
		prefixA: {prefixA + "production"},
		prefixB: {prefixB + "staging"},
	}
	queryCount := 0
	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
			queryCount++
			prefix, ok := args[0].(string)
			if !ok {
				t.Fatalf("route lookup prefix arg = %T, want string", args[0])
			}
			return &stringRows{values: knownRoutes[prefix]}, nil
		},
	}
	q := NewPgQueQueue(db, nil, PgQueConfig{})
	refs := []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: "priority"},
		{ProjectID: "project-b", QueueName: "default"},
		{ProjectID: "project-c", QueueName: "critical", EnvironmentID: "production"},
	}

	first, err := q.workerRouteKeys(ctx, refs)
	if err != nil {
		t.Fatalf("workerRouteKeys first error = %v", err)
	}
	second, err := q.workerRouteKeys(ctx, refs)
	if err != nil {
		t.Fatalf("workerRouteKeys second error = %v", err)
	}

	want := []string{
		prefixA + "production",
		prefixA,
		prefixB + "staging",
		prefixB,
		pgQueWorkerRouteKey("project-c", "critical", "production"),
	}
	if !slices.Equal(first, want) {
		t.Fatalf("first worker routes = %v, want %v", first, want)
	}
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
	refCacheKey := refs[0]
	refCacheKey.QueueName = runQueueName(refCacheKey.QueueName)
	routeEntry := q.routeCache[prefix]
	routeEntry.expiresAt = time.Now().Add(time.Hour)
	q.routeCache[prefix] = routeEntry
	refEntry := q.routeRefCache[refCacheKey]
	refEntry.expiresAt = time.Now().Add(time.Hour)
	q.routeRefCache[refCacheKey] = refEntry
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

func BenchmarkPgQueWorkerRouteKeysMultiRefCached(b *testing.B) {
	ctx := context.Background()
	prefixA := pgQueWorkerRouteKey("project-a", "priority", "")
	prefixB := pgQueWorkerRouteKey("project-b", "default", "")
	knownRoutes := map[string][]string{
		prefixA: {
			prefixA + "production",
			prefixA + "staging",
		},
		prefixB: {
			prefixB + "production",
			prefixB + "canary",
		},
	}
	queryCount := 0
	allowQuery := true
	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
			if !allowQuery {
				b.Fatal("cached worker route lookup queried the database")
			}
			queryCount++
			prefix, ok := args[0].(string)
			if !ok {
				b.Fatalf("route lookup prefix arg = %T, want string", args[0])
			}
			return &stringRows{values: knownRoutes[prefix]}, nil
		},
	}
	q := NewPgQueQueue(db, nil, PgQueConfig{})
	refs := []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: "priority"},
		{ProjectID: "project-b", QueueName: "default"},
		{ProjectID: "project-c", QueueName: "critical", EnvironmentID: "production"},
	}

	if _, err := q.workerRouteKeys(ctx, refs); err != nil {
		b.Fatalf("workerRouteKeys warmup error = %v", err)
	}
	q.routeMu.Lock()
	for _, ref := range refs {
		ref.QueueName = runQueueName(ref.QueueName)
		entry := q.routeRefCache[ref]
		entry.expiresAt = time.Now().Add(time.Hour)
		q.routeRefCache[ref] = entry
		if ref.EnvironmentID == "" {
			prefix := pgQueWorkerRouteKey(ref.ProjectID, ref.QueueName, "")
			routeEntry := q.routeCache[prefix]
			routeEntry.expiresAt = time.Now().Add(time.Hour)
			q.routeCache[prefix] = routeEntry
		}
	}
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
	if queryCount != 2 {
		b.Fatalf("route lookup count = %d, want 2", queryCount)
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
	refCacheKey := refs[0]
	refCacheKey.QueueName = runQueueName(refCacheKey.QueueName)
	routeEntry := q.routeCache[prefix]
	routeEntry.expiresAt = time.Now().Add(-time.Second)
	q.routeCache[prefix] = routeEntry
	refEntry := q.routeRefCache[refCacheKey]
	refEntry.expiresAt = time.Now().Add(-time.Second)
	q.routeRefCache[refCacheKey] = refEntry
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
