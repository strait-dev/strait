package queue

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

var pgQueQueueNameBenchmarkSink string
var pgQueWorkerRouteRefBenchmarkSink domain.WorkerQueueRef
var pgQueWorkerRouteRefOKBenchmarkSink bool
var pgQueRouteBenchmarkSink []string

func TestPgQueQueueNameDeterministicAndNotifySafe(t *testing.T) {
	routeKey := pgQueWorkerRouteKey(strings.Repeat("project", 20), strings.Repeat("queue", 20), strings.Repeat("env", 20))

	first := pgQueQueueName(routeKey)
	second := pgQueQueueName(routeKey)
	require.Equal(t,
		second,
		first)
	require.LessOrEqual(t, len(first), 57)
	require.True(t,
		strings.HasPrefix(first,
			pgQueQueuePrefix,
		))
	require.Equal(t,
		"stq_e0603c499aae47eb89343ad0ef3178e0",

		pgQueQueueName(pgQueHTTPRouteKey))

	workerRoute := pgQueWorkerRouteKey("project-a", "critical", "prod")
	require.Equal(t,
		"stq_27d44f587337af384a66c080216b17d5",

		pgQueQueueName(workerRoute))
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

func BenchmarkPgQueWorkerRouteRef(b *testing.B) {
	routeKey := pgQueWorkerRouteKey(
		strings.Repeat("project", 20),
		strings.Repeat("queue", 20),
		strings.Repeat("env", 20),
	)

	b.ReportAllocs()
	for b.Loop() {
		pgQueWorkerRouteRefBenchmarkSink, pgQueWorkerRouteRefOKBenchmarkSink = pgQueWorkerRouteRef(routeKey)
	}
}

func TestPgQueRouteKeyForRun(t *testing.T) {
	httpRun := &domain.JobRun{ProjectID: "project-a", ExecutionMode: domain.ExecutionModeHTTP}
	require.Equal(t,
		pgQueHTTPRouteKey,

		pgQueRouteKeyForRun(
			httpRun))

	workerRun := &domain.JobRun{ProjectID: "project-a", ExecutionMode: domain.ExecutionModeWorker, QueueName: "critical"}
	require.Equal(t,
		"worker:project-a:critical:",

		pgQueRouteKeyForRun(workerRun))
}

func TestPgQueWorkerRouteRef(t *testing.T) {
	tests := []struct {
		name     string
		routeKey string
		want     domain.WorkerQueueRef
		wantOK   bool
	}{
		{
			name:     "environment route",
			routeKey: "worker:project-a:critical:prod",
			want: domain.WorkerQueueRef{
				ProjectID:     "project-a",
				QueueName:     "critical",
				EnvironmentID: "prod",
			},
			wantOK: true,
		},
		{
			name:     "empty environment route",
			routeKey: "worker:project-a:critical:",
			want: domain.WorkerQueueRef{
				ProjectID: "project-a",
				QueueName: "critical",
			},
			wantOK: true,
		},
		{
			name:     "missing queue",
			routeKey: "worker:project-a::prod",
			wantOK:   false,
		},
		{
			name:     "missing project",
			routeKey: "worker::critical:prod",
			wantOK:   false,
		},
		{
			name:     "non worker route",
			routeKey: pgQueHTTPRouteKey,
			wantOK:   false,
		},
		{
			name:     "extra colon",
			routeKey: "worker:project-a:critical:prod:blue",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := pgQueWorkerRouteRef(tt.routeKey)
			require.Equal(t,
				tt.wantOK,
				ok)
			require.Equal(t,
				tt.want,
				got)
		})
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
			require.False(t,
				len(args) != 2 || args[0] != prefix ||

					args[1] != prefix+"%")

			return &stringRows{values: knownRoutes}, nil
		},
	}
	q := NewPgQueQueue(db, nil, PgQueConfig{})
	refs := []domain.WorkerQueueRef{{
		ProjectID: "project-a",
		QueueName: "priority",
	}}

	first, err := q.workerRouteKeys(ctx, refs)
	require.NoError(t, err)

	second, err := q.workerRouteKeys(ctx, refs)
	require.NoError(t, err)

	want := []string{knownRoutes[0], knownRoutes[1], prefix}
	require.True(t,
		slices.Equal(first,
			want))
	require.True(t,
		slices.Equal(second,
			want))
	require.Equal(t, 1, queryCount)
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
	require.NoError(t, err)
	require.True(t,
		slices.Equal(first,
			[]string{productionRoute,

				prefix}))
	require.NoError(t, q.ensureRoute(ctx,
		db, stagingRoute,

		pgQueQueueName(stagingRoute)))

	second, err := q.workerRouteKeys(ctx, refs)
	require.NoError(t, err)

	want := []string{productionRoute, stagingRoute, prefix}
	require.True(t,
		slices.Equal(second,
			want))
	require.Equal(t, 2, queryCount)
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
	require.NoError(t, err)
	require.True(t,
		slices.Equal(first,
			[]string{routeKey}),
	)

	refCacheKey := refs[0]
	refCacheKey.QueueName = runQueueName(refCacheKey.QueueName)
	q.routeMu.Lock()
	if _, ok := q.routeRefCache[refCacheKey]; !ok {
		q.routeMu.Unlock()
		require.Fail(t,

			"worker ref cache was not populated")
	}
	q.routeMu.Unlock()
	require.NoError(t, q.ensureRoute(ctx,
		db, routeKey,
		pgQueQueueName(routeKey)))

	q.routeMu.Lock()
	_, ok := q.routeRefCache[refCacheKey]
	q.routeMu.Unlock()
	require.False(t,
		ok)
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
			require.True(t,
				ok)

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
	require.NoError(t, err)

	second, err := q.workerRouteKeys(ctx, refs)
	require.NoError(t, err)

	want := []string{
		prefixA + "production",
		prefixA,
		prefixB + "staging",
		prefixB,
		pgQueWorkerRouteKey("project-c", "critical", "production"),
	}
	require.True(t,
		slices.Equal(first,
			want))
	require.True(t,
		slices.Equal(second,
			want))
	require.Equal(t, 2, queryCount)
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
	require.NoError(t, err)
	require.True(t,
		slices.Equal(first,
			[]string{productionRoute,

				prefix}))

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
	require.NoError(t, err)

	want := []string{productionRoute, stagingRoute, prefix}
	require.True(t,
		slices.Equal(second,
			want))
	require.Equal(t, 2, queryCount)
}
