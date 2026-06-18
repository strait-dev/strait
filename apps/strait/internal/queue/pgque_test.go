package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

var pgQueCandidateBenchmarkSink []pgQueCandidate
var pgQueWorkerRefArgsBenchmarkSink struct {
	projectIDs     []string
	queueNames     []string
	environmentIDs []string
}
var pgQueClaimSelectionBenchmarkSink pgQueClaimSelection
var pgQueCandidateRunIDBenchmarkSink string
var pgQueReadyEmitBatchErrBenchmarkSink error
var pgQueReadyRunsBenchmarkSink []pgQueReadyRun
var pgQueSendReadyEventsErrBenchmarkSink error
var pgQueScanWorkerRoutesBenchmarkSink []domain.JobRun
var pgQueWorkerRefsBenchmarkSink []domain.WorkerQueueRef

func TestPgQueFinishBatchReservationReopensAfterAckFailure(t *testing.T) {
	ctx := context.Background()
	ackErr := errors.New("temporary ack failure")
	ackAttempts := 0
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if !strings.Contains(sql, "pgque.ack") {
				return pgconn.CommandTag{}, nil
			}
			ackAttempts++
			if ackAttempts == 1 {
				return pgconn.CommandTag{}, ackErr
			}
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	state := &pgQueRouteState{
		activeBatch: &pgQueActiveBatch{
			BatchID:  42,
			InFlight: 1,
		},
	}
	batch := state.activeBatch

	err := q.finishBatchReservation(ctx, state, batch, nil)
	require.Error(t, err)
	require.ErrorIs(
		t, err, ackErr)

	state.mu.Lock()
	activeBatch := state.activeBatch
	inFlight := batch.InFlight
	closing := batch.Closing
	state.mu.Unlock()
	require.Equal(t, batch,
		activeBatch)
	require.Equal(t, 0, inFlight)
	require.False(t, closing)
	require.NoError(t, q.finishBatchReservation(ctx, state,
		batch,
		nil))

	state.mu.Lock()
	activeBatch = state.activeBatch
	state.mu.Unlock()
	require.Nil(t, activeBatch)
	require.Equal(t, 2, ackAttempts)
}

func TestPgQueMaintainRunsRotationPhases(t *testing.T) {
	ctx := context.Background()
	type execCall struct {
		sql  string
		args []any
	}
	var calls []execCall
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			require.Contains(
				t, sql, "pgque.maint_operations()")

			return &stringRows{values: []string{"stq_a", "stq_b"}}, nil
		},
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			calls = append(calls, execCall{sql: sql, args: args})
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	require.NoError(t, q.Maintain(ctx))
	require.Len(t,
		calls, 3)
	require.False(t, !strings.Contains(calls[0].sql, "pgque.maint_rotate_tables_step1") ||
		calls[0].
			args[0] !=
			"stq_a")
	require.False(t, !strings.Contains(calls[1].sql, "pgque.maint_rotate_tables_step1") ||
		calls[1].
			args[0] !=
			"stq_b")
	require.Contains(
		t, calls[2].
			sql, "pgque.maint_rotate_tables_step2()")
}

func TestPgQuePrepareRouteForJobWarmsFreshWorkerRoute(t *testing.T) {
	ctx := context.Background()
	var calls []string
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			calls = append(calls, sql)
			return pgconn.CommandTag{}, nil
		},
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			require.Contains(t, sql, "FROM jobs")
			return &mockRow{scanFn: func(dest ...any) error {
				require.Len(t, dest, 2)
				*dest[0].(*string) = "tx-worker"
				*dest[1].(*string) = "env-1"
				return nil
			}}
		},
	}

	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	job := &domain.Job{
		ID:            "job-1",
		ProjectID:     "project-1",
		ExecutionMode: domain.ExecutionModeWorker,
		Queue:         "tx-worker",
		EnvironmentID: "env-1",
	}
	routeKey := pgQueWorkerRouteKey(job.ProjectID, job.Queue, job.EnvironmentID)

	require.NoError(t, q.PrepareRouteForJob(ctx, job))
	require.Len(t, calls, 5)
	joined := strings.Join(calls, "\n")
	require.Contains(t, joined, "INSERT INTO strait_pgque_routes")
	require.Contains(t, joined, "pgque.create_queue")
	require.True(t, q.routeConfigured(routeKey))
}

func TestPgQueMaintainWrapsPhaseErrors(t *testing.T) {
	ctx := context.Background()
	operationsErr := errors.New("operations failed")
	step1Err := errors.New("step1 failed")
	rotateErr := errors.New("rotate failed")

	tests := []struct {
		name     string
		queryErr error
		execFn   func(sql string) error
		wantErr  error
	}{
		{
			name:     "operations",
			queryErr: operationsErr,
			wantErr:  operationsErr,
		},
		{
			name: "rotate step1",
			execFn: func(sql string) error {
				if strings.Contains(sql, "pgque.maint_rotate_tables_step1") {
					return step1Err
				}
				return nil
			},
			wantErr: step1Err,
		},
		{
			name: "rotate step2",
			execFn: func(sql string) error {
				if strings.Contains(sql, "pgque.maint_rotate_tables_step2()") {
					return rotateErr
				}
				return nil
			},
			wantErr: rotateErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockDBTX{
				queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
					if tt.queryErr != nil {
						return nil, tt.queryErr
					}
					return &stringRows{values: []string{"stq_a"}}, nil
				},
				execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
					if tt.execFn == nil {
						return pgconn.CommandTag{}, nil
					}
					return pgconn.CommandTag{}, tt.execFn(sql)
				},
			}
			q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

			err := q.Maintain(ctx)
			require.ErrorIs(
				t, err, tt.wantErr)
		})
	}
}

func TestPgQueNextWorkerRouteStartRotates(t *testing.T) {
	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{})
	got := []int{
		q.nextWorkerRouteStart(3),
		q.nextWorkerRouteStart(3),
		q.nextWorkerRouteStart(3),
		q.nextWorkerRouteStart(3),
		q.nextWorkerRouteStart(3),
	}
	want := []int{0, 1, 2, 0, 1}
	require.True(
		t, slices.Equal(got, want))
}

func TestPgQueNextWorkerRouteStartHandlesSmallRouteCounts(t *testing.T) {
	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{})
	require.Equal(t, 0, q.nextWorkerRouteStart(0))
	require.Equal(t, 0, q.nextWorkerRouteStart(1))
	require.Equal(t, 0, q.nextWorkerRouteStart(2))
	require.Equal(t, 1, q.nextWorkerRouteStart(2))
}

func TestPgQueScanWorkerRoutesRotatesAcrossManyRoutes(t *testing.T) {
	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{})
	routes := []string{"route-a", "route-b", "route-c", "route-d"}
	var firstScan, secondScan []string

	if _, err := q.scanWorkerRoutes(routes, 1, func(routeKey string, _ int) ([]domain.JobRun, error) {
		firstScan = append(firstScan, routeKey)
		return nil, nil
	}); err != nil {
		require.Failf(t, "test failure",

			"first scanWorkerRoutes error = %v", err)
	}
	if _, err := q.scanWorkerRoutes(routes, 1, func(routeKey string, _ int) ([]domain.JobRun, error) {
		secondScan = append(secondScan, routeKey)
		return nil, nil
	}); err != nil {
		require.Failf(t, "test failure",

			"second scanWorkerRoutes error = %v", err)
	}

	if want := []string{"route-a", "route-b", "route-c", "route-d"}; !slices.Equal(firstScan, want) {
		require.Failf(t, "test failure",

			"first route scan = %v, want %v", firstScan, want)
	}
	if want := []string{"route-b", "route-c", "route-d", "route-a"}; !slices.Equal(secondScan, want) {
		require.Failf(t, "test failure",

			"second route scan = %v, want %v", secondScan, want)
	}
}

func TestPgQueScanWorkerRoutesStopsAtCapacity(t *testing.T) {
	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{})
	routes := []string{"route-a", "route-b", "route-c"}
	var scanned []string

	claimed, err := q.scanWorkerRoutes(routes, 2, func(routeKey string, remaining int) ([]domain.JobRun, error) {
		scanned = append(scanned, routeKey)
		require.Equal(t, 2, remaining)

		return []domain.JobRun{
			{ID: "run-a"},
			{ID: "run-b"},
		}, nil
	})
	require.NoError(t, err)
	require.Len(t,
		claimed,
		2)

	if want := []string{"route-a"}; !slices.Equal(scanned, want) {
		require.Failf(t, "test failure",

			"scanned routes = %v, want %v", scanned, want)
	}
}

func TestPgQueScanWorkerRoutesSingleRouteReturnsBatch(t *testing.T) {
	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{})
	batch := []domain.JobRun{{ID: "run-a"}, {ID: "run-b"}}
	scans := 0

	claimed, err := q.scanWorkerRoutes([]string{"route-a"}, 2, func(routeKey string, remaining int) ([]domain.JobRun, error) {
		scans++
		require.Equal(t, "route-a", routeKey)
		require.Equal(t, 2, remaining)
		return batch, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, scans)
	require.Equal(t, batch, claimed)
}

func BenchmarkPgQueScanWorkerRoutesSingleRoute(b *testing.B) {
	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{})
	routes := []string{"route-a"}
	batch := []domain.JobRun{
		{ID: "run-a"},
		{ID: "run-b"},
		{ID: "run-c"},
		{ID: "run-d"},
	}

	b.ReportAllocs()
	for b.Loop() {
		claimed, err := q.scanWorkerRoutes(routes, len(batch), func(routeKey string, remaining int) ([]domain.JobRun, error) {
			if routeKey != "route-a" || remaining != len(batch) {
				b.Fatalf("scan args = (%q, %d)", routeKey, remaining)
			}
			return batch, nil
		})
		if err != nil {
			b.Fatal(err)
		}
		pgQueScanWorkerRoutesBenchmarkSink = claimed
	}
}

func TestPgQueLogBackgroundErrorWritesWarning(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{Logger: logger})

	q.logBackgroundError(context.Background(), "ticker", "pgque ticker failed", errors.New("tick failed"))

	got := buf.String()
	require.Contains(
		t, got, "pgque ticker failed")
	require.Contains(
		t, got, "tick failed")
}

func TestPgQueLogBackgroundErrorSkipsCanceledContext(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{Logger: logger})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	q.logBackgroundError(ctx, "ticker", "pgque ticker failed", errors.New("tick failed"))
	require.Empty(t, buf.
		String())
}

func TestPgQueBackgroundOperationLabelBoundsCardinality(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		want      string
	}{
		{name: "ticker", operation: "ticker", want: "ticker"},
		{name: "maintenance", operation: "maintenance", want: "maintenance"},
		{name: "nack", operation: "nack", want: "nack"},
		{name: "unknown", operation: "route:project-a:critical", want: "other"},
		{name: "empty", operation: "", want: "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want,
				pgQueBackgroundOperationLabel(tt.operation))
		})
	}
}

func TestPgQueNackReservedMessageLogsFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "pgque.nack") {
				return pgconn.CommandTag{}, errors.New("nack failed")
			}
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{Logger: logger})

	q.nackReservedMessage(context.Background(), pgQueMessage{
		ID:        12,
		BatchID:   34,
		Type:      pgQueReadyEventType,
		Payload:   "{}",
		CreatedAt: time.Now(),
	}, "invalid ready event")

	got := buf.String()
	require.Contains(
		t, got, "pgque nack failed")
	require.Contains(
		t, got, "invalid ready event")
	require.Contains(
		t, got, "nack failed")
}

func TestPgQueNackReservedMessageSkipsCanceledContext(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "pgque.nack") {
				return pgconn.CommandTag{}, errors.New("nack failed")
			}
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{Logger: logger})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	q.nackReservedMessage(ctx, pgQueMessage{
		ID:        12,
		BatchID:   34,
		Type:      pgQueReadyEventType,
		Payload:   "{}",
		CreatedAt: time.Now(),
	}, "not claimable")
	require.Empty(t, buf.
		String())
}

type stringRows struct {
	values []string
	idx    int
}

func (r *stringRows) Close()                                       {}
func (r *stringRows) Err() error                                   { return nil }
func (r *stringRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *stringRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *stringRows) Next() bool {
	if r.idx >= len(r.values) {
		return false
	}
	r.idx++
	return true
}
func (r *stringRows) Scan(dest ...any) error {
	if len(dest) != 1 {
		return errors.New("stringRows: expected one destination")
	}
	ptr, ok := dest[0].(*string)
	if !ok {
		return errors.New("stringRows: destination is not *string")
	}
	*ptr = r.values[r.idx-1]
	return nil
}
func (r *stringRows) Values() ([]any, error) { return nil, nil }
func (r *stringRows) RawValues() [][]byte    { return nil }
func (r *stringRows) Conn() *pgx.Conn        { return nil }

type noRows struct{}

func (r *noRows) Close()                                       {}
func (r *noRows) Err() error                                   { return nil }
func (r *noRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *noRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *noRows) Next() bool                                   { return false }
func (r *noRows) Scan(...any) error                            { return pgx.ErrNoRows }
func (r *noRows) Values() ([]any, error)                       { return nil, nil }
func (r *noRows) RawValues() [][]byte                          { return nil }
func (r *noRows) Conn() *pgx.Conn                              { return nil }

func TestPgQueActiveBatchLockedReturnsSentinelForEmptyReceive(t *testing.T) {
	ctx := context.Background()
	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &noRows{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

	batch, err := q.activeBatchLocked(ctx, &pgQueRouteState{}, "stq_empty")
	require.ErrorIs(
		t, err, errPgQueNoMessages)
	require.Nil(t, batch)
}

func TestPgQueActiveBatchLockedBoundsEmptyLagCatchUp(t *testing.T) {
	ctx := context.Background()
	var receiveCalls int
	var lagCalls int
	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			receiveCalls++
			return &noRows{}, nil
		},
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			lagCalls++
			return &mockRow{scanFn: func(dest ...any) error {
				lag, ok := dest[0].(*int64)
				require.True(t, ok)
				*lag = 1
				return nil
			}}
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

	batch, err := q.activeBatchLocked(ctx, &pgQueRouteState{}, "stq_empty_lag")
	require.ErrorIs(t, err, errPgQueNoMessages)
	require.Nil(t, batch)
	require.Equal(t, pgQueMaxCatchUpBatches, receiveCalls)
	require.Equal(t, pgQueMaxCatchUpBatches, lagCalls)
	require.Less(t, receiveCalls, 1024)
}

func TestUnclaimedReservedCandidates(t *testing.T) {
	newCandidates := func() []pgQueCandidate {
		return []pgQueCandidate{
			{Event: pgQueReadyEvent{RunID: "run-1"}},
			{Event: pgQueReadyEvent{RunID: "run-2"}},
			{Event: pgQueReadyEvent{RunID: "run-3"}},
		}
	}
	candidates := newCandidates()
	wantCandidates := newCandidates()

	unclaimed := unclaimedReservedCandidates(candidates, []domain.JobRun{
		{ID: "run-1"},
		{ID: "run-3"},
	})
	require.False(t, len(unclaimed) != 1 || unclaimed[0].Event.RunID !=
		"run-2",
	)

	allClaimed := unclaimedReservedCandidates(newCandidates(), []domain.JobRun{
		{ID: "run-1"},
		{ID: "run-2"},
		{ID: "run-3"},
	})
	require.Empty(t,
		allClaimed)

	noneClaimed := unclaimedReservedCandidates(newCandidates(), nil)
	require.True(
		t, slices.Equal(noneClaimed,
			wantCandidates,
		))
}

func TestUnclaimedReservedCandidatesLargeBatch(t *testing.T) {
	candidates := []pgQueCandidate{
		{Event: pgQueReadyEvent{RunID: "run-1"}},
		{Event: pgQueReadyEvent{RunID: "run-2"}},
		{Event: pgQueReadyEvent{RunID: "run-3"}},
		{Event: pgQueReadyEvent{RunID: "run-4"}},
		{Event: pgQueReadyEvent{RunID: "run-5"}},
		{Event: pgQueReadyEvent{RunID: "run-6"}},
		{Event: pgQueReadyEvent{RunID: "run-7"}},
		{Event: pgQueReadyEvent{RunID: "run-8"}},
		{Event: pgQueReadyEvent{RunID: "run-9"}},
		{Event: pgQueReadyEvent{RunID: "run-10"}},
	}
	runs := []domain.JobRun{
		{ID: "run-1"},
		{ID: "run-2"},
		{ID: "run-3"},
		{ID: "run-4"},
		{ID: "run-5"},
		{ID: "run-6"},
		{ID: "run-7"},
		{ID: "run-8"},
		{ID: "run-9"},
	}

	unclaimed := unclaimedReservedCandidates(candidates, runs)
	require.False(t, len(unclaimed) != 1 || unclaimed[0].Event.RunID !=
		"run-10",
	)
}

func BenchmarkUnclaimedReservedCandidatesAllClaimed(b *testing.B) {
	candidates := []pgQueCandidate{
		{Event: pgQueReadyEvent{RunID: "run-1"}},
		{Event: pgQueReadyEvent{RunID: "run-2"}},
		{Event: pgQueReadyEvent{RunID: "run-3"}},
		{Event: pgQueReadyEvent{RunID: "run-4"}},
		{Event: pgQueReadyEvent{RunID: "run-5"}},
		{Event: pgQueReadyEvent{RunID: "run-6"}},
		{Event: pgQueReadyEvent{RunID: "run-7"}},
		{Event: pgQueReadyEvent{RunID: "run-8"}},
	}
	runs := []domain.JobRun{
		{ID: "run-1"},
		{ID: "run-2"},
		{ID: "run-3"},
		{ID: "run-4"},
		{ID: "run-5"},
		{ID: "run-6"},
		{ID: "run-7"},
		{ID: "run-8"},
	}

	for b.Loop() {
		pgQueCandidateBenchmarkSink = unclaimedReservedCandidates(candidates, runs)
	}
}

func BenchmarkUnclaimedReservedCandidatesPartialSmallBatch(b *testing.B) {
	baseCandidates := []pgQueCandidate{
		{Event: pgQueReadyEvent{RunID: "run-1"}},
		{Event: pgQueReadyEvent{RunID: "run-2"}},
		{Event: pgQueReadyEvent{RunID: "run-3"}},
		{Event: pgQueReadyEvent{RunID: "run-4"}},
		{Event: pgQueReadyEvent{RunID: "run-5"}},
		{Event: pgQueReadyEvent{RunID: "run-6"}},
		{Event: pgQueReadyEvent{RunID: "run-7"}},
		{Event: pgQueReadyEvent{RunID: "run-8"}},
	}
	runs := []domain.JobRun{
		{ID: "run-2"},
		{ID: "run-7"},
	}
	candidates := make([]pgQueCandidate, len(baseCandidates))

	for b.Loop() {
		copy(candidates, baseCandidates)
		pgQueCandidateBenchmarkSink = unclaimedReservedCandidates(candidates, runs)
	}
}

func TestSelectPgQueClaimCandidates(t *testing.T) {
	candidates := []pgQueCandidate{
		{Event: pgQueReadyEvent{RunID: "run-1", Generation: 10}},
		{Event: pgQueReadyEvent{RunID: "run-2", Generation: 20}, HasConcurrencyLimit: true},
		{Event: pgQueReadyEvent{RunID: "run-3", Generation: 30}},
	}

	var buffer pgQueClaimSelectionBuffer
	selection := selectPgQueClaimCandidates(candidates, 2, &buffer)
	require.True(
		t, slices.Equal(selection.RunIDs,
			[]string{"run-1",
				"run-2"},
		))
	require.True(
		t, slices.Equal(selection.Generations,

			[]int64{10,
				20}))
	require.True(
		t, selection.
			HasConcurrencyLimit,
	)
	require.Len(t,
		selection.
			Candidates, 2)
}

func TestClaimReservedCandidatesEmptySkipsClaim(t *testing.T) {
	t.Parallel()

	q := NewPgQueQueue(&mockDBTX{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			t.Fatal("empty candidate set must not query claims")
			return nil, nil
		},
	}, nil, PgQueConfig{})

	runs, unclaimed, retry, err := q.claimReservedCandidates(context.Background(), nil, 1, pgQueClaimFilter{})
	require.NoError(t, err)
	require.Nil(t, runs)
	require.Nil(t, unclaimed)
	require.False(t, retry)
}

func TestClaimReservedCandidatesReturnsRetryableSelectionWhenNothingClaimed(t *testing.T) {
	t.Parallel()

	q := NewPgQueQueue(&mockDBTX{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return routeErrorRows{}, nil
		},
	}, nil, PgQueConfig{})
	candidates := []pgQueCandidate{
		{Event: pgQueReadyEvent{RunID: "run-1", Generation: 1}},
		{Event: pgQueReadyEvent{RunID: "run-2", Generation: 2}},
	}

	runs, unclaimed, retry, err := q.claimReservedCandidates(context.Background(), candidates, 1, pgQueClaimFilter{})
	require.NoError(t, err)
	require.Nil(t, runs)
	require.True(t, retry)
	require.Len(t, unclaimed, 1)
	require.Equal(t, "run-1", unclaimed[0].Event.RunID)
}

func TestClaimReservedCandidatesPropagatesClaimError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("claim failed")
	q := NewPgQueQueue(&mockDBTX{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return nil, wantErr
		},
	}, nil, PgQueConfig{})
	candidates := []pgQueCandidate{{Event: pgQueReadyEvent{RunID: "run-1", Generation: 1}}}

	runs, unclaimed, retry, err := q.claimReservedCandidates(context.Background(), candidates, 1, pgQueClaimFilter{})
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, runs)
	require.Nil(t, unclaimed)
	require.False(t, retry)
}

func TestClaimRunsValidatesRequestShape(t *testing.T) {
	t.Parallel()

	q := NewPgQueQueue(&mockDBTX{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			t.Fatal("invalid request must not query claims")
			return nil, nil
		},
	}, nil, PgQueConfig{})

	runs, err := q.claimRuns(context.Background(), pgQueClaimRunRequest{})
	require.NoError(t, err)
	require.Nil(t, runs)

	runs, err = q.claimRuns(context.Background(), pgQueClaimRunRequest{
		RunIDs:      []string{"run-1"},
		Generations: nil,
		Limit:       1,
	})
	require.ErrorContains(t, err, "mismatched id/generation counts")
	require.Nil(t, runs)
}

func TestClaimRunsWrapsQueryErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		hasConcurrencyLimit bool
		want                string
	}{
		{
			name: "unconstrained",
			want: "pgque claim unconstrained runs",
		},
		{
			name:                "with concurrency",
			hasConcurrencyLimit: true,
			want:                "pgque claim runs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wantErr := errors.New("query failed")
			q := NewPgQueQueue(&mockDBTX{
				queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return nil, wantErr
				},
			}, nil, PgQueConfig{})

			runs, err := q.claimRuns(context.Background(), pgQueClaimRunRequest{
				RunIDs:              []string{"run-1"},
				Generations:         []int64{1},
				Limit:               1,
				HasConcurrencyLimit: tt.hasConcurrencyLimit,
			})
			require.ErrorContains(t, err, tt.want)
			require.ErrorIs(t, err, wantErr)
			require.Nil(t, runs)
		})
	}
}

func TestScanPgQueClaimedRunsWrapsScanAndRowsErrors(t *testing.T) {
	t.Parallel()

	t.Run("scan error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("scan failed")
		runs, err := scanPgQueClaimedRuns(&claimScanErrorRows{err: wantErr}, 1, "claim test")
		require.ErrorContains(t, err, "claim test scan")
		require.ErrorIs(t, err, wantErr)
		require.Nil(t, runs)
	})

	t.Run("rows error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("rows failed")
		runs, err := scanPgQueClaimedRuns(routeErrorRows{err: wantErr}, 1, "claim test")
		require.ErrorContains(t, err, "claim test rows")
		require.ErrorIs(t, err, wantErr)
		require.Nil(t, runs)
	})
}

func TestMarshalPgQueReadyEventEscapesFields(t *testing.T) {
	want := pgQueReadyEvent{
		RunID:      "run-\"\\\n\u2603",
		RouteKey:   "project:env:worker:<queue>&",
		Generation: 42,
		Priority:   -3,
	}

	payload := marshalPgQueReadyEvent(want)

	var got pgQueReadyEvent
	require.NoError(t, json.Unmarshal(payload, &got))
	require.Equal(t, want, got)
}

func TestPgQueCandidateRunIDsPreservesOrder(t *testing.T) {
	candidates := []pgQueCandidate{
		{Event: pgQueReadyEvent{RunID: "run-1"}},
		{Event: pgQueReadyEvent{RunID: "run-2"}},
		{Event: pgQueReadyEvent{RunID: "run-3"}},
	}

	var buffer pgQueCandidateRunIDBuffer
	runIDs := buffer.collect(candidates)
	require.True(
		t, slices.Equal(runIDs, []string{"run-1",
			"run-2",
			"run-3"}),
	)
}

func TestPgQueRefreshCandidateClaimStateUpdatesSmallBatch(t *testing.T) {
	ctx := context.Background()
	candidates := []pgQueCandidate{
		{Event: pgQueReadyEvent{RunID: "run-1", Priority: 1}},
		{Event: pgQueReadyEvent{RunID: "run-2", Priority: 2}},
		{Event: pgQueReadyEvent{RunID: "run-1", Priority: 3}},
	}
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
			require.Contains(
				t, sql, "job_run_state")
			require.Len(t,
				args, 1)

			runIDs, ok := args[0].([]string)
			require.True(
				t, ok)
			require.True(
				t, slices.Equal(runIDs, []string{"run-1",
					"run-2",
					"run-1"}),
			)

			return &pgQueCandidateClaimStateRows{
				values: []pgQueCandidateClaimState{
					{runID: "run-1", priority: 9, hasConcurrencyLimit: true},
					{runID: "run-2", priority: 7, hasConcurrencyLimit: false},
				},
			}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	require.NoError(t, q.refreshCandidateClaimState(ctx,
		candidates,
	))
	require.False(t, candidates[0].Event.Priority !=
		9 ||
		!candidates[0].HasConcurrencyLimit,
	)
	require.False(t, candidates[1].Event.Priority !=
		7 ||
		candidates[1].HasConcurrencyLimit,
	)
	require.False(t, candidates[2].Event.Priority !=
		9 ||
		!candidates[2].HasConcurrencyLimit,
	)
}

func BenchmarkSelectPgQueClaimCandidates(b *testing.B) {
	candidates := []pgQueCandidate{
		{Event: pgQueReadyEvent{RunID: "run-1", Generation: 1}},
		{Event: pgQueReadyEvent{RunID: "run-2", Generation: 2}},
		{Event: pgQueReadyEvent{RunID: "run-3", Generation: 3}},
		{Event: pgQueReadyEvent{RunID: "run-4", Generation: 4}},
		{Event: pgQueReadyEvent{RunID: "run-5", Generation: 5}},
		{Event: pgQueReadyEvent{RunID: "run-6", Generation: 6}},
		{Event: pgQueReadyEvent{RunID: "run-7", Generation: 7}, HasConcurrencyLimit: true},
		{Event: pgQueReadyEvent{RunID: "run-8", Generation: 8}},
	}
	var buffer pgQueClaimSelectionBuffer

	for b.Loop() {
		pgQueClaimSelectionBenchmarkSink = selectPgQueClaimCandidates(candidates, len(candidates), &buffer)
	}
}

func BenchmarkPgQueCandidateRunIDs(b *testing.B) {
	candidates := []pgQueCandidate{
		{Event: pgQueReadyEvent{RunID: "run-1"}},
		{Event: pgQueReadyEvent{RunID: "run-2"}},
		{Event: pgQueReadyEvent{RunID: "run-3"}},
		{Event: pgQueReadyEvent{RunID: "run-4"}},
		{Event: pgQueReadyEvent{RunID: "run-5"}},
		{Event: pgQueReadyEvent{RunID: "run-6"}},
		{Event: pgQueReadyEvent{RunID: "run-7"}},
		{Event: pgQueReadyEvent{RunID: "run-8"}},
	}
	var buffer pgQueCandidateRunIDBuffer

	for b.Loop() {
		runIDs := buffer.collect(candidates)
		pgQueCandidateRunIDBenchmarkSink = runIDs[len(runIDs)-1]
	}
}

func BenchmarkPgQueClaimFilterWorkerRefArgs(b *testing.B) {
	refs := []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: "default"},
		{ProjectID: "project-a", QueueName: "default"},
		{ProjectID: "project-a", QueueName: "critical", EnvironmentID: "production"},
		{ProjectID: "project-a", QueueName: "critical", EnvironmentID: "staging"},
		{ProjectID: "project-b", QueueName: "default"},
		{ProjectID: "project-c", QueueName: "bulk", EnvironmentID: "production"},
	}
	filter := pgQueClaimFilter{
		WorkerRefs:    refs,
		workerRefArgs: workerQueueRefArgs(refs),
	}

	for b.Loop() {
		args := filter.workerArgs()
		pgQueWorkerRefArgsBenchmarkSink.projectIDs = args.ProjectIDs
		pgQueWorkerRefArgsBenchmarkSink.queueNames = args.QueueNames
		pgQueWorkerRefArgsBenchmarkSink.environmentIDs = args.EnvironmentIDs
	}
}

func BenchmarkWorkerQueueRefArgsSingle(b *testing.B) {
	refs := []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: "critical", EnvironmentID: "production"},
	}

	b.ReportAllocs()
	for b.Loop() {
		args := workerQueueRefArgs(refs)
		pgQueWorkerRefArgsBenchmarkSink.projectIDs = args.ProjectIDs
		pgQueWorkerRefArgsBenchmarkSink.queueNames = args.QueueNames
		pgQueWorkerRefArgsBenchmarkSink.environmentIDs = args.EnvironmentIDs
	}
}

func BenchmarkWorkerQueueRefArgsSmall(b *testing.B) {
	refs := []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: "default"},
		{ProjectID: "project-a", QueueName: "default"},
		{ProjectID: "project-a", QueueName: "critical", EnvironmentID: "production"},
		{ProjectID: "project-b", QueueName: "bulk", EnvironmentID: "staging"},
	}

	b.ReportAllocs()
	for b.Loop() {
		args := workerQueueRefArgs(refs)
		pgQueWorkerRefArgsBenchmarkSink.projectIDs = args.ProjectIDs
		pgQueWorkerRefArgsBenchmarkSink.queueNames = args.QueueNames
		pgQueWorkerRefArgsBenchmarkSink.environmentIDs = args.EnvironmentIDs
	}
}

func BenchmarkWorkerQueueRefArgsNormalized(b *testing.B) {
	refs := []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: "default"},
		{ProjectID: "project-a", QueueName: "critical", EnvironmentID: "production"},
		{ProjectID: "project-a", QueueName: "critical", EnvironmentID: "staging"},
		{ProjectID: "project-b", QueueName: "default"},
		{ProjectID: "project-c", QueueName: "bulk", EnvironmentID: "production"},
	}

	for b.Loop() {
		args := workerQueueRefArgsFromNormalized(refs)
		pgQueWorkerRefArgsBenchmarkSink.projectIDs = args.ProjectIDs
		pgQueWorkerRefArgsBenchmarkSink.queueNames = args.QueueNames
		pgQueWorkerRefArgsBenchmarkSink.environmentIDs = args.EnvironmentIDs
	}
}

func BenchmarkNormalizePgQueWorkerQueueRefsSingle(b *testing.B) {
	refs := []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: "critical", EnvironmentID: "production"},
	}

	b.ReportAllocs()
	for b.Loop() {
		pgQueWorkerRefsBenchmarkSink = normalizePgQueWorkerQueueRefs(refs)
	}
}

func BenchmarkNormalizePgQueWorkerQueueRefsSmall(b *testing.B) {
	refs := []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: "default"},
		{ProjectID: "project-a", QueueName: "default"},
		{ProjectID: "project-a", QueueName: "critical", EnvironmentID: "production"},
		{ProjectID: "project-b", QueueName: "bulk", EnvironmentID: "staging"},
	}

	b.ReportAllocs()
	for b.Loop() {
		pgQueWorkerRefsBenchmarkSink = normalizePgQueWorkerQueueRefs(refs)
	}
}

func TestWorkerQueueRefArgsFromNormalized(t *testing.T) {
	refs := []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: "default"},
		{ProjectID: "project-a", QueueName: "critical", EnvironmentID: "production"},
		{ProjectID: "project-b", QueueName: "bulk", EnvironmentID: "staging"},
	}

	args := workerQueueRefArgsFromNormalized(refs)
	require.True(
		t, slices.Equal(args.ProjectIDs,
			[]string{"project-a",
				"project-a",
				"project-b",
			}))
	require.True(
		t, slices.Equal(args.QueueNames,
			[]string{"default",
				"critical",
				"bulk",
			}))
	require.True(
		t, slices.Equal(args.EnvironmentIDs,
			[]string{"",
				"production",
				"staging",
			}))
}

func TestPgQueClaimFilterWorkerArgsUsesPrecomputedArgs(t *testing.T) {
	refs := []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: "default"},
	}
	args := pgQueWorkerRefArgs{
		ProjectIDs:     []string{"precomputed-project"},
		QueueNames:     []string{"precomputed-queue"},
		EnvironmentIDs: []string{"precomputed-env"},
	}
	filter := pgQueClaimFilter{
		WorkerRefs:    refs,
		workerRefArgs: args,
	}

	got := filter.workerArgs()
	require.True(
		t, slices.Equal(got.ProjectIDs,
			args.ProjectIDs,
		))
	require.True(
		t, slices.Equal(got.QueueNames,
			args.QueueNames,
		))
	require.True(
		t, slices.Equal(got.EnvironmentIDs,
			args.
				EnvironmentIDs,
		))
}

func TestRemoveReservedMessagesKeepsUnreservedBatchMessages(t *testing.T) {
	batch := &pgQueActiveBatch{
		Messages: []pgQueMessage{
			{ID: 1},
			{ID: 2},
			{ID: 3},
			{ID: 4},
		},
	}
	invalid := []pgQueMessage{{ID: 2}}
	candidates := []pgQueCandidate{
		{Message: pgQueMessage{ID: 4}},
	}

	removeReservedMessages(batch, invalid, candidates)

	gotIDs := make([]int64, 0, len(batch.Messages))
	for _, msg := range batch.Messages {
		gotIDs = append(gotIDs, msg.ID)
	}
	wantIDs := []int64{1, 3}
	require.True(
		t, slices.Equal(gotIDs, wantIDs))
}

func TestRemoveReservedMessagesClearsCompactedTail(t *testing.T) {
	messages := []pgQueMessage{
		{ID: 1, Payload: "keep-1"},
		{ID: 2, Payload: "remove-2"},
		{ID: 3, Payload: "keep-3"},
		{ID: 4, Payload: "remove-4"},
		{ID: 5, Payload: "keep-5"},
	}
	batch := &pgQueActiveBatch{Messages: messages}
	candidates := []pgQueCandidate{
		{Message: pgQueMessage{ID: 2}},
		{Message: pgQueMessage{ID: 4}},
	}

	removeReservedMessages(batch, nil, candidates)

	gotIDs := make([]int64, 0, len(batch.Messages))
	for _, msg := range batch.Messages {
		gotIDs = append(gotIDs, msg.ID)
	}
	wantIDs := []int64{1, 3, 5}
	require.True(
		t, slices.Equal(gotIDs, wantIDs))

	for _, msg := range messages[len(batch.Messages):] {
		require.Equal(t, (pgQueMessage{}), msg)
	}
}

func BenchmarkRemoveReservedMessagesSingleCandidate(b *testing.B) {
	for b.Loop() {
		batch := &pgQueActiveBatch{
			Messages: []pgQueMessage{
				{ID: 1},
				{ID: 2},
				{ID: 3},
				{ID: 4},
			},
		}
		removeReservedMessages(batch, nil, []pgQueCandidate{
			{Message: pgQueMessage{ID: 2}},
		})
	}
}

func BenchmarkRemoveReservedMessagesFullBatch(b *testing.B) {
	candidates := []pgQueCandidate{
		{Message: pgQueMessage{ID: 1}},
		{Message: pgQueMessage{ID: 2}},
		{Message: pgQueMessage{ID: 3}},
		{Message: pgQueMessage{ID: 4}},
	}
	for b.Loop() {
		batch := &pgQueActiveBatch{
			Messages: []pgQueMessage{
				{ID: 1},
				{ID: 2},
				{ID: 3},
				{ID: 4},
			},
		}
		removeReservedMessages(batch, nil, candidates)
	}
}

func BenchmarkRemoveReservedMessagesPartialBatch(b *testing.B) {
	candidates := []pgQueCandidate{
		{Message: pgQueMessage{ID: 2}},
		{Message: pgQueMessage{ID: 4}},
	}
	for b.Loop() {
		batch := &pgQueActiveBatch{
			Messages: []pgQueMessage{
				{ID: 1},
				{ID: 2},
				{ID: 3},
				{ID: 4},
				{ID: 5},
				{ID: 6},
			},
		}
		removeReservedMessages(batch, nil, candidates)
	}
}

func TestPgQueEnsureRouteConfiguresRotationPeriod(t *testing.T) {
	ctx := context.Background()
	var rotationPeriod string
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "pgque.set_queue_config") && len(args) == 3 && args[1] == "rotation_period" {
				arg, ok := args[1].(string)
				require.True(
					t, ok)
				require.Equal(t, "rotation_period",
					arg)

				value, ok := args[2].(string)
				require.True(
					t, ok)

				rotationPeriod = value
			}
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{RotationPeriod: 90 * time.Second})
	require.NoError(t, q.ensureRoute(ctx, db,
		"http", "stq_test",
	))
	require.Equal(t, "90000000 microseconds",

		rotationPeriod,
	)
}

func TestPgQueSendReadyEventsFetchesGenerationsSetBased(t *testing.T) {
	ctx := context.Background()
	var queryCalls int
	var queryRowCalls int
	var sendBatchCalls int
	var recordCalls int
	var gotRunIDs []string
	var sentPayloads []string

	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
			queryCalls++
			require.False(t, !strings.Contains(sql, "ready_generation") ||
				!strings.Contains(sql,
					"ANY($1::text[])",
				),
			)
			require.Len(t,
				args, 1)

			runIDs, ok := args[0].([]string)
			require.True(
				t, ok)

			gotRunIDs = append([]string(nil), runIDs...)
			return &pgQueGenerationRows{
				values: []pgQueGenerationRow{
					{runID: "run-a", generation: 11},
					{runID: "run-b", generation: 12},
				},
			}, nil
		},
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			queryRowCalls++
			require.Failf(t, "test failure",

				"unexpected per-run QueryRow SQL = %q", sql)
			return &mockRow{}
		},
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "strait_pgque_ready_events") {
				recordCalls++
				require.Len(t,
					args, 2)

				runIDs, ok := args[0].([]string)
				require.True(
					t, ok)

				generations, ok := args[1].([]int64)
				require.True(
					t, ok)
				require.True(
					t, slices.Equal(runIDs, []string{"run-a",
						"run-b"},
					))
				require.True(
					t, slices.Equal(generations,
						[]int64{11,
							12}))

				return pgconn.CommandTag{}, nil
			}
			require.Contains(
				t, sql, "pgque.send_batch")

			sendBatchCalls++
			require.Len(t,
				args, 3)

			eventType, ok := args[1].(string)
			require.False(t, !ok ||
				eventType != pgQueReadyEventType,
			)

			payloads, ok := args[2].([]string)
			require.True(
				t, ok)

			sentPayloads = append([]string(nil), payloads...)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	q.routeState(pgQueHTTPRouteKey).configured.Store(true)

	runs := []*domain.JobRun{
		{ID: "run-a", Status: domain.StatusQueued, Priority: 9},
		{ID: "run-delayed", Status: domain.StatusDelayed, Priority: 8},
		{ID: "run-b", Status: domain.StatusQueued, Priority: 7},
	}
	require.NoError(t, q.sendReadyEvents(ctx,
		db, runs),
	)
	require.Equal(t, 1, queryCalls)
	require.Equal(t, 0, queryRowCalls)
	require.Equal(t, 1, sendBatchCalls)
	require.Equal(t, 1, recordCalls)
	require.True(
		t, slices.Equal(gotRunIDs, []string{"run-a",
			"run-b",
		}))
	require.Len(t,
		sentPayloads,
		2)

	assertPgQueReadyEvent(t, sentPayloads[0], pgQueReadyEvent{
		RunID:      "run-a",
		RouteKey:   pgQueHTTPRouteKey,
		Generation: 11,
		Priority:   9,
	})
	assertPgQueReadyEvent(t, sentPayloads[1], pgQueReadyEvent{
		RunID:      "run-b",
		RouteKey:   pgQueHTTPRouteKey,
		Generation: 12,
		Priority:   7,
	})
}

func TestPgQueSendFreshReadyEventsUsesInitialGenerationWithoutLookup(t *testing.T) {
	ctx := context.Background()
	var queryCalls int
	var queryRowCalls int
	var sendBatchCalls int
	var recordCalls int
	var sentPayloads []string

	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			queryCalls++
			require.Failf(t, "test failure", "unexpected Query SQL = %q", sql)
			return nil, nil
		},
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			queryRowCalls++
			require.Failf(t, "test failure", "unexpected QueryRow SQL = %q", sql)
			return &mockRow{}
		},
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "strait_pgque_ready_events") {
				recordCalls++
				require.Len(t, args, 2)

				runIDs, ok := args[0].([]string)
				require.True(t, ok)
				generations, ok := args[1].([]int64)
				require.True(t, ok)
				require.True(t, slices.Equal(runIDs, []string{"run-a", "run-b"}))
				require.True(t, slices.Equal(generations, []int64{0, 0}))

				return pgconn.CommandTag{}, nil
			}
			require.Contains(t, sql, "pgque.send_batch")
			sendBatchCalls++
			payloads, ok := args[2].([]string)
			require.True(t, ok)
			sentPayloads = append([]string(nil), payloads...)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	q.routeState(pgQueHTTPRouteKey).configured.Store(true)

	runs := []*domain.JobRun{
		{ID: "run-a", Status: domain.StatusQueued, Priority: 9},
		{ID: "run-delayed", Status: domain.StatusDelayed, Priority: 8},
		{ID: "run-b", Status: domain.StatusQueued, Priority: 7},
	}
	require.NoError(t, q.sendFreshReadyEvents(ctx, db, runs))
	require.Equal(t, 0, queryCalls)
	require.Equal(t, 0, queryRowCalls)
	require.Equal(t, 1, sendBatchCalls)
	require.Equal(t, 1, recordCalls)
	require.Len(t, sentPayloads, 2)

	assertPgQueReadyEvent(t, sentPayloads[0], pgQueReadyEvent{
		RunID:      "run-a",
		RouteKey:   pgQueHTTPRouteKey,
		Generation: 0,
		Priority:   9,
	})
	assertPgQueReadyEvent(t, sentPayloads[1], pgQueReadyEvent{
		RunID:      "run-b",
		RouteKey:   pgQueHTTPRouteKey,
		Generation: 0,
		Priority:   7,
	})
}

func TestPgQueSendReadyEventsFetchesWorkerRoutesSetBased(t *testing.T) {
	ctx := context.Background()
	var jobRouteQueries int
	var generationQueries int
	var queryRowCalls int
	var recordCalls int
	gotJobIDs := []string{}
	sentEvents := map[string]pgQueReadyEvent{}

	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
			switch {
			case strings.Contains(sql, "FROM jobs"):
				jobRouteQueries++
				if len(args) != 1 {
					require.Failf(t, "test failure",

						"worker route args = %+v, want job ids", args)
				}
				jobIDs, ok := args[0].([]string)
				if !ok {
					require.Failf(t, "test failure",

						"worker route arg type = %T, want []string", args[0])
				}
				gotJobIDs = append([]string(nil), jobIDs...)
				return &pgQueWorkerJobRouteRows{
					values: []pgQueWorkerJobRouteRow{
						{jobID: "job-a", queueName: "default", environmentID: "prod"},
						{jobID: "job-b", queueName: "bulk"},
					},
				}, nil
			case strings.Contains(sql, "FROM job_run_state"):
				generationQueries++
				return &pgQueGenerationRows{
					values: []pgQueGenerationRow{
						{runID: "run-a", generation: 11},
						{runID: "run-b", generation: 12},
						{runID: "run-c", generation: 13},
					},
				}, nil
			default:
				require.Failf(t, "test failure", "unexpected Query SQL = %q", sql)
				return nil, nil
			}
		},
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			queryRowCalls++
			require.Failf(t, "test failure",

				"unexpected per-run QueryRow SQL = %q", sql)
			return &mockRow{}
		},
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "strait_pgque_ready_events") {
				recordCalls++
				return pgconn.CommandTag{}, nil
			}
			require.Contains(
				t, sql, "pgque.send_batch")

			payloads, ok := args[2].([]string)
			require.True(
				t, ok)

			for _, payload := range payloads {
				var event pgQueReadyEvent
				require.NoError(t, json.
					Unmarshal([]byte(
						payload),
						&event))

				sentEvents[event.RunID] = event
			}
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	q.routeState(pgQueWorkerRouteKey("project-a", "default", "prod")).configured.Store(true)
	q.routeState(pgQueWorkerRouteKey("project-a", "critical", "prod")).configured.Store(true)
	q.routeState(pgQueWorkerRouteKey("project-b", "bulk", "")).configured.Store(true)

	runs := []*domain.JobRun{
		{ID: "run-a", JobID: "job-a", ProjectID: "project-a", Status: domain.StatusQueued, Priority: 9, ExecutionMode: domain.ExecutionModeWorker},
		{ID: "run-b", JobID: "job-a", ProjectID: "project-a", Status: domain.StatusQueued, Priority: 8, ExecutionMode: domain.ExecutionModeWorker, QueueName: "critical"},
		{ID: "run-c", JobID: "job-b", ProjectID: "project-b", Status: domain.StatusQueued, Priority: 7, ExecutionMode: domain.ExecutionModeWorker},
	}
	require.NoError(t, q.sendReadyEvents(ctx,
		db, runs),
	)
	require.Equal(t, 1, jobRouteQueries)
	require.True(
		t, slices.Equal(gotJobIDs, []string{"job-a",
			"job-b",
		}))
	require.Equal(t, 1, generationQueries)
	require.Equal(t, 0, queryRowCalls)
	require.Equal(t, 1, recordCalls)
	require.Len(t,
		sentEvents,
		3)

	wantEvents := map[string]pgQueReadyEvent{
		"run-a": {RunID: "run-a", RouteKey: pgQueWorkerRouteKey("project-a", "default", "prod"), Generation: 11, Priority: 9},
		"run-b": {RunID: "run-b", RouteKey: pgQueWorkerRouteKey("project-a", "critical", "prod"), Generation: 12, Priority: 8},
		"run-c": {RunID: "run-c", RouteKey: pgQueWorkerRouteKey("project-b", "bulk", ""), Generation: 13, Priority: 7},
	}
	for runID, want := range wantEvents {
		require.Equal(t, want, sentEvents[runID])
	}
}

func BenchmarkPgQueSendReadyEventsHTTPBatch(b *testing.B) {
	ctx := context.Background()
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if !strings.Contains(sql, "FROM job_run_state") {
				b.Fatalf("unexpected Query SQL = %q", sql)
			}
			return &pgQueGenerationRows{
				values: []pgQueGenerationRow{
					{runID: "run-a", generation: 11},
					{runID: "run-b", generation: 12},
					{runID: "run-c", generation: 13},
					{runID: "run-d", generation: 14},
				},
			}, nil
		},
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, nil, PgQueConfig{})
	q.routeState(pgQueHTTPRouteKey).configured.Store(true)
	runs := []*domain.JobRun{
		{ID: "run-a", Status: domain.StatusQueued, Priority: 9},
		{ID: "run-b", Status: domain.StatusQueued, Priority: 8},
		{ID: "run-c", Status: domain.StatusQueued, Priority: 7},
		{ID: "run-d", Status: domain.StatusQueued, Priority: 6},
	}

	b.ReportAllocs()
	for b.Loop() {
		pgQueSendReadyEventsErrBenchmarkSink = q.sendReadyEvents(ctx, db, runs)
	}
}

func TestPgQueEnsureRunRoutesCachedFetchesWorkerRoutesSetBased(t *testing.T) {
	ctx := context.Background()
	var jobRouteQueries int
	var queryRowCalls int
	gotJobIDs := []string{}
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
			require.Contains(
				t, sql, "FROM jobs")

			jobRouteQueries++
			require.Len(t,
				args, 1)

			jobIDs, ok := args[0].([]string)
			require.True(
				t, ok)

			gotJobIDs = append([]string(nil), jobIDs...)
			return &pgQueWorkerJobRouteRows{
				values: []pgQueWorkerJobRouteRow{
					{jobID: "job-a", queueName: "default", environmentID: "prod"},
					{jobID: "job-b", queueName: "bulk"},
				},
			}, nil
		},
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			queryRowCalls++
			require.Failf(t, "test failure",

				"unexpected per-run QueryRow SQL = %q", sql)
			return &mockRow{}
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			require.Failf(t, "test failure",

				"unexpected route setup Exec SQL = %q", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	q.routeState(pgQueWorkerRouteKey("project-a", "default", "prod")).configured.Store(true)
	q.routeState(pgQueWorkerRouteKey("project-a", "critical", "prod")).configured.Store(true)
	q.routeState(pgQueWorkerRouteKey("project-b", "bulk", "")).configured.Store(true)

	runs := []*domain.JobRun{
		{ID: "run-a", JobID: "job-a", ProjectID: "project-a", Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeWorker},
		{ID: "run-delayed", JobID: "job-a", ProjectID: "project-a", Status: domain.StatusDelayed, ExecutionMode: domain.ExecutionModeWorker},
		{ID: "run-b", JobID: "job-a", ProjectID: "project-a", Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeWorker, QueueName: "critical"},
		{ID: "run-c", JobID: "job-b", ProjectID: "project-b", Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeWorker},
	}
	require.NoError(t, q.ensureRunRoutesCached(ctx, runs))
	require.Equal(t, 1, jobRouteQueries)
	require.True(
		t, slices.Equal(gotJobIDs, []string{"job-a",
			"job-b",
		}))
	require.Equal(t, 0, queryRowCalls)
}

func TestPgQueEnsureRunRoutesCachedFailsWhenWorkerJobMissing(t *testing.T) {
	ctx := context.Background()
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			require.Contains(
				t, sql, "FROM jobs")

			return &pgQueWorkerJobRouteRows{
				values: []pgQueWorkerJobRouteRow{
					{jobID: "job-a", queueName: "default"},
				},
			}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

	err := q.ensureRunRoutesCached(ctx, []*domain.JobRun{
		{ID: "run-a", JobID: "job-a", ProjectID: "project-a", Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeWorker},
		{ID: "run-b", JobID: "job-b", ProjectID: "project-a", Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeWorker},
	})
	require.Error(t, err)
	require.Contains(
		t, err.Error(), "missing job job-b")
}

func TestPgQueSendReadyEventsSkipsNoQueuedRuns(t *testing.T) {
	ctx := context.Background()
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			require.Failf(t, "test failure",

				"unexpected Query SQL = %q", sql)
			return nil, nil
		},
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			require.Failf(t, "test failure",

				"unexpected QueryRow SQL = %q", sql)
			return &mockRow{}
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			require.Failf(t, "test failure",

				"unexpected Exec SQL = %q", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

	err := q.sendReadyEvents(ctx, db, []*domain.JobRun{
		nil,
		{ID: "run-delayed", Status: domain.StatusDelayed, ExecutionMode: domain.ExecutionModeWorker, JobID: "job-a"},
		{ID: "run-complete", Status: domain.StatusCompleted},
	})
	require.NoError(t, err)
}

func TestPgQueEnsureRunRoutesCachedSkipsNoQueuedRuns(t *testing.T) {
	ctx := context.Background()
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			require.Failf(t, "test failure",

				"unexpected Query SQL = %q", sql)
			return nil, nil
		},
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			require.Failf(t, "test failure",

				"unexpected QueryRow SQL = %q", sql)
			return &mockRow{}
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			require.Failf(t, "test failure",

				"unexpected Exec SQL = %q", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

	err := q.ensureRunRoutesCached(ctx, []*domain.JobRun{
		{ID: "run-delayed", Status: domain.StatusDelayed, ExecutionMode: domain.ExecutionModeWorker, JobID: "job-a"},
		{ID: "run-complete", Status: domain.StatusCompleted},
	})
	require.NoError(t, err)
}

func BenchmarkPgQueEnsureRunRoutesCachedHTTPBatch(b *testing.B) {
	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{})
	q.routeState(pgQueHTTPRouteKey).configured.Store(true)
	runs := []*domain.JobRun{
		{ID: "run-a", Status: domain.StatusQueued},
		{ID: "run-b", Status: domain.StatusQueued},
		{ID: "run-c", Status: domain.StatusQueued},
		{ID: "run-d", Status: domain.StatusQueued},
	}

	b.ReportAllocs()
	for b.Loop() {
		pgQueReadyEmitBatchErrBenchmarkSink = q.ensureRunRoutesCached(context.Background(), runs)
	}
}

func BenchmarkPgQueReadyRunsForEventsNoQueuedRuns(b *testing.B) {
	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{})
	runs := []*domain.JobRun{
		nil,
		{ID: "run-delayed-a", Status: domain.StatusDelayed, ExecutionMode: domain.ExecutionModeWorker, JobID: "job-a"},
		{ID: "run-completed-a", Status: domain.StatusCompleted, ExecutionMode: domain.ExecutionModeHTTP},
		{ID: "run-delayed-b", Status: domain.StatusDelayed, ExecutionMode: domain.ExecutionModeWorker, JobID: "job-b"},
		{ID: "run-failed-a", Status: domain.StatusFailed, ExecutionMode: domain.ExecutionModeHTTP},
	}

	b.ReportAllocs()
	for b.Loop() {
		readyRuns, _, err := q.readyRunsForEvents(context.Background(), q.db, runs)
		if err != nil {
			b.Fatal(err)
		}
		pgQueReadyRunsBenchmarkSink = readyRuns
	}
}

func BenchmarkPgQueReadyRunsForEventsSingleWorkerJob(b *testing.B) {
	ctx := context.Background()
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if !strings.Contains(sql, "FROM jobs") {
				b.Fatalf("unexpected Query SQL = %q", sql)
			}
			return &pgQueWorkerJobRouteRows{
				values: []pgQueWorkerJobRouteRow{
					{jobID: "job-a", queueName: "default", environmentID: "prod"},
				},
			}, nil
		},
	}
	q := NewPgQueQueue(db, nil, PgQueConfig{})
	runs := []*domain.JobRun{
		{
			ID:            "run-a",
			JobID:         "job-a",
			ProjectID:     "project-a",
			Status:        domain.StatusQueued,
			ExecutionMode: domain.ExecutionModeWorker,
		},
		{
			ID:            "run-b",
			JobID:         "job-a",
			ProjectID:     "project-a",
			Status:        domain.StatusQueued,
			ExecutionMode: domain.ExecutionModeWorker,
		},
		{
			ID:            "run-c",
			JobID:         "job-a",
			ProjectID:     "project-a",
			Status:        domain.StatusQueued,
			ExecutionMode: domain.ExecutionModeWorker,
		},
	}

	b.ReportAllocs()
	for b.Loop() {
		readyRuns, _, err := q.readyRunsForEvents(ctx, db, runs)
		if err != nil {
			b.Fatal(err)
		}
		pgQueReadyRunsBenchmarkSink = readyRuns
	}
}

func TestPgQueSendFreshReadyEventsFetchesWorkerRoutesButNotGenerations(t *testing.T) {
	ctx := context.Background()
	var jobRouteQueries int
	var generationQueries int
	var queryRowCalls int
	var recordCalls int
	gotJobIDs := []string{}
	sentEvents := map[string]pgQueReadyEvent{}

	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
			switch {
			case strings.Contains(sql, "FROM jobs"):
				jobRouteQueries++
				jobIDs, ok := args[0].([]string)
				require.True(t, ok)
				gotJobIDs = append([]string(nil), jobIDs...)
				return &pgQueWorkerJobRouteRows{
					values: []pgQueWorkerJobRouteRow{
						{jobID: "job-a", queueName: "default", environmentID: "prod"},
						{jobID: "job-b", queueName: "bulk"},
					},
				}, nil
			case strings.Contains(sql, "FROM job_run_state"):
				generationQueries++
				require.Failf(t, "test failure", "fresh ready events must not query generations: %q", sql)
				return nil, nil
			default:
				require.Failf(t, "test failure", "unexpected Query SQL = %q", sql)
				return nil, nil
			}
		},
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			queryRowCalls++
			require.Failf(t, "test failure", "unexpected QueryRow SQL = %q", sql)
			return &mockRow{}
		},
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "strait_pgque_ready_events") {
				recordCalls++
				return pgconn.CommandTag{}, nil
			}
			require.Contains(t, sql, "pgque.send_batch")
			payloads, ok := args[2].([]string)
			require.True(t, ok)
			for _, payload := range payloads {
				var event pgQueReadyEvent
				require.NoError(t, json.Unmarshal([]byte(payload), &event))
				sentEvents[event.RunID] = event
			}
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	q.routeState(pgQueWorkerRouteKey("project-a", "default", "prod")).configured.Store(true)
	q.routeState(pgQueWorkerRouteKey("project-a", "critical", "prod")).configured.Store(true)
	q.routeState(pgQueWorkerRouteKey("project-b", "bulk", "")).configured.Store(true)

	runs := []*domain.JobRun{
		{ID: "run-a", JobID: "job-a", ProjectID: "project-a", Status: domain.StatusQueued, Priority: 9, ExecutionMode: domain.ExecutionModeWorker},
		{ID: "run-b", JobID: "job-a", ProjectID: "project-a", Status: domain.StatusQueued, Priority: 8, ExecutionMode: domain.ExecutionModeWorker, QueueName: "critical"},
		{ID: "run-c", JobID: "job-b", ProjectID: "project-b", Status: domain.StatusQueued, Priority: 7, ExecutionMode: domain.ExecutionModeWorker},
	}
	require.NoError(t, q.sendFreshReadyEvents(ctx, db, runs))
	require.Equal(t, 1, jobRouteQueries)
	require.True(t, slices.Equal(gotJobIDs, []string{"job-a", "job-b"}))
	require.Equal(t, 0, generationQueries)
	require.Equal(t, 0, queryRowCalls)
	require.Equal(t, 1, recordCalls)
	require.Len(t, sentEvents, 3)

	wantEvents := map[string]pgQueReadyEvent{
		"run-a": {RunID: "run-a", RouteKey: pgQueWorkerRouteKey("project-a", "default", "prod"), Generation: 0, Priority: 9},
		"run-b": {RunID: "run-b", RouteKey: pgQueWorkerRouteKey("project-a", "critical", "prod"), Generation: 0, Priority: 8},
		"run-c": {RunID: "run-c", RouteKey: pgQueWorkerRouteKey("project-b", "bulk", ""), Generation: 0, Priority: 7},
	}
	for runID, want := range wantEvents {
		require.Equal(t, want, sentEvents[runID])
	}
}

func TestPgQueReadyRunsForEventsFailsWhenWorkerJobMissing(t *testing.T) {
	ctx := context.Background()
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			require.Contains(
				t, sql, "FROM jobs")

			return &pgQueWorkerJobRouteRows{
				values: []pgQueWorkerJobRouteRow{
					{jobID: "job-a", queueName: "default"},
				},
			}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

	_, _, err := q.readyRunsForEvents(ctx, db, []*domain.JobRun{
		{ID: "run-a", JobID: "job-a", ProjectID: "project-a", Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeWorker},
		{ID: "run-b", JobID: "job-b", ProjectID: "project-b", Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeWorker},
	})
	require.Error(t, err)
	require.Contains(
		t, err.Error(), "missing job job-b")
}

func TestPgQueReadyRunsForEventsKeepsDistinctWorkerRouteOverrides(t *testing.T) {
	ctx := context.Background()
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			require.Contains(t, sql, "FROM jobs")
			return &pgQueWorkerJobRouteRows{
				values: []pgQueWorkerJobRouteRow{
					{jobID: "job-a", queueName: "default", environmentID: "prod"},
				},
			}, nil
		},
	}
	q := NewPgQueQueue(db, nil, PgQueConfig{})

	readyRuns, runIDs, err := q.readyRunsForEvents(ctx, db, []*domain.JobRun{
		{ID: "run-a", JobID: "job-a", ProjectID: "project-a", Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeWorker},
		{ID: "run-b", JobID: "job-a", ProjectID: "project-a", Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeWorker, QueueName: "critical"},
		{ID: "run-c", JobID: "job-a", ProjectID: "project-a", Status: domain.StatusQueued, ExecutionMode: domain.ExecutionModeWorker},
	})
	require.NoError(t, err)
	require.True(t, slices.Equal(runIDs, []string{"run-a", "run-b", "run-c"}))
	require.Len(t, readyRuns, 3)
	require.Equal(t, pgQueWorkerRouteKey("project-a", "default", "prod"), readyRuns[0].routeKey)
	require.Equal(t, pgQueWorkerRouteKey("project-a", "critical", "prod"), readyRuns[1].routeKey)
	require.Equal(t, pgQueWorkerRouteKey("project-a", "default", "prod"), readyRuns[2].routeKey)
}

func TestPgQueSendReadyEventsFailsWhenGenerationMissing(t *testing.T) {
	ctx := context.Background()
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			require.Contains(
				t, sql, "ready_generation")

			return &pgQueGenerationRows{
				values: []pgQueGenerationRow{
					{runID: "run-a", generation: 11},
				},
			}, nil
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			require.Failf(t, "test failure",

				"unexpected Exec SQL = %q", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	q.routeState(pgQueHTTPRouteKey).configured.Store(true)

	err := q.sendReadyEvents(ctx, db, []*domain.JobRun{
		{ID: "run-a", Status: domain.StatusQueued},
		{ID: "run-b", Status: domain.StatusQueued},
	})
	require.Error(t, err)
	require.Contains(
		t, err.Error(), "missing run run-b")
}

func TestPgQueRecordReadyEmitBatchRejectsMismatchedInputs(t *testing.T) {
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			require.Failf(t, "test failure",

				"unexpected Exec SQL = %q", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

	err := q.recordReadyEmitBatch(context.Background(), db, []string{"run-a"}, nil)
	require.Error(t, err)
	require.Contains(
		t, err.Error(), "mismatched id/generation counts")
}

func BenchmarkPgQueRecordReadyEmitBatch(b *testing.B) {
	db := &mockDBTX{}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	runIDs := []string{
		"run-1",
		"run-2",
		"run-3",
		"run-4",
		"run-5",
		"run-6",
		"run-7",
		"run-8",
	}
	readyGenerations := []int64{1, 2, 3, 4, 5, 6, 7, 8}

	b.ReportAllocs()
	for b.Loop() {
		pgQueReadyEmitBatchErrBenchmarkSink = q.recordReadyEmitBatch(context.Background(), db, runIDs, readyGenerations)
	}
}

func TestPgQueEnqueueExistingSendsReadyEventForQueuedRun(t *testing.T) {
	ctx := context.Background()
	var sentPayload string
	var tickedQueue string
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			require.Contains(
				t, sql, "ready_generation")
			require.False(t, len(args) != 1 || args[0] != "run-queued")

			return &mockRow{scanFn: func(dest ...any) error {
				generation, ok := dest[0].(*int64)
				if !ok {
					return errors.New("generation destination is not *int64")
				}
				*generation = 7
				return nil
			}}
		},
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			switch {
			case strings.Contains(sql, "pgque.send"):
				if len(args) != 3 {
					require.Failf(t, "test failure",

						"pgque.send args = %+v, want queue, event type, and payload", args)
				}
				eventType, ok := args[1].(string)
				if !ok || eventType != pgQueReadyEventType {
					require.Failf(t, "test failure",

						"pgque.send event type = %v, want %s", args[1], pgQueReadyEventType)
				}
				payload, ok := args[2].(string)
				if !ok {
					require.Failf(t, "test failure",

						"pgque.send payload arg type = %T, want string", args[2])
				}
				sentPayload = payload
			case strings.Contains(sql, "pgque.ticker"):
				if len(args) != 1 {
					require.Failf(t, "test failure",

						"pgque.ticker args = %+v, want queue", args)
				}
				queueName, ok := args[0].(string)
				if !ok {
					require.Failf(t, "test failure",

						"pgque.ticker queue arg type = %T, want string", args[0])
				}
				tickedQueue = queueName
			case strings.Contains(sql, "strait_pgque_ready_events"):
				if len(args) != 2 {
					require.Failf(t, "test failure",

						"ready emit marker args = %+v, want run ids and generations", args)
				}
				runIDs, ok := args[0].([]string)
				if !ok {
					require.Failf(t, "test failure",

						"ready emit marker run id arg type = %T, want []string", args[0])
				}
				generations, ok := args[1].([]int64)
				if !ok {
					require.Failf(t, "test failure",

						"ready emit marker generation arg type = %T, want []int64", args[1])
				}
				if !slices.Equal(runIDs, []string{"run-queued"}) || !slices.Equal(generations, []int64{7}) {
					require.Failf(t, "test failure",

						"ready emit marker = %v/%v, want run-queued generation 7", runIDs, generations)
				}
			default:
				require.Failf(t, "test failure", "unexpected Exec SQL = %q", sql)
			}
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	q.routeState(pgQueHTTPRouteKey).configured.Store(true)

	run := &domain.JobRun{
		ID:       "run-queued",
		Status:   domain.StatusQueued,
		Priority: 9,
	}
	require.NoError(t, q.EnqueueExisting(ctx,
		run))

	var event pgQueReadyEvent
	require.NoError(t, json.
		Unmarshal([]byte(
			sentPayload,
		), &event))
	require.False(t, event.RunID !=
		run.ID ||
		event.RouteKey !=
			pgQueHTTPRouteKey ||
		event.
			Generation !=
			7 ||
		event.Priority != 9)
	require.Equal(t, pgQueQueueName(pgQueHTTPRouteKey),
		tickedQueue,
	)
}

func TestPgQueEnqueueExistingIgnoresNonQueuedRun(t *testing.T) {
	ctx := context.Background()
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			require.Failf(t, "test failure",

				"unexpected QueryRow SQL = %q", sql)
			return &mockRow{}
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			require.Failf(t, "test failure",

				"unexpected Exec SQL = %q", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	require.NoError(t, q.EnqueueExisting(ctx,
		&domain.JobRun{ID: "run-done",

			Status: domain.
				StatusCompleted,
		},
	))
}

type pgQueGenerationRow struct {
	runID      string
	generation int64
}

type pgQueWorkerJobRouteRow struct {
	jobID         string
	queueName     string
	environmentID string
}

type pgQueWorkerJobRouteRows struct {
	values []pgQueWorkerJobRouteRow
	idx    int
}

func (r *pgQueWorkerJobRouteRows) Close()                                       {}
func (r *pgQueWorkerJobRouteRows) Err() error                                   { return nil }
func (r *pgQueWorkerJobRouteRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *pgQueWorkerJobRouteRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *pgQueWorkerJobRouteRows) Next() bool {
	if r.idx >= len(r.values) {
		return false
	}
	r.idx++
	return true
}
func (r *pgQueWorkerJobRouteRows) Scan(dest ...any) error {
	if len(dest) != 3 {
		return errors.New("pgQueWorkerJobRouteRows: expected three destinations")
	}
	jobID, ok := dest[0].(*string)
	if !ok {
		return errors.New("pgQueWorkerJobRouteRows: job id destination is not *string")
	}
	queueName, ok := dest[1].(*string)
	if !ok {
		return errors.New("pgQueWorkerJobRouteRows: queue destination is not *string")
	}
	environmentID, ok := dest[2].(*string)
	if !ok {
		return errors.New("pgQueWorkerJobRouteRows: environment destination is not *string")
	}
	row := r.values[r.idx-1]
	*jobID = row.jobID
	*queueName = row.queueName
	*environmentID = row.environmentID
	return nil
}
func (r *pgQueWorkerJobRouteRows) Values() ([]any, error) { return nil, nil }
func (r *pgQueWorkerJobRouteRows) RawValues() [][]byte    { return nil }
func (r *pgQueWorkerJobRouteRows) Conn() *pgx.Conn        { return nil }

type pgQueCandidateClaimStateRows struct {
	values []pgQueCandidateClaimState
	idx    int
}

func (r *pgQueCandidateClaimStateRows) Close()                                       {}
func (r *pgQueCandidateClaimStateRows) Err() error                                   { return nil }
func (r *pgQueCandidateClaimStateRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *pgQueCandidateClaimStateRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *pgQueCandidateClaimStateRows) Next() bool {
	if r.idx >= len(r.values) {
		return false
	}
	r.idx++
	return true
}
func (r *pgQueCandidateClaimStateRows) Scan(dest ...any) error {
	if len(dest) != 3 {
		return errors.New("pgQueCandidateClaimStateRows: expected three destinations")
	}
	runID, ok := dest[0].(*string)
	if !ok {
		return errors.New("pgQueCandidateClaimStateRows: run id destination is not *string")
	}
	priority, ok := dest[1].(*int)
	if !ok {
		return errors.New("pgQueCandidateClaimStateRows: priority destination is not *int")
	}
	hasConcurrencyLimit, ok := dest[2].(*bool)
	if !ok {
		return errors.New("pgQueCandidateClaimStateRows: concurrency destination is not *bool")
	}
	row := r.values[r.idx-1]
	*runID = row.runID
	*priority = row.priority
	*hasConcurrencyLimit = row.hasConcurrencyLimit
	return nil
}
func (r *pgQueCandidateClaimStateRows) Values() ([]any, error) { return nil, nil }
func (r *pgQueCandidateClaimStateRows) RawValues() [][]byte    { return nil }
func (r *pgQueCandidateClaimStateRows) Conn() *pgx.Conn        { return nil }

type claimScanErrorRows struct {
	err     error
	scanned bool
}

func (r *claimScanErrorRows) Close()                                       {}
func (r *claimScanErrorRows) Err() error                                   { return nil }
func (r *claimScanErrorRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *claimScanErrorRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *claimScanErrorRows) Next() bool {
	if r.scanned {
		return false
	}
	r.scanned = true
	return true
}
func (r *claimScanErrorRows) Scan(...any) error      { return r.err }
func (r *claimScanErrorRows) Values() ([]any, error) { return nil, nil }
func (r *claimScanErrorRows) RawValues() [][]byte    { return nil }
func (r *claimScanErrorRows) Conn() *pgx.Conn        { return nil }

type pgQueGenerationRows struct {
	values []pgQueGenerationRow
	idx    int
}

func (r *pgQueGenerationRows) Close()                                       {}
func (r *pgQueGenerationRows) Err() error                                   { return nil }
func (r *pgQueGenerationRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *pgQueGenerationRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *pgQueGenerationRows) Next() bool {
	if r.idx >= len(r.values) {
		return false
	}
	r.idx++
	return true
}
func (r *pgQueGenerationRows) Scan(dest ...any) error {
	if len(dest) != 2 {
		return errors.New("pgQueGenerationRows: expected two destinations")
	}
	runID, ok := dest[0].(*string)
	if !ok {
		return errors.New("pgQueGenerationRows: run id destination is not *string")
	}
	generation, ok := dest[1].(*int64)
	if !ok {
		return errors.New("pgQueGenerationRows: generation destination is not *int64")
	}
	row := r.values[r.idx-1]
	*runID = row.runID
	*generation = row.generation
	return nil
}
func (r *pgQueGenerationRows) Values() ([]any, error) { return nil, nil }
func (r *pgQueGenerationRows) RawValues() [][]byte    { return nil }
func (r *pgQueGenerationRows) Conn() *pgx.Conn        { return nil }

func assertPgQueReadyEvent(t *testing.T, payload string, want pgQueReadyEvent) {
	t.Helper()

	var got pgQueReadyEvent
	require.NoError(t, json.
		Unmarshal([]byte(
			payload),
			&got))
	require.Equal(t, want, got)
}
