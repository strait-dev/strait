package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestPgQueFinishBatchReservationHandlesNilAndUndrainedBatches(t *testing.T) {
	t.Parallel()

	q := NewPgQueQueue(&mockDBTX{
		execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			t.Fatal("nil or undrained batch must not ack")
			return pgconn.CommandTag{}, nil
		},
	}, nil, PgQueConfig{})

	require.NoError(t, q.finishBatchReservation(context.Background(), &pgQueRouteState{}, nil, nil))

	batch := &pgQueActiveBatch{
		BatchID:  10,
		InFlight: 2,
	}
	state := &pgQueRouteState{activeBatch: batch}
	require.NoError(t, q.finishBatchReservation(context.Background(), state, batch, []pgQueCandidate{
		{Message: pgQueMessage{ID: 1}},
	}))
	require.False(t, batch.Closing)
	require.Equal(t, 1, batch.InFlight)
	require.Equal(t, []pgQueMessage{{ID: 1}}, batch.Messages)
	require.Same(t, batch, state.activeBatch)
}

func TestPgQueBatchLifecycleIgnoresInactiveBatch(t *testing.T) {
	t.Parallel()

	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{})
	active := &pgQueActiveBatch{BatchID: 1, Closing: true}
	inactive := &pgQueActiveBatch{BatchID: 2, Closing: true}
	state := &pgQueRouteState{activeBatch: active}

	require.False(t, q.closeBatchIfDrained(state, inactive, []pgQueCandidate{{Message: pgQueMessage{ID: 9}}}))
	q.reopenBatchAfterAckFailure(state, inactive)
	q.clearAckedBatch(state, inactive)

	require.Same(t, active, state.activeBatch)
	require.True(t, active.Closing)
	require.Empty(t, inactive.Messages)
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

func TestPgQueScanWorkerRoutesEmptyInputsSkipScan(t *testing.T) {
	t.Parallel()

	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{})
	scan := func(string, int) ([]domain.JobRun, error) {
		t.Fatal("empty route scan must not call scanner")
		return nil, nil
	}

	claimed, err := q.scanWorkerRoutes([]string{"route-a"}, 0, scan)
	require.NoError(t, err)
	require.Nil(t, claimed)

	claimed, err = q.scanWorkerRoutes(nil, 1, scan)
	require.NoError(t, err)
	require.Nil(t, claimed)
}

func TestPgQueScanWorkerRoutesReturnsPartialClaimsOnError(t *testing.T) {
	t.Parallel()

	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{})
	wantErr := errors.New("route failed")
	calls := 0

	claimed, err := q.scanWorkerRoutes([]string{"route-a", "route-b"}, 2, func(routeKey string, remaining int) ([]domain.JobRun, error) {
		calls++
		switch calls {
		case 1:
			require.Equal(t, "route-a", routeKey)
			require.Equal(t, 2, remaining)
			return []domain.JobRun{{ID: "run-a"}}, nil
		case 2:
			require.Equal(t, "route-b", routeKey)
			require.Equal(t, 1, remaining)
			return nil, wantErr
		default:
			t.Fatalf("unexpected scan call %d", calls)
			return nil, nil
		}
	})

	require.ErrorIs(t, err, wantErr)
	require.Equal(t, []domain.JobRun{{ID: "run-a"}}, claimed)
}

func TestPgQueDequeueReturnsNilWhenNoRunAvailable(t *testing.T) {
	t.Parallel()

	q := NewPgQueQueue(&mockDBTX{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &noRows{}, nil
		},
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				lag, ok := dest[0].(*int64)
				require.True(t, ok)
				*lag = 0
				return nil
			}}
		},
	}, nil, PgQueConfig{TickInterval: time.Hour})
	state := q.routeState(pgQueHTTPRouteKey)
	state.configured.Store(true)
	state.lastForceTick = time.Now()

	run, err := q.Dequeue(context.Background())
	require.NoError(t, err)
	require.Nil(t, run)
}

func TestPgQueDequeuePropagatesRouteSetupError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("route setup failed")
	q := NewPgQueQueue(&mockDBTX{
		execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, wantErr
		},
	}, nil, PgQueConfig{})

	run, err := q.Dequeue(context.Background())
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, run)
}

func TestPgQueDequeueNForWorkerQueuesSkipsEmptyInputs(t *testing.T) {
	t.Parallel()

	q := NewPgQueQueue(&mockDBTX{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			t.Fatal("empty worker dequeue input must not query routes")
			return nil, nil
		},
	}, nil, PgQueConfig{})

	runs, err := q.DequeueNForWorkerQueues(context.Background(), 0, []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: "worker"},
	})
	require.NoError(t, err)
	require.Nil(t, runs)

	runs, err = q.DequeueNForWorkerQueues(context.Background(), 1, []domain.WorkerQueueRef{
		{ProjectID: "", QueueName: "worker"},
	})
	require.NoError(t, err)
	require.Nil(t, runs)
}

func TestPgQueDequeueNForWorkerQueuesPropagatesRouteLookupError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("route lookup failed")
	q := NewPgQueQueue(&mockDBTX{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return nil, wantErr
		},
	}, nil, PgQueConfig{})

	runs, err := q.DequeueNForWorkerQueues(context.Background(), 1, []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: "worker"},
	})
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, runs)
}

func TestPgQueDequeueNZeroSkipsRouteSetup(t *testing.T) {
	t.Parallel()

	q := NewPgQueQueue(&mockDBTX{
		execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			t.Fatal("zero limit dequeue must not setup route")
			return pgconn.CommandTag{}, nil
		},
	}, nil, PgQueConfig{})

	runs, err := q.DequeueN(context.Background(), 0)
	require.NoError(t, err)
	require.Nil(t, runs)

	runs, err = q.DequeueNByProject(context.Background(), 0, "project-a")
	require.NoError(t, err)
	require.Nil(t, runs)
}

func TestPgQueDequeueFromRouteInvalidBatchAcksAndReturnsEmpty(t *testing.T) {
	t.Parallel()

	var acked bool
	var nacked []string
	db := &mockDBTX{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &noRows{}, nil
		},
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				lag, ok := dest[0].(*int64)
				require.True(t, ok)
				*lag = 0
				return nil
			}}
		},
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			switch {
			case strings.Contains(sql, "pgque.nack"):
				nacked = append(nacked, args[11].(string))
			case strings.Contains(sql, "pgque.ack"):
				acked = true
			default:
				t.Fatalf("unexpected exec SQL = %s", sql)
			}
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{TickInterval: time.Hour})
	state := q.routeState("route-invalid")
	state.configured.Store(true)
	state.lastForceTick = time.Now()
	state.activeBatch = &pgQueActiveBatch{
		BatchID: 7,
		Messages: []pgQueMessage{
			{ID: 1, BatchID: 7, Type: pgQueReadyEventType, Payload: "not-json", CreatedAt: time.Now()},
			{ID: 2, BatchID: 7, Type: pgQueReadyEventType, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{})), CreatedAt: time.Now()},
		},
	}

	runs, err := q.dequeueFromRoute(context.Background(), 1, "route-invalid", pgQueClaimFilter{})
	require.NoError(t, err)
	require.Empty(t, runs)
	require.True(t, acked)
	require.Equal(t, []string{"invalid ready event", "invalid ready event"}, nacked)
	require.Nil(t, state.activeBatch)
}

func TestPgQueDequeueFromRouteNacksUnclaimableCandidates(t *testing.T) {
	t.Parallel()

	var nacked []string
	var acked bool
	queryCalls := 0
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			queryCalls++
			switch {
			case strings.Contains(sql, "COALESCE(priority.priority"):
				return &pgQueCandidateClaimStateRows{values: []pgQueCandidateClaimState{
					{runID: "run-1", priority: 5, hasConcurrencyLimit: false},
				}}, nil
			case strings.Contains(sql, "inserted_claims"):
				return routeErrorRows{}, nil
			case strings.Contains(sql, "pgque.receive"):
				return &noRows{}, nil
			default:
				t.Fatalf("unexpected query SQL = %s", sql)
				return nil, nil
			}
		},
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				lag, ok := dest[0].(*int64)
				require.True(t, ok)
				*lag = 0
				return nil
			}}
		},
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			switch {
			case strings.Contains(sql, "pgque.nack"):
				nacked = append(nacked, args[11].(string))
			case strings.Contains(sql, "pgque.ack"):
				acked = true
			default:
				t.Fatalf("unexpected exec SQL = %s", sql)
			}
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{TickInterval: time.Hour})
	state := q.routeState("route-unclaimable")
	state.configured.Store(true)
	state.lastForceTick = time.Now()
	state.activeBatch = &pgQueActiveBatch{
		BatchID: 8,
		Messages: []pgQueMessage{
			{ID: 1, BatchID: 8, Type: pgQueReadyEventType, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{RunID: "run-1"})), CreatedAt: time.Now()},
		},
	}

	runs, err := q.dequeueFromRoute(context.Background(), 1, "route-unclaimable", pgQueClaimFilter{})
	require.NoError(t, err)
	require.Empty(t, runs)
	require.True(t, acked)
	require.Equal(t, []string{"not claimable"}, nacked)
	require.GreaterOrEqual(t, queryCalls, 3)
}

func TestPgQueDequeueFromRouteReturnsClaimErrorAfterReturningCandidates(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("claim failed")
	var acked bool
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			switch {
			case strings.Contains(sql, "COALESCE(priority.priority"):
				return &pgQueCandidateClaimStateRows{values: []pgQueCandidateClaimState{
					{runID: "run-1", priority: 5, hasConcurrencyLimit: false},
				}}, nil
			case strings.Contains(sql, "inserted_claims"):
				return nil, wantErr
			default:
				t.Fatalf("unexpected query SQL = %s", sql)
				return nil, nil
			}
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "pgque.ack") {
				acked = true
				return pgconn.CommandTag{}, nil
			}
			t.Fatalf("unexpected exec SQL = %s", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	state := q.routeState("route-claim-error")
	state.configured.Store(true)
	batch := &pgQueActiveBatch{
		BatchID: 9,
		Messages: []pgQueMessage{
			{ID: 1, BatchID: 9, Type: pgQueReadyEventType, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{RunID: "run-1"})), CreatedAt: time.Now()},
		},
	}
	state.activeBatch = batch

	runs, err := q.dequeueFromRoute(context.Background(), 1, "route-claim-error", pgQueClaimFilter{})
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, runs)
	require.False(t, acked)
	require.Same(t, batch, state.activeBatch)
	require.Len(t, state.activeBatch.Messages, 1)
	require.EqualValues(t, 1, state.activeBatch.Messages[0].ID)
}

func TestPgQueDequeueFromRouteReturnsFinishError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("ack failed")
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			switch {
			case strings.Contains(sql, "COALESCE(priority.priority"):
				return &pgQueCandidateClaimStateRows{values: []pgQueCandidateClaimState{
					{runID: "run-1", priority: 5, hasConcurrencyLimit: false},
				}}, nil
			case strings.Contains(sql, "inserted_claims"):
				return routeErrorRows{}, nil
			default:
				t.Fatalf("unexpected query SQL = %s", sql)
				return nil, nil
			}
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "pgque.ack") {
				return pgconn.CommandTag{}, wantErr
			}
			if strings.Contains(sql, "pgque.nack") {
				return pgconn.CommandTag{}, nil
			}
			t.Fatalf("unexpected exec SQL = %s", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	state := q.routeState("route-finish-error")
	state.configured.Store(true)
	state.activeBatch = &pgQueActiveBatch{
		BatchID: 10,
		Messages: []pgQueMessage{
			{ID: 1, BatchID: 10, Type: pgQueReadyEventType, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{RunID: "run-1"})), CreatedAt: time.Now()},
		},
	}

	runs, err := q.dequeueFromRoute(context.Background(), 1, "route-finish-error", pgQueClaimFilter{})
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, runs)
	require.False(t, state.activeBatch.Closing)
}

func TestPgQueDequeueFromRouteReturnsClaimedRunsWhenCandidatesRemain(t *testing.T) {
	t.Parallel()

	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			switch {
			case strings.Contains(sql, "COALESCE(priority.priority"):
				return &pgQueCandidateClaimStateRows{values: []pgQueCandidateClaimState{
					{runID: "run-1", priority: 10, hasConcurrencyLimit: false},
					{runID: "run-2", priority: 5, hasConcurrencyLimit: false},
				}}, nil
			case strings.Contains(sql, "inserted_claims"):
				return &pgQueClaimedRunRows{values: []domain.JobRun{{
					ID:        "run-1",
					JobID:     "job-1",
					ProjectID: "project-a",
					Status:    domain.StatusExecuting,
				}}}, nil
			default:
				t.Fatalf("unexpected query SQL = %s", sql)
				return nil, nil
			}
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "pgque.ack") {
				return pgconn.CommandTag{}, nil
			}
			t.Fatalf("unexpected exec SQL = %s", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	state := q.routeState("route-unclaimed-return")
	state.configured.Store(true)
	batch := &pgQueActiveBatch{
		BatchID: 11,
		Messages: []pgQueMessage{
			{ID: 1, BatchID: 11, Type: pgQueReadyEventType, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{RunID: "run-1"})), CreatedAt: time.Now()},
			{ID: 2, BatchID: 11, Type: pgQueReadyEventType, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{RunID: "run-2"})), CreatedAt: time.Now()},
		},
	}
	state.activeBatch = batch

	runs, err := q.dequeueFromRoute(context.Background(), 2, "route-unclaimed-return", pgQueClaimFilter{})
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "run-1", runs[0].ID)
	require.Same(t, batch, state.activeBatch)
	require.Len(t, state.activeBatch.Messages, 1)
	require.EqualValues(t, 2, state.activeBatch.Messages[0].ID)
}

func TestPgQueDequeueFromRouteReturnsPartialClaimWhenFillStops(t *testing.T) {
	t.Parallel()

	receiveErr := errors.New("receive failed")
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			switch {
			case strings.Contains(sql, "COALESCE(priority.priority"):
				return &pgQueCandidateClaimStateRows{values: []pgQueCandidateClaimState{
					{runID: "run-1", priority: 10, hasConcurrencyLimit: false},
				}}, nil
			case strings.Contains(sql, "inserted_claims"):
				return &pgQueClaimedRunRows{values: []domain.JobRun{{
					ID:        "run-1",
					JobID:     "job-1",
					ProjectID: "project-a",
					Status:    domain.StatusExecuting,
				}}}, nil
			case strings.Contains(sql, "pgque.receive"):
				return nil, receiveErr
			default:
				t.Fatalf("unexpected query SQL = %s", sql)
				return nil, nil
			}
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "pgque.ack") || strings.Contains(sql, "pgque.force_next_tick") || strings.Contains(sql, "pgque.ticker") {
				return pgconn.CommandTag{}, nil
			}
			t.Fatalf("unexpected exec SQL = %s", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	state := q.routeState("route-partial-fill")
	state.configured.Store(true)
	state.activeBatch = &pgQueActiveBatch{
		BatchID: 12,
		Messages: []pgQueMessage{
			{ID: 1, BatchID: 12, Type: pgQueReadyEventType, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{RunID: "run-1"})), CreatedAt: time.Now()},
		},
	}

	runs, err := q.dequeueFromRoute(context.Background(), 2, "route-partial-fill", pgQueClaimFilter{})
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "run-1", runs[0].ID)
}

func TestPgQueDequeueFromRouteReturnsPartialClaimWhenEmptyBatchFinishFails(t *testing.T) {
	t.Parallel()

	ackErr := errors.New("ack failed")
	receiveCalls := 0
	ackCalls := 0
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			switch {
			case strings.Contains(sql, "COALESCE(priority.priority"):
				return &pgQueCandidateClaimStateRows{values: []pgQueCandidateClaimState{
					{runID: "run-1", priority: 10, hasConcurrencyLimit: false},
				}}, nil
			case strings.Contains(sql, "inserted_claims"):
				return &pgQueClaimedRunRows{values: []domain.JobRun{{
					ID:        "run-1",
					JobID:     "job-1",
					ProjectID: "project-a",
					Status:    domain.StatusExecuting,
				}}}, nil
			case strings.Contains(sql, "pgque.receive"):
				receiveCalls++
				return &pgQueMessageRows{values: []pgQueMessage{
					{ID: 2, BatchID: 13, Type: pgQueReadyEventType, Payload: "not-json", CreatedAt: time.Now()},
				}}, nil
			default:
				t.Fatalf("unexpected query SQL = %s", sql)
				return nil, nil
			}
		},
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				lag, ok := dest[0].(*int64)
				require.True(t, ok)
				*lag = 0
				return nil
			}}
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			switch {
			case strings.Contains(sql, "pgque.ack"):
				ackCalls++
				if ackCalls == 2 {
					return pgconn.CommandTag{}, ackErr
				}
			case strings.Contains(sql, "pgque.nack"):
			case strings.Contains(sql, "pgque.force_next_tick"), strings.Contains(sql, "pgque.ticker"):
			default:
				t.Fatalf("unexpected exec SQL = %s", sql)
			}
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	state := q.routeState("route-partial-empty-finish")
	state.configured.Store(true)
	state.activeBatch = &pgQueActiveBatch{
		BatchID: 12,
		Messages: []pgQueMessage{
			{ID: 1, BatchID: 12, Type: pgQueReadyEventType, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{RunID: "run-1"})), CreatedAt: time.Now()},
		},
	}

	runs, err := q.dequeueFromRoute(context.Background(), 2, "route-partial-empty-finish", pgQueClaimFilter{})
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "run-1", runs[0].ID)
	require.Equal(t, 1, receiveCalls)
	require.Equal(t, 2, ackCalls)
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

type pgQueMessageRows struct {
	values []pgQueMessage
	idx    int
}

func (r *pgQueMessageRows) Close()                                       {}
func (r *pgQueMessageRows) Err() error                                   { return nil }
func (r *pgQueMessageRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *pgQueMessageRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *pgQueMessageRows) Next() bool {
	if r.idx >= len(r.values) {
		return false
	}
	r.idx++
	return true
}
func (r *pgQueMessageRows) Scan(dest ...any) error {
	if len(dest) != 10 {
		return errors.New("pgQueMessageRows: expected ten destinations")
	}
	msg := r.values[r.idx-1]
	assignments := []struct {
		dest any
		src  any
	}{
		{dest: dest[0], src: msg.ID},
		{dest: dest[1], src: msg.BatchID},
		{dest: dest[2], src: msg.Type},
		{dest: dest[3], src: msg.Payload},
		{dest: dest[4], src: msg.RetryCount},
		{dest: dest[5], src: msg.CreatedAt},
		{dest: dest[6], src: msg.Extra1},
		{dest: dest[7], src: msg.Extra2},
		{dest: dest[8], src: msg.Extra3},
		{dest: dest[9], src: msg.Extra4},
	}
	for _, assignment := range assignments {
		if err := assignPgQueMessageValue(assignment.dest, assignment.src); err != nil {
			return err
		}
	}
	return nil
}
func (r *pgQueMessageRows) Values() ([]any, error) { return nil, nil }
func (r *pgQueMessageRows) RawValues() [][]byte    { return nil }
func (r *pgQueMessageRows) Conn() *pgx.Conn        { return nil }

func assignPgQueMessageValue(dest, src any) error {
	switch ptr := dest.(type) {
	case *int64:
		value, ok := src.(int64)
		if !ok {
			return fmt.Errorf("pgQueMessageRows: source %T is not int64", src)
		}
		*ptr = value
	case *string:
		value, ok := src.(string)
		if !ok {
			return fmt.Errorf("pgQueMessageRows: source %T is not string", src)
		}
		*ptr = value
	case **int32:
		value, ok := src.(*int32)
		if !ok && src != nil {
			return fmt.Errorf("pgQueMessageRows: source %T is not *int32", src)
		}
		*ptr = value
	case *time.Time:
		value, ok := src.(time.Time)
		if !ok {
			return fmt.Errorf("pgQueMessageRows: source %T is not time.Time", src)
		}
		*ptr = value
	case **string:
		value, ok := src.(*string)
		if !ok && src != nil {
			return fmt.Errorf("pgQueMessageRows: source %T is not *string", src)
		}
		*ptr = value
	default:
		return fmt.Errorf("pgQueMessageRows: unsupported destination %T", dest)
	}
	return nil
}

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

func TestPgQueActiveBatchLockedReturnsExistingActiveBatch(t *testing.T) {
	t.Parallel()

	q := NewPgQueQueue(&mockDBTX{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			t.Fatal("existing active batch must not receive from pgque")
			return nil, nil
		},
	}, nil, PgQueConfig{})

	tests := []struct {
		name  string
		batch *pgQueActiveBatch
	}{
		{
			name:  "messages",
			batch: &pgQueActiveBatch{Messages: []pgQueMessage{{ID: 1}}},
		},
		{
			name:  "in flight",
			batch: &pgQueActiveBatch{InFlight: 1},
		},
		{
			name:  "closing",
			batch: &pgQueActiveBatch{Closing: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &pgQueRouteState{activeBatch: tt.batch}

			got, err := q.activeBatchLocked(context.Background(), state, "stq_active")
			require.NoError(t, err)
			require.Same(t, tt.batch, got)
			require.Same(t, tt.batch, state.activeBatch)
		})
	}
}

func TestPgQueActiveBatchLockedPropagatesReceiveAndLagErrors(t *testing.T) {
	t.Parallel()

	t.Run("receive error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("receive failed")
		q := NewPgQueQueue(&mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return nil, wantErr
			},
		}, nil, PgQueConfig{})

		batch, err := q.activeBatchLocked(context.Background(), &pgQueRouteState{}, "stq_receive_error")
		require.ErrorContains(t, err, "pgque receive")
		require.ErrorIs(t, err, wantErr)
		require.Nil(t, batch)
	})

	t.Run("consumer lag error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("lag failed")
		q := NewPgQueQueue(&mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &noRows{}, nil
			},
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return wantErr
				}}
			},
		}, nil, PgQueConfig{})

		batch, err := q.activeBatchLocked(context.Background(), &pgQueRouteState{}, "stq_lag_error")
		require.ErrorContains(t, err, "pgque consumer lag")
		require.ErrorIs(t, err, wantErr)
		require.Nil(t, batch)
	})
}

func TestPgQueActiveBatchLockedStoresReceivedBatch(t *testing.T) {
	t.Parallel()

	createdAt := time.Now()
	q := NewPgQueQueue(&mockDBTX{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &pgQueMessageRows{values: []pgQueMessage{
				{ID: 11, BatchID: 7, Type: pgQueReadyEventType, Payload: "{}", CreatedAt: createdAt},
				{ID: 12, BatchID: 7, Type: pgQueReadyEventType, Payload: "{}", CreatedAt: createdAt},
			}}, nil
		},
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				lag, ok := dest[0].(*int64)
				require.True(t, ok)
				*lag = 3
				return nil
			}}
		},
	}, nil, PgQueConfig{})
	state := &pgQueRouteState{}

	batch, err := q.activeBatchLocked(context.Background(), state, "stq_ready")
	require.NoError(t, err)
	require.NotNil(t, batch)
	require.Same(t, batch, state.activeBatch)
	require.EqualValues(t, 7, batch.BatchID)
	require.Len(t, batch.Messages, 2)
	require.EqualValues(t, 11, batch.Messages[0].ID)
}

func TestPgQueReserveFromActiveBatchReturnsClosingAndDrainedBatches(t *testing.T) {
	t.Parallel()

	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{})

	closingState := &pgQueRouteState{activeBatch: &pgQueActiveBatch{Closing: true}}
	reservation, err := q.reserveFromActiveBatch(context.Background(), closingState, "stq_closing", 1)
	require.NoError(t, err)
	require.Nil(t, reservation.Batch)

	drainedBatch := &pgQueActiveBatch{}
	drainedState := &pgQueRouteState{activeBatch: drainedBatch}
	reservation, err = q.reserveFromActiveBatch(context.Background(), drainedState, "stq_drained", 1)
	require.NoError(t, err)
	require.Same(t, drainedBatch, reservation.Batch)
	require.Empty(t, reservation.Candidates)
	require.Empty(t, reservation.Invalid)
}

func TestPgQueReserveFromActiveBatchHandlesReceiveMissAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("empty receive", func(t *testing.T) {
		t.Parallel()

		q := NewPgQueQueue(&mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &noRows{}, nil
			},
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(dest ...any) error {
					lag, ok := dest[0].(*int64)
					require.True(t, ok)
					*lag = 0
					return nil
				}}
			},
		}, nil, PgQueConfig{})

		reservation, err := q.reserveFromActiveBatch(context.Background(), &pgQueRouteState{}, "stq_empty", 1)
		require.NoError(t, err)
		require.Nil(t, reservation.Batch)
	})

	t.Run("receive error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("receive failed")
		q := NewPgQueQueue(&mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return nil, wantErr
			},
		}, nil, PgQueConfig{})

		reservation, err := q.reserveFromActiveBatch(context.Background(), &pgQueRouteState{}, "stq_error", 1)
		require.ErrorIs(t, err, wantErr)
		require.Nil(t, reservation.Batch)
	})
}

func TestPgQueReserveFromActiveBatchParsesSortsAndLimitsCandidates(t *testing.T) {
	t.Parallel()

	batch := &pgQueActiveBatch{
		BatchID: 50,
		Messages: []pgQueMessage{
			{ID: 1, BatchID: 50, Payload: "not json"},
			{ID: 2, BatchID: 50, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{RunID: "run-low", Generation: 1, Priority: 1}))},
			{ID: 3, BatchID: 50, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{RunID: "run-missing", Generation: 1, Priority: 9}))},
			{ID: 4, BatchID: 50, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{RunID: "run-high", Generation: 1, Priority: 1}))},
			{ID: 5, BatchID: 50, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{RunID: "", Generation: 1, Priority: 1}))},
			{ID: 6, BatchID: 50, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{RunID: "run-also-high", Generation: 1, Priority: 1}))},
		},
	}
	q := NewPgQueQueue(&mockDBTX{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &pgQueCandidateClaimStateRows{values: []pgQueCandidateClaimState{
				{runID: "run-low", priority: 10, hasConcurrencyLimit: false},
				{runID: "run-high", priority: 30, hasConcurrencyLimit: true},
				{runID: "run-also-high", priority: 30, hasConcurrencyLimit: false},
				{runID: "run-missing", priority: 20, hasConcurrencyLimit: false},
			}}, nil
		},
	}, nil, PgQueConfig{})
	state := &pgQueRouteState{activeBatch: batch}

	reservation, err := q.reserveFromActiveBatch(context.Background(), state, "stq_ready", 3)
	require.NoError(t, err)
	require.Same(t, batch, reservation.Batch)
	require.Len(t, reservation.Invalid, 2)
	require.EqualValues(t, 1, reservation.Invalid[0].ID)
	require.EqualValues(t, 5, reservation.Invalid[1].ID)
	require.Len(t, reservation.Candidates, 3)
	require.Equal(t, "run-high", reservation.Candidates[0].Event.RunID)
	require.True(t, reservation.Candidates[0].HasConcurrencyLimit)
	require.Equal(t, "run-also-high", reservation.Candidates[1].Event.RunID)
	require.Equal(t, "run-missing", reservation.Candidates[2].Event.RunID)
	require.Equal(t, 1, batch.InFlight)
	require.Equal(t, []pgQueMessage{{ID: 2, BatchID: 50, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{RunID: "run-low", Generation: 1, Priority: 1}))}}, batch.Messages)
}

func TestPgQueReserveFromActiveBatchPropagatesRefreshError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("claim state failed")
	q := NewPgQueQueue(&mockDBTX{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return nil, wantErr
		},
	}, nil, PgQueConfig{})
	state := &pgQueRouteState{activeBatch: &pgQueActiveBatch{
		Messages: []pgQueMessage{
			{ID: 1, Payload: string(marshalPgQueReadyEvent(pgQueReadyEvent{RunID: "run-1"}))},
		},
	}}

	reservation, err := q.reserveFromActiveBatch(context.Background(), state, "stq_refresh_error", 1)
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, reservation.Batch)
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

func TestScanPgQueReadyRunsWrapsScanAndRowsErrors(t *testing.T) {
	t.Parallel()

	t.Run("scan error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("scan failed")
		runs, err := scanPgQueReadyRuns(&claimScanErrorRows{err: wantErr}, 1, "ready test")
		require.ErrorContains(t, err, "ready test scan")
		require.ErrorIs(t, err, wantErr)
		require.Nil(t, runs)
	})

	t.Run("rows error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("rows failed")
		runs, err := scanPgQueReadyRuns(routeErrorRows{err: wantErr}, 1, "ready test")
		require.ErrorContains(t, err, "ready test rows")
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

func TestMarshalPgQueReadyEventEscapesControlCharacters(t *testing.T) {
	want := pgQueReadyEvent{
		RunID:      "run-\b\f\r\t\x01\x1f",
		RouteKey:   "route-\b\f\r\t\x02\x1e",
		Generation: 7,
		Priority:   4,
	}

	payload := marshalPgQueReadyEventText(want)

	require.Contains(t, payload, `\b`)
	require.Contains(t, payload, `\f`)
	require.Contains(t, payload, `\r`)
	require.Contains(t, payload, `\t`)
	require.Contains(t, payload, `\u0001`)
	require.Contains(t, payload, `\u001f`)
	require.Contains(t, payload, `\u0002`)
	require.Contains(t, payload, `\u001e`)

	var got pgQueReadyEvent
	require.NoError(t, json.Unmarshal([]byte(payload), &got))
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

func TestPgQueRefreshCandidateClaimStateSkipsEmptyInput(t *testing.T) {
	t.Parallel()

	q := NewPgQueQueue(&mockDBTX{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			t.Fatal("empty candidate set must not query claim state")
			return nil, nil
		},
	}, nil, PgQueConfig{})

	require.NoError(t, q.refreshCandidateClaimState(context.Background(), nil))
}

func TestPgQueRefreshCandidateClaimStateWrapsQueryScanAndRowsErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rows    pgx.Rows
		queryFn func() (pgx.Rows, error)
		want    string
	}{
		{
			name: "query error",
			queryFn: func() (pgx.Rows, error) {
				return nil, errors.New("query failed")
			},
			want: "pgque candidate priorities",
		},
		{
			name: "scan error",
			queryFn: func() (pgx.Rows, error) {
				return &claimScanErrorRows{err: errors.New("scan failed")}, nil
			},
			want: "pgque candidate claim state scan",
		},
		{
			name: "rows error",
			queryFn: func() (pgx.Rows, error) {
				return routeErrorRows{err: errors.New("rows failed")}, nil
			},
			want: "pgque candidate claim state rows",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			q := NewPgQueQueue(&mockDBTX{
				queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return tt.queryFn()
				},
			}, nil, PgQueConfig{})
			candidates := []pgQueCandidate{{Event: pgQueReadyEvent{RunID: "run-1"}}}

			err := q.refreshCandidateClaimState(context.Background(), candidates)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestPgQueRefreshCandidateClaimStateUpdatesLargeBatchByMap(t *testing.T) {
	t.Parallel()

	candidates := []pgQueCandidate{
		{Event: pgQueReadyEvent{RunID: "run-1", Priority: 1}},
		{Event: pgQueReadyEvent{RunID: "run-2", Priority: 2}},
		{Event: pgQueReadyEvent{RunID: "run-3", Priority: 3}},
		{Event: pgQueReadyEvent{RunID: "run-4", Priority: 4}},
		{Event: pgQueReadyEvent{RunID: "run-5", Priority: 5}},
		{Event: pgQueReadyEvent{RunID: "run-6", Priority: 6}},
		{Event: pgQueReadyEvent{RunID: "run-7", Priority: 7}},
		{Event: pgQueReadyEvent{RunID: "run-8", Priority: 8}},
		{Event: pgQueReadyEvent{RunID: "run-9", Priority: 9}},
	}
	q := NewPgQueQueue(&mockDBTX{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &pgQueCandidateClaimStateRows{values: []pgQueCandidateClaimState{
				{runID: "run-2", priority: 20, hasConcurrencyLimit: true},
				{runID: "run-9", priority: 90, hasConcurrencyLimit: true},
			}}, nil
		},
	}, nil, PgQueConfig{})

	require.NoError(t, q.refreshCandidateClaimState(context.Background(), candidates))
	require.Equal(t, 1, candidates[0].Event.Priority)
	require.False(t, candidates[0].HasConcurrencyLimit)
	require.Equal(t, 20, candidates[1].Event.Priority)
	require.True(t, candidates[1].HasConcurrencyLimit)
	require.Equal(t, 90, candidates[8].Event.Priority)
	require.True(t, candidates[8].HasConcurrencyLimit)
}

func TestPgQueRefreshCandidateClaimStateWrapsLargeBatchScanAndRowsErrors(t *testing.T) {
	t.Parallel()

	newLargeCandidates := func() []pgQueCandidate {
		candidates := make([]pgQueCandidate, pgQueSmallCandidateSetLimit+1)
		for i := range candidates {
			candidates[i] = pgQueCandidate{Event: pgQueReadyEvent{RunID: fmt.Sprintf("run-%d", i+1)}}
		}
		return candidates
	}

	tests := []struct {
		name string
		rows pgx.Rows
		want string
	}{
		{
			name: "scan error",
			rows: &claimScanErrorRows{err: errors.New("scan failed")},
			want: "pgque candidate claim state scan",
		},
		{
			name: "rows error",
			rows: routeErrorRows{err: errors.New("rows failed")},
			want: "pgque candidate claim state rows",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			q := NewPgQueQueue(&mockDBTX{
				queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return tt.rows, nil
				},
			}, nil, PgQueConfig{})

			err := q.refreshCandidateClaimState(context.Background(), newLargeCandidates())
			require.ErrorContains(t, err, tt.want)
		})
	}
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

func TestRemoveReservedMessagesCoversEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil and empty batch", func(t *testing.T) {
		t.Parallel()

		require.NotPanics(t, func() {
			removeReservedMessages(nil, []pgQueMessage{{ID: 1}}, nil)
		})
		batch := &pgQueActiveBatch{}
		removeReservedMessages(batch, []pgQueMessage{{ID: 1}}, nil)
		require.Empty(t, batch.Messages)
	})

	t.Run("no removals", func(t *testing.T) {
		t.Parallel()

		batch := &pgQueActiveBatch{Messages: []pgQueMessage{{ID: 1}, {ID: 2}}}
		removeReservedMessages(batch, nil, nil)
		require.Equal(t, []pgQueMessage{{ID: 1}, {ID: 2}}, batch.Messages)
	})

	t.Run("all removals", func(t *testing.T) {
		t.Parallel()

		messages := []pgQueMessage{{ID: 1}, {ID: 2}}
		batch := &pgQueActiveBatch{Messages: messages}
		removeReservedMessages(batch, []pgQueMessage{{ID: 1}}, []pgQueCandidate{{Message: pgQueMessage{ID: 2}}})
		require.Empty(t, batch.Messages)
		require.Equal(t, pgQueMessage{}, messages[0])
		require.Equal(t, pgQueMessage{}, messages[1])
	})

	t.Run("single invalid", func(t *testing.T) {
		t.Parallel()

		batch := &pgQueActiveBatch{Messages: []pgQueMessage{{ID: 1}, {ID: 2}, {ID: 3}}}
		removeReservedMessages(batch, []pgQueMessage{{ID: 2}}, nil)
		require.Equal(t, []pgQueMessage{{ID: 1}, {ID: 3}}, batch.Messages)
	})

	t.Run("single candidate", func(t *testing.T) {
		t.Parallel()

		batch := &pgQueActiveBatch{Messages: []pgQueMessage{{ID: 1}, {ID: 2}, {ID: 3}}}
		removeReservedMessages(batch, nil, []pgQueCandidate{{Message: pgQueMessage{ID: 1}}})
		require.Equal(t, []pgQueMessage{{ID: 2}, {ID: 3}}, batch.Messages)
	})

	t.Run("missing single id", func(t *testing.T) {
		t.Parallel()

		batch := &pgQueActiveBatch{Messages: []pgQueMessage{{ID: 1}, {ID: 2}}}
		removeReservedMessage(batch, 99)
		require.Equal(t, []pgQueMessage{{ID: 1}, {ID: 2}}, batch.Messages)
	})
}

func TestRemoveReservedMessagesLargeRemovalSet(t *testing.T) {
	t.Parallel()

	messages := []pgQueMessage{
		{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5},
		{ID: 6}, {ID: 7}, {ID: 8}, {ID: 9}, {ID: 10}, {ID: 11},
	}
	batch := &pgQueActiveBatch{Messages: messages}
	invalid := []pgQueMessage{{ID: 2}, {ID: 4}, {ID: 6}, {ID: 8}, {ID: 10}}
	candidates := []pgQueCandidate{
		{Message: pgQueMessage{ID: 3}},
		{Message: pgQueMessage{ID: 5}},
		{Message: pgQueMessage{ID: 7}},
		{Message: pgQueMessage{ID: 9}},
	}

	removeReservedMessages(batch, invalid, candidates)

	require.Equal(t, []pgQueMessage{{ID: 1}, {ID: 11}}, batch.Messages)
	for _, msg := range messages[len(batch.Messages):] {
		require.Equal(t, pgQueMessage{}, msg)
	}
}

func TestPgQueMessageIDInSet(t *testing.T) {
	t.Parallel()

	require.True(t, pgQueMessageIDInSet([]int64{1, 3, 5}, 3))
	require.False(t, pgQueMessageIDInSet([]int64{1, 3, 5}, 2))
	require.False(t, pgQueMessageIDInSet(nil, 1))
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

func TestPgQueSendReadyEventWrapsRouteSetupError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("create queue failed")
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			require.Contains(t, sql, "strait_pgque_routes")
			return pgconn.CommandTag{}, wantErr
		},
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			require.Failf(t, "test failure", "unexpected QueryRow SQL = %q", sql)
			return &mockRow{}
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

	err := q.sendReadyEvent(ctx, db, &domain.JobRun{ID: "run-a", Status: domain.StatusQueued})

	require.ErrorIs(t, err, wantErr)
}

func TestPgQueSendFreshReadyEventWrapsRouteLookupSetupSendAndRecordErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("route lookup error", func(t *testing.T) {
		wantErr := errors.New("lookup failed")
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				require.Contains(t, sql, "FROM jobs")
				return &mockRow{scanFn: func(...any) error { return wantErr }}
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		err := q.sendFreshReadyEvent(ctx, db, &domain.JobRun{
			ID:            "run-a",
			JobID:         "job-a",
			ProjectID:     "project-a",
			Status:        domain.StatusQueued,
			ExecutionMode: domain.ExecutionModeWorker,
		})

		require.ErrorContains(t, err, "pgque worker route lookup")
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("route setup error", func(t *testing.T) {
		wantErr := errors.New("create queue failed")
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "strait_pgque_routes")
				return pgconn.CommandTag{}, wantErr
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		err := q.sendFreshReadyEvent(ctx, db, &domain.JobRun{ID: "run-a", Status: domain.StatusQueued})

		require.ErrorIs(t, err, wantErr)
	})

	t.Run("send error", func(t *testing.T) {
		wantErr := errors.New("send failed")
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "pgque.send")
				return pgconn.CommandTag{}, wantErr
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
		q.routeState(pgQueHTTPRouteKey).configured.Store(true)

		err := q.sendFreshReadyEvent(ctx, db, &domain.JobRun{ID: "run-a", Status: domain.StatusQueued})

		require.ErrorContains(t, err, "pgque send ready event")
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("record error", func(t *testing.T) {
		wantErr := errors.New("record failed")
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
				if strings.Contains(sql, "strait_pgque_ready_events") {
					return pgconn.CommandTag{}, wantErr
				}
				require.Contains(t, sql, "pgque.send")
				return pgconn.CommandTag{}, nil
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
		q.routeState(pgQueHTTPRouteKey).configured.Store(true)

		err := q.sendFreshReadyEvent(ctx, db, &domain.JobRun{ID: "run-a", Status: domain.StatusQueued})

		require.ErrorContains(t, err, "pgque record ready emits")
		require.ErrorIs(t, err, wantErr)
	})
}

func TestPgQueSendReadyPayloadBatchWrapsRouteSetupAndSendErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("route setup error", func(t *testing.T) {
		wantErr := errors.New("create queue failed")
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "strait_pgque_routes")
				return pgconn.CommandTag{}, wantErr
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		err := q.sendReadyPayloadBatch(ctx, db, pgQueHTTPRouteKey, []string{"{}"})

		require.ErrorIs(t, err, wantErr)
	})

	t.Run("send batch error", func(t *testing.T) {
		wantErr := errors.New("send batch failed")
		db := &mockDBTX{
			execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "pgque.send_batch")
				return pgconn.CommandTag{}, wantErr
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
		q.routeState(pgQueHTTPRouteKey).configured.Store(true)

		err := q.sendReadyPayloadBatch(ctx, db, pgQueHTTPRouteKey, []string{"{}"})

		require.ErrorContains(t, err, "pgque send ready event batch")
		require.ErrorIs(t, err, wantErr)
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

func TestPgQueRecordReadyEmitBatchSkipsEmptyAndWrapsExecError(t *testing.T) {
	ctx := context.Background()
	var execCalls int
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			execCalls++
			require.Contains(t, sql, "strait_pgque_ready_events")
			return pgconn.CommandTag{}, errors.New("insert failed")
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

	require.NoError(t, q.recordReadyEmitBatch(ctx, db, nil, nil))
	require.Equal(t, 0, execCalls)

	err := q.recordReadyEmitBatch(ctx, db, []string{"run-a"}, []int64{3})
	require.ErrorContains(t, err, "pgque record ready emits")
	require.Equal(t, 1, execCalls)
}

func TestNotifyExecutorQueueWakeSkipsEmptyAndWrapsExecError(t *testing.T) {
	ctx := context.Background()
	var execCalls int
	wantErr := errors.New("notify failed")
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			execCalls++
			require.Contains(t, sql, "pg_notify")
			require.Equal(t, QueueWakeChannel, args[0])
			require.Equal(t, "reason:2", args[1])
			return pgconn.CommandTag{}, wantErr
		},
	}

	require.NoError(t, notifyExecutorQueueWake(ctx, db, "reason", 0))
	require.Equal(t, 0, execCalls)

	err := notifyExecutorQueueWake(ctx, db, "reason", 2)
	require.ErrorContains(t, err, "pgque notify executor wake")
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, 1, execCalls)
}

func TestPgQueReconcileReadyRunsHandlesEmptyAndErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("non positive limit skips query", func(t *testing.T) {
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
				require.Failf(t, "test failure", "unexpected Query SQL = %q", sql)
				return nil, nil
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.ReconcileReadyRuns(ctx, 0)

		require.NoError(t, err)
		require.Zero(t, count)
	})

	t.Run("query error", func(t *testing.T) {
		wantErr := errors.New("query failed")
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "job_run_read_state")
				return nil, wantErr
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.ReconcileReadyRuns(ctx, 1)

		require.ErrorContains(t, err, "pgque reconcile ready runs: query")
		require.ErrorIs(t, err, wantErr)
		require.Zero(t, count)
	})

	t.Run("scan error", func(t *testing.T) {
		wantErr := errors.New("scan failed")
		db := &mockDBTX{
			queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
				return &claimScanErrorRows{err: wantErr}, nil
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.ReconcileReadyRuns(ctx, 1)

		require.ErrorContains(t, err, "pgque reconcile ready runs: scan")
		require.ErrorIs(t, err, wantErr)
		require.Zero(t, count)
	})

	t.Run("rows error", func(t *testing.T) {
		wantErr := errors.New("rows failed")
		db := &mockDBTX{
			queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
				return routeErrorRows{err: wantErr}, nil
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.ReconcileReadyRuns(ctx, 1)

		require.ErrorContains(t, err, "pgque reconcile ready runs: rows")
		require.ErrorIs(t, err, wantErr)
		require.Zero(t, count)
	})

	t.Run("empty rows", func(t *testing.T) {
		db := &mockDBTX{
			queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
				return routeErrorRows{}, nil
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.ReconcileReadyRuns(ctx, 1)

		require.NoError(t, err)
		require.Zero(t, count)
	})

	t.Run("send ready event error", func(t *testing.T) {
		wantErr := errors.New("send failed")
		var queryCalls int
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
				queryCalls++
				if strings.Contains(sql, "job_run_read_state") {
					return &pgQueReconcileReadyRows{
						values: []domain.JobRun{{
							ID:            "run-a",
							JobID:         "job-a",
							ProjectID:     "project-a",
							Status:        domain.StatusQueued,
							Attempt:       1,
							Priority:      5,
							ExecutionMode: domain.ExecutionModeHTTP,
						}},
					}, nil
				}
				require.Contains(t, sql, "ready_generation")
				return &pgQueGenerationRows{
					values: []pgQueGenerationRow{{runID: "run-a", generation: 2}},
				}, nil
			},
			execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
				require.Contains(t, sql, "pgque.send_batch")
				return pgconn.CommandTag{}, wantErr
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
		q.routeState(pgQueHTTPRouteKey).configured.Store(true)

		count, err := q.ReconcileReadyRuns(ctx, 1)

		require.ErrorContains(t, err, "pgque send ready event batch")
		require.ErrorIs(t, err, wantErr)
		require.Zero(t, count)
		require.Equal(t, 2, queryCalls)
	})
}

func TestPgQueReadyPromotionHelpersHandleGuardsAndQueryErrors(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("query failed")
	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, wantErr
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

	runs, err := q.promoteDueRunsInTx(ctx, db, 1)
	require.ErrorContains(t, err, "pgque promote due runs")
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, runs)

	runs, err = q.promoteReadyRetriesInTx(ctx, db, 0)
	require.NoError(t, err)
	require.Nil(t, runs)

	runs, err = q.promoteReadyRetriesInTx(ctx, db, 1)
	require.ErrorContains(t, err, "pgque promote ready retries")
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, runs)

	runs, err = q.requeuePausedJobRunsInTx(ctx, db, "workflow-run-a")
	require.ErrorContains(t, err, "pgque requeue paused job runs")
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, runs)
}

func TestPgQueActivateDueRunsHandlesGuardsEmptyPromotionAndCommitErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("non positive limit skips transaction", func(t *testing.T) {
		db := &mockDBTX{}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.ActivateDueRuns(ctx, 0)

		require.NoError(t, err)
		require.Zero(t, count)
	})

	t.Run("requires transaction support", func(t *testing.T) {
		db := &mockDBTX{}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.ActivateDueRuns(ctx, 1)

		require.ErrorContains(t, err, "requires transaction support")
		require.Zero(t, count)
	})

	t.Run("begin error", func(t *testing.T) {
		wantErr := errors.New("begin failed")
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return nil, wantErr
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.ActivateDueRuns(ctx, 1)

		require.ErrorContains(t, err, "pgque activate due runs: begin tx")
		require.ErrorIs(t, err, wantErr)
		require.Zero(t, count)
	})

	t.Run("empty promotion commits", func(t *testing.T) {
		tx := &readyMockTx{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return routeErrorRows{}, nil
			},
		}
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return tx, nil
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.ActivateDueRuns(ctx, 1)

		require.NoError(t, err)
		require.Zero(t, count)
		require.Equal(t, 1, tx.commitCalls)
		require.Equal(t, 1, tx.rollbackCalls)
	})

	t.Run("empty promotion commit error", func(t *testing.T) {
		wantErr := errors.New("commit failed")
		tx := &readyMockTx{
			commitErr: wantErr,
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return routeErrorRows{}, nil
			},
		}
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return tx, nil
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.ActivateDueRuns(ctx, 1)

		require.ErrorContains(t, err, "pgque activate due runs: commit empty promotion")
		require.ErrorIs(t, err, wantErr)
		require.Zero(t, count)
	})
}

func TestPgQueActivateDueRunsSendsReadyEventsAndHandlesNotifyAndCommitErrors(t *testing.T) {
	ctx := context.Background()

	newTx := func(t *testing.T, execErr func(string) error, commitErr error) *readyMockTx {
		t.Helper()
		queryCalls := 0
		return &readyMockTx{
			commitErr: commitErr,
			queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
				queryCalls++
				switch queryCalls {
				case 1, 3:
					require.Contains(t, sql, "scheduled_at")
					return routeErrorRows{}, nil
				case 2:
					require.Contains(t, sql, "job_retries")
					return &pgQueClaimedRunRows{
						values: []domain.JobRun{{
							ID:            "run-a",
							JobID:         "job-a",
							ProjectID:     "project-a",
							Status:        domain.StatusQueued,
							Attempt:       1,
							Priority:      6,
							ExecutionMode: domain.ExecutionModeHTTP,
						}},
					}, nil
				case 4:
					require.Contains(t, sql, "ready_generation")
					return &pgQueGenerationRows{
						values: []pgQueGenerationRow{{runID: "run-a", generation: 9}},
					}, nil
				default:
					require.Failf(t, "test failure", "unexpected Query SQL = %q", sql)
					return nil, nil
				}
			},
			execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
				if execErr != nil {
					if err := execErr(sql); err != nil {
						return pgconn.CommandTag{}, err
					}
				}
				return pgconn.CommandTag{}, nil
			},
		}
	}

	t.Run("notify error", func(t *testing.T) {
		wantErr := errors.New("notify failed")
		tx := newTx(t, func(sql string) error {
			if strings.Contains(sql, "pg_notify") {
				return wantErr
			}
			return nil
		}, nil)
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return tx, nil
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
		q.routeState(pgQueHTTPRouteKey).configured.Store(true)

		count, err := q.ActivateDueRuns(ctx, 2)

		require.ErrorContains(t, err, "pgque notify executor wake")
		require.ErrorIs(t, err, wantErr)
		require.Zero(t, count)
		require.Zero(t, tx.commitCalls)
	})

	t.Run("commit error", func(t *testing.T) {
		wantErr := errors.New("commit failed")
		tx := newTx(t, nil, wantErr)
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return tx, nil
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
		q.routeState(pgQueHTTPRouteKey).configured.Store(true)

		count, err := q.ActivateDueRuns(ctx, 2)

		require.ErrorContains(t, err, "pgque activate due runs: commit")
		require.ErrorIs(t, err, wantErr)
		require.Zero(t, count)
		require.Equal(t, 1, tx.commitCalls)
	})
}

func TestPgQueRequeuePausedJobRunsHandlesGuardsEmptyAndCommitErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("empty workflow id skips transaction", func(t *testing.T) {
		db := &mockDBTX{}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.RequeuePausedJobRuns(ctx, "")

		require.NoError(t, err)
		require.Zero(t, count)
	})

	t.Run("requires transaction support", func(t *testing.T) {
		db := &mockDBTX{}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.RequeuePausedJobRuns(ctx, "workflow-run-a")

		require.ErrorContains(t, err, "requires transaction support")
		require.Zero(t, count)
	})

	t.Run("begin error", func(t *testing.T) {
		wantErr := errors.New("begin failed")
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return nil, wantErr
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.RequeuePausedJobRuns(ctx, "workflow-run-a")

		require.ErrorContains(t, err, "pgque requeue paused job runs: begin tx")
		require.ErrorIs(t, err, wantErr)
		require.Zero(t, count)
	})

	t.Run("empty requeue commits", func(t *testing.T) {
		tx := &readyMockTx{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return routeErrorRows{}, nil
			},
		}
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return tx, nil
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.RequeuePausedJobRuns(ctx, "workflow-run-a")

		require.NoError(t, err)
		require.Zero(t, count)
		require.Equal(t, 1, tx.commitCalls)
		require.Equal(t, 1, tx.rollbackCalls)
	})

	t.Run("commit error", func(t *testing.T) {
		wantErr := errors.New("commit failed")
		tx := &readyMockTx{
			commitErr: wantErr,
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return routeErrorRows{}, nil
			},
		}
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return tx, nil
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

		count, err := q.RequeuePausedJobRuns(ctx, "workflow-run-a")

		require.ErrorContains(t, err, "pgque requeue paused job runs: commit")
		require.ErrorIs(t, err, wantErr)
		require.Zero(t, count)
		require.Equal(t, 1, tx.commitCalls)
	})

	t.Run("send ready event error", func(t *testing.T) {
		wantErr := errors.New("generation failed")
		queryCalls := 0
		tx := &readyMockTx{
			queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
				queryCalls++
				if queryCalls == 1 {
					require.Contains(t, sql, "workflow_step_runs")
					return &pgQueClaimedRunRows{
						values: []domain.JobRun{{
							ID:            "run-a",
							JobID:         "job-a",
							ProjectID:     "project-a",
							Status:        domain.StatusQueued,
							Attempt:       1,
							Priority:      4,
							ExecutionMode: domain.ExecutionModeHTTP,
						}},
					}, nil
				}
				require.Contains(t, sql, "ready_generation")
				return nil, wantErr
			},
		}
		db := &mockTxDBTX{
			beginFn: func(context.Context) (pgx.Tx, error) {
				return tx, nil
			},
		}
		q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
		q.routeState(pgQueHTTPRouteKey).configured.Store(true)

		count, err := q.RequeuePausedJobRuns(ctx, "workflow-run-a")

		require.ErrorContains(t, err, "pgque ready generations")
		require.ErrorIs(t, err, wantErr)
		require.Zero(t, count)
		require.Zero(t, tx.commitCalls)
	})
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

type pgQueClaimedRunRows struct {
	values []domain.JobRun
	idx    int
}

func (r *pgQueClaimedRunRows) Close()                                       {}
func (r *pgQueClaimedRunRows) Err() error                                   { return nil }
func (r *pgQueClaimedRunRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *pgQueClaimedRunRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *pgQueClaimedRunRows) Next() bool {
	if r.idx >= len(r.values) {
		return false
	}
	r.idx++
	return true
}
func (r *pgQueClaimedRunRows) Scan(dest ...any) error {
	if len(dest) != 35 {
		return fmt.Errorf("pgQueClaimedRunRows: got %d destinations, want 35", len(dest))
	}
	run := r.values[r.idx-1]
	now := time.Now()
	assignments := []func() error{
		func() error { return assignStringDest(dest[0], run.ID) },
		func() error { return assignStringDest(dest[1], run.JobID) },
		func() error { return assignStringDest(dest[2], run.ProjectID) },
		func() error { return assignRunStatusDest(dest[3], run.Status) },
		func() error { return assignIntDest(dest[4], max(run.Attempt, 1)) },
		func() error { return assignBytesDest(dest[5], []byte(`{}`)) },
		func() error { return assignBytesDest(dest[6], nil) },
		func() error { return assignBytesDest(dest[7], []byte(`{}`)) },
		func() error { return assignStringPtrDest(dest[8], nil) },
		func() error { return assignStringPtrDest(dest[9], nil) },
		func() error { return assignStringDest(dest[10], "system") },
		func() error { return assignTimePtrDest(dest[11], nil) },
		func() error { return assignTimePtrDest(dest[12], &now) },
		func() error { return assignTimePtrDest(dest[13], nil) },
		func() error { return assignTimePtrDest(dest[14], nil) },
		func() error { return assignTimePtrDest(dest[15], nil) },
		func() error { return assignTimePtrDest(dest[16], nil) },
		func() error { return assignStringPtrDest(dest[17], nil) },
		func() error { return assignIntDest(dest[18], run.Priority) },
		func() error { return assignStringPtrDest(dest[19], nil) },
		func() error { return assignIntDest(dest[20], max(run.JobVersion, 1)) },
		func() error { return assignTimeDest(dest[21], now) },
		func() error { return assignStringPtrDest(dest[22], nil) },
		func() error { return assignBytesDest(dest[23], nil) },
		func() error { return assignBoolDest(dest[24], run.DebugMode) },
		func() error { return assignStringPtrDest(dest[25], nil) },
		func() error { return assignIntDest(dest[26], run.LineageDepth) },
		func() error { return assignBytesDest(dest[27], []byte(`{}`)) },
		func() error { return assignStringPtrDest(dest[28], nil) },
		func() error { return assignStringPtrDest(dest[29], nil) },
		func() error { return assignStringPtrDest(dest[30], nil) },
		func() error { return assignStringPtrDest(dest[31], nil) },
		func() error { return assignStringPtrDest(dest[32], stringPtr(string(domain.ExecutionModeHTTP))) },
		func() error { return assignBoolDest(dest[33], false) },
		func() error { return assignStringPtrDest(dest[34], nil) },
	}
	for _, assign := range assignments {
		if err := assign(); err != nil {
			return err
		}
	}
	return nil
}
func (r *pgQueClaimedRunRows) Values() ([]any, error) { return nil, nil }
func (r *pgQueClaimedRunRows) RawValues() [][]byte    { return nil }
func (r *pgQueClaimedRunRows) Conn() *pgx.Conn        { return nil }

func assignStringDest(dest any, value string) error {
	ptr, ok := dest.(*string)
	if !ok {
		return fmt.Errorf("destination %T is not *string", dest)
	}
	*ptr = value
	return nil
}

func assignRunStatusDest(dest any, value domain.RunStatus) error {
	ptr, ok := dest.(*domain.RunStatus)
	if !ok {
		return fmt.Errorf("destination %T is not *domain.RunStatus", dest)
	}
	*ptr = value
	return nil
}

func assignIntDest(dest any, value int) error {
	ptr, ok := dest.(*int)
	if !ok {
		return fmt.Errorf("destination %T is not *int", dest)
	}
	*ptr = value
	return nil
}

func assignBytesDest(dest any, value []byte) error {
	ptr, ok := dest.(*[]byte)
	if !ok {
		return fmt.Errorf("destination %T is not *[]byte", dest)
	}
	*ptr = value
	return nil
}

func assignStringPtrDest(dest any, value *string) error {
	ptr, ok := dest.(**string)
	if !ok {
		return fmt.Errorf("destination %T is not **string", dest)
	}
	*ptr = value
	return nil
}

func assignTimePtrDest(dest any, value *time.Time) error {
	ptr, ok := dest.(**time.Time)
	if !ok {
		return fmt.Errorf("destination %T is not **time.Time", dest)
	}
	*ptr = value
	return nil
}

func assignTimeDest(dest any, value time.Time) error {
	ptr, ok := dest.(*time.Time)
	if !ok {
		return fmt.Errorf("destination %T is not *time.Time", dest)
	}
	*ptr = value
	return nil
}

func assignBoolDest(dest any, value bool) error {
	ptr, ok := dest.(*bool)
	if !ok {
		return fmt.Errorf("destination %T is not *bool", dest)
	}
	*ptr = value
	return nil
}

func stringPtr(value string) *string {
	return &value
}

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

type pgQueReconcileReadyRows struct {
	values []domain.JobRun
	idx    int
}

func (r *pgQueReconcileReadyRows) Close()                                       {}
func (r *pgQueReconcileReadyRows) Err() error                                   { return nil }
func (r *pgQueReconcileReadyRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *pgQueReconcileReadyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *pgQueReconcileReadyRows) Next() bool {
	if r.idx >= len(r.values) {
		return false
	}
	r.idx++
	return true
}
func (r *pgQueReconcileReadyRows) Scan(dest ...any) error {
	if len(dest) != 8 {
		return errors.New("pgQueReconcileReadyRows: expected eight destinations")
	}
	run := r.values[r.idx-1]
	assignments := []func() error{
		func() error { return assignStringDest(dest[0], run.ID) },
		func() error { return assignStringDest(dest[1], run.JobID) },
		func() error { return assignStringDest(dest[2], run.ProjectID) },
		func() error { return assignRunStatusDest(dest[3], run.Status) },
		func() error { return assignIntDest(dest[4], run.Attempt) },
		func() error { return assignIntDest(dest[5], run.Priority) },
		func() error { return assignExecutionModeDest(dest[6], run.ExecutionMode) },
		func() error { return assignStringDest(dest[7], run.QueueName) },
	}
	for _, assign := range assignments {
		if err := assign(); err != nil {
			return err
		}
	}
	return nil
}
func (r *pgQueReconcileReadyRows) Values() ([]any, error) { return nil, nil }
func (r *pgQueReconcileReadyRows) RawValues() [][]byte    { return nil }
func (r *pgQueReconcileReadyRows) Conn() *pgx.Conn        { return nil }

func assignExecutionModeDest(dest any, value domain.ExecutionMode) error {
	ptr, ok := dest.(*domain.ExecutionMode)
	if !ok {
		return fmt.Errorf("destination %T is not *domain.ExecutionMode", dest)
	}
	*ptr = value
	return nil
}

type readyMockTx struct {
	queryFn       func(context.Context, string, ...any) (pgx.Rows, error)
	execFn        func(context.Context, string, ...any) (pgconn.CommandTag, error)
	queryRowFn    func(context.Context, string, ...any) pgx.Row
	commitErr     error
	rollbackErr   error
	batchResults  pgx.BatchResults
	sentBatch     *pgx.Batch
	commitCalls   int
	rollbackCalls int
}

func (m *readyMockTx) Begin(context.Context) (pgx.Tx, error) { return nil, errors.New("nested") }
func (m *readyMockTx) Commit(context.Context) error {
	m.commitCalls++
	return m.commitErr
}
func (m *readyMockTx) Rollback(context.Context) error {
	m.rollbackCalls++
	return m.rollbackErr
}
func (m *readyMockTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (m *readyMockTx) SendBatch(_ context.Context, b *pgx.Batch) pgx.BatchResults {
	m.sentBatch = b
	return m.batchResults
}
func (m *readyMockTx) LargeObjects() pgx.LargeObjects { return pgx.LargeObjects{} }
func (m *readyMockTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (m *readyMockTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}
func (m *readyMockTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, sql, args...)
	}
	return routeErrorRows{}, nil
}
func (m *readyMockTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, sql, args...)
	}
	return &mockRow{}
}
func (m *readyMockTx) Conn() *pgx.Conn { return nil }

func assertPgQueReadyEvent(t *testing.T, payload string, want pgQueReadyEvent) {
	t.Helper()

	var got pgQueReadyEvent
	require.NoError(t, json.
		Unmarshal([]byte(
			payload),
			&got))
	require.Equal(t, want, got)
}
