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
)

var pgQueCandidateBenchmarkSink []pgQueCandidate
var pgQueWorkerRefArgsBenchmarkSink struct {
	projectIDs     []string
	queueNames     []string
	environmentIDs []string
}
var pgQueClaimSelectionBenchmarkSink pgQueClaimSelection
var pgQueCandidateRunIDsBenchmarkSink []string
var pgQueReadyEmitBatchErrBenchmarkSink error

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
	if err == nil {
		t.Fatal("finishBatchReservation error = nil, want ack failure")
	}
	if !errors.Is(err, ackErr) {
		t.Fatalf("finishBatchReservation error = %v, want %v", err, ackErr)
	}

	state.mu.Lock()
	activeBatch := state.activeBatch
	inFlight := batch.InFlight
	closing := batch.Closing
	state.mu.Unlock()
	if activeBatch != batch {
		t.Fatal("active batch was cleared after ack failure")
	}
	if inFlight != 0 {
		t.Fatalf("batch in-flight = %d, want 0", inFlight)
	}
	if closing {
		t.Fatal("batch stayed closing after ack failure")
	}

	if err := q.finishBatchReservation(ctx, state, batch, nil); err != nil {
		t.Fatalf("finishBatchReservation retry error = %v", err)
	}
	state.mu.Lock()
	activeBatch = state.activeBatch
	state.mu.Unlock()
	if activeBatch != nil {
		t.Fatal("active batch was not cleared after ack retry")
	}
	if ackAttempts != 2 {
		t.Fatalf("ack attempts = %d, want 2", ackAttempts)
	}
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
			if !strings.Contains(sql, "pgque.maint_operations()") {
				t.Fatalf("unexpected query = %q", sql)
			}
			return &stringRows{values: []string{"stq_a", "stq_b"}}, nil
		},
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			calls = append(calls, execCall{sql: sql, args: args})
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

	if err := q.Maintain(ctx); err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	if len(calls) != 3 {
		t.Fatalf("maint calls = %d, want 3", len(calls))
	}
	if !strings.Contains(calls[0].sql, "pgque.maint_rotate_tables_step1") || calls[0].args[0] != "stq_a" {
		t.Fatalf("first maint call = %#v, want step1 for stq_a", calls[0])
	}
	if !strings.Contains(calls[1].sql, "pgque.maint_rotate_tables_step1") || calls[1].args[0] != "stq_b" {
		t.Fatalf("second maint call = %#v, want step1 for stq_b", calls[1])
	}
	if !strings.Contains(calls[2].sql, "pgque.maint_rotate_tables_step2()") {
		t.Fatalf("third maint call = %q, want pgque.maint_rotate_tables_step2()", calls[2].sql)
	}
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
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Maintain() error = %v, want wrapped %v", err, tt.wantErr)
			}
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
	if !slices.Equal(got, want) {
		t.Fatalf("route starts = %v, want %v", got, want)
	}
}

func TestPgQueNextWorkerRouteStartHandlesSmallRouteCounts(t *testing.T) {
	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{})
	if got := q.nextWorkerRouteStart(0); got != 0 {
		t.Fatalf("route start for zero routes = %d, want 0", got)
	}
	if got := q.nextWorkerRouteStart(1); got != 0 {
		t.Fatalf("route start for one route = %d, want 0", got)
	}
	if got := q.nextWorkerRouteStart(2); got != 0 {
		t.Fatalf("first route start for two routes = %d, want 0", got)
	}
	if got := q.nextWorkerRouteStart(2); got != 1 {
		t.Fatalf("second route start for two routes = %d, want 1", got)
	}
}

func TestPgQueLogBackgroundErrorWritesWarning(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{Logger: logger})

	q.logBackgroundError(context.Background(), "pgque ticker failed", errors.New("tick failed"))

	got := buf.String()
	if !strings.Contains(got, "pgque ticker failed") {
		t.Fatalf("log output = %q, want message", got)
	}
	if !strings.Contains(got, "tick failed") {
		t.Fatalf("log output = %q, want error", got)
	}
}

func TestPgQueLogBackgroundErrorSkipsCanceledContext(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
	q := NewPgQueQueue(&mockDBTX{}, nil, PgQueConfig{Logger: logger})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	q.logBackgroundError(ctx, "pgque ticker failed", errors.New("tick failed"))

	if got := buf.String(); got != "" {
		t.Fatalf("log output = %q, want empty output", got)
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
	if !strings.Contains(got, "pgque nack failed") {
		t.Fatalf("log output = %q, want nack failure message", got)
	}
	if !strings.Contains(got, "invalid ready event") {
		t.Fatalf("log output = %q, want nack reason", got)
	}
	if !strings.Contains(got, "nack failed") {
		t.Fatalf("log output = %q, want wrapped nack error", got)
	}
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

	if got := buf.String(); got != "" {
		t.Fatalf("log output = %q, want empty output", got)
	}
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
	if !errors.Is(err, errPgQueNoMessages) {
		t.Fatalf("activeBatchLocked() error = %v, want %v", err, errPgQueNoMessages)
	}
	if batch != nil {
		t.Fatalf("activeBatchLocked() batch = %#v, want nil", batch)
	}
}

func TestUnclaimedReservedCandidates(t *testing.T) {
	candidates := []pgQueCandidate{
		{Event: pgQueReadyEvent{RunID: "run-1"}},
		{Event: pgQueReadyEvent{RunID: "run-2"}},
		{Event: pgQueReadyEvent{RunID: "run-3"}},
	}
	runs := []domain.JobRun{
		{ID: "run-1"},
		{ID: "run-3"},
	}

	unclaimed := unclaimedReservedCandidates(candidates, runs)

	if len(unclaimed) != 1 || unclaimed[0].Event.RunID != "run-2" {
		t.Fatalf("unclaimed candidates = %+v, want run-2", unclaimed)
	}

	allClaimed := unclaimedReservedCandidates(candidates, []domain.JobRun{
		{ID: "run-1"},
		{ID: "run-2"},
		{ID: "run-3"},
	})
	if len(allClaimed) != 0 {
		t.Fatalf("all-claimed unclaimed candidates = %+v, want none", allClaimed)
	}

	noneClaimed := unclaimedReservedCandidates(candidates, nil)
	if !slices.Equal(noneClaimed, candidates) {
		t.Fatalf("none-claimed candidates = %+v, want all candidates", noneClaimed)
	}
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

func TestSelectPgQueClaimCandidates(t *testing.T) {
	candidates := []pgQueCandidate{
		{Event: pgQueReadyEvent{RunID: "run-1", Generation: 10}},
		{Event: pgQueReadyEvent{RunID: "run-2", Generation: 20}, HasConcurrencyLimit: true},
		{Event: pgQueReadyEvent{RunID: "run-3", Generation: 30}},
	}

	selection := selectPgQueClaimCandidates(candidates, 2)

	if !slices.Equal(selection.RunIDs, []string{"run-1", "run-2"}) {
		t.Fatalf("run IDs = %v, want run-1/run-2", selection.RunIDs)
	}
	if !slices.Equal(selection.Generations, []int64{10, 20}) {
		t.Fatalf("generations = %v, want 10/20", selection.Generations)
	}
	if !selection.HasConcurrencyLimit {
		t.Fatal("HasConcurrencyLimit = false, want true")
	}
	if len(selection.Candidates) != 2 {
		t.Fatalf("selected candidates = %d, want 2", len(selection.Candidates))
	}
}

func TestPgQueCandidateRunIDsPreservesOrder(t *testing.T) {
	candidates := []pgQueCandidate{
		{Event: pgQueReadyEvent{RunID: "run-1"}},
		{Event: pgQueReadyEvent{RunID: "run-2"}},
		{Event: pgQueReadyEvent{RunID: "run-3"}},
	}

	runIDs := pgQueCandidateRunIDs(candidates)

	if !slices.Equal(runIDs, []string{"run-1", "run-2", "run-3"}) {
		t.Fatalf("run IDs = %v", runIDs)
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

	for b.Loop() {
		pgQueClaimSelectionBenchmarkSink = selectPgQueClaimCandidates(candidates, len(candidates))
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

	for b.Loop() {
		pgQueCandidateRunIDsBenchmarkSink = pgQueCandidateRunIDs(candidates)
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

func TestWorkerQueueRefArgsFromNormalized(t *testing.T) {
	refs := []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: "default"},
		{ProjectID: "project-a", QueueName: "critical", EnvironmentID: "production"},
		{ProjectID: "project-b", QueueName: "bulk", EnvironmentID: "staging"},
	}

	args := workerQueueRefArgsFromNormalized(refs)

	if !slices.Equal(args.ProjectIDs, []string{"project-a", "project-a", "project-b"}) {
		t.Fatalf("projectIDs = %v", args.ProjectIDs)
	}
	if !slices.Equal(args.QueueNames, []string{"default", "critical", "bulk"}) {
		t.Fatalf("queueNames = %v", args.QueueNames)
	}
	if !slices.Equal(args.EnvironmentIDs, []string{"", "production", "staging"}) {
		t.Fatalf("environmentIDs = %v", args.EnvironmentIDs)
	}
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

	if !slices.Equal(got.ProjectIDs, args.ProjectIDs) {
		t.Fatalf("projectIDs = %v, want %v", got.ProjectIDs, args.ProjectIDs)
	}
	if !slices.Equal(got.QueueNames, args.QueueNames) {
		t.Fatalf("queueNames = %v, want %v", got.QueueNames, args.QueueNames)
	}
	if !slices.Equal(got.EnvironmentIDs, args.EnvironmentIDs) {
		t.Fatalf("environmentIDs = %v, want %v", got.EnvironmentIDs, args.EnvironmentIDs)
	}
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
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("remaining message ids = %v, want %v", gotIDs, wantIDs)
	}
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
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("remaining message ids = %v, want %v", gotIDs, wantIDs)
	}
	for i, msg := range messages[len(batch.Messages):] {
		if msg != (pgQueMessage{}) {
			t.Fatalf("compacted tail message %d = %#v, want zero value", i, msg)
		}
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
				if !ok {
					t.Fatalf("rotation_period key arg type = %T, want string", args[1])
				}
				if arg != "rotation_period" {
					t.Fatalf("rotation_period key = %q, want rotation_period", arg)
				}
				value, ok := args[2].(string)
				if !ok {
					t.Fatalf("rotation_period value arg type = %T, want string", args[2])
				}
				rotationPeriod = value
			}
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{RotationPeriod: 90 * time.Second})

	if err := q.ensureRoute(ctx, db, "http", "stq_test"); err != nil {
		t.Fatalf("ensureRoute() error = %v", err)
	}
	if rotationPeriod != "90000000 microseconds" {
		t.Fatalf("rotation_period = %q, want explicit microsecond interval", rotationPeriod)
	}
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
			if !strings.Contains(sql, "ready_generation") || !strings.Contains(sql, "ANY($1::text[])") {
				t.Fatalf("unexpected ready generation query = %q", sql)
			}
			if len(args) != 1 {
				t.Fatalf("ready generation args = %+v, want run ids", args)
			}
			runIDs, ok := args[0].([]string)
			if !ok {
				t.Fatalf("ready generation arg type = %T, want []string", args[0])
			}
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
			t.Fatalf("unexpected per-run QueryRow SQL = %q", sql)
			return &mockRow{}
		},
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "strait_pgque_ready_events") {
				recordCalls++
				if len(args) != 2 {
					t.Fatalf("ready emit marker args = %+v, want run ids and generations", args)
				}
				runIDs, ok := args[0].([]string)
				if !ok {
					t.Fatalf("ready emit marker run id arg type = %T, want []string", args[0])
				}
				generations, ok := args[1].([]int64)
				if !ok {
					t.Fatalf("ready emit marker generation arg type = %T, want []int64", args[1])
				}
				if !slices.Equal(runIDs, []string{"run-a", "run-b"}) {
					t.Fatalf("ready emit marker run ids = %v, want queued runs", runIDs)
				}
				if !slices.Equal(generations, []int64{11, 12}) {
					t.Fatalf("ready emit marker generations = %v, want queued generations", generations)
				}
				return pgconn.CommandTag{}, nil
			}
			if !strings.Contains(sql, "pgque.send_batch") {
				t.Fatalf("unexpected Exec SQL = %q", sql)
			}
			sendBatchCalls++
			if len(args) != 3 {
				t.Fatalf("pgque.send_batch args = %+v, want queue, event type, and payloads", args)
			}
			eventType, ok := args[1].(string)
			if !ok || eventType != pgQueReadyEventType {
				t.Fatalf("pgque.send_batch event type = %v, want %s", args[1], pgQueReadyEventType)
			}
			payloads, ok := args[2].([]string)
			if !ok {
				t.Fatalf("pgque.send_batch payload arg type = %T, want []string", args[2])
			}
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
	if err := q.sendReadyEvents(ctx, db, runs); err != nil {
		t.Fatalf("sendReadyEvents() error = %v", err)
	}

	if queryCalls != 1 {
		t.Fatalf("ready generation query calls = %d, want 1", queryCalls)
	}
	if queryRowCalls != 0 {
		t.Fatalf("ready generation QueryRow calls = %d, want 0", queryRowCalls)
	}
	if sendBatchCalls != 1 {
		t.Fatalf("send_batch calls = %d, want 1", sendBatchCalls)
	}
	if recordCalls != 1 {
		t.Fatalf("ready emit marker calls = %d, want 1", recordCalls)
	}
	if !slices.Equal(gotRunIDs, []string{"run-a", "run-b"}) {
		t.Fatalf("ready generation run ids = %v, want queued runs only", gotRunIDs)
	}
	if len(sentPayloads) != 2 {
		t.Fatalf("sent payload count = %d, want 2", len(sentPayloads))
	}
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
					t.Fatalf("worker route args = %+v, want job ids", args)
				}
				jobIDs, ok := args[0].([]string)
				if !ok {
					t.Fatalf("worker route arg type = %T, want []string", args[0])
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
				t.Fatalf("unexpected Query SQL = %q", sql)
				return nil, nil
			}
		},
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			queryRowCalls++
			t.Fatalf("unexpected per-run QueryRow SQL = %q", sql)
			return &mockRow{}
		},
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "strait_pgque_ready_events") {
				recordCalls++
				return pgconn.CommandTag{}, nil
			}
			if !strings.Contains(sql, "pgque.send_batch") {
				t.Fatalf("unexpected Exec SQL = %q", sql)
			}
			payloads, ok := args[2].([]string)
			if !ok {
				t.Fatalf("pgque.send_batch payload arg type = %T, want []string", args[2])
			}
			for _, payload := range payloads {
				var event pgQueReadyEvent
				if err := json.Unmarshal([]byte(payload), &event); err != nil {
					t.Fatalf("ready payload is not JSON: %v", err)
				}
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
	if err := q.sendReadyEvents(ctx, db, runs); err != nil {
		t.Fatalf("sendReadyEvents() error = %v", err)
	}

	if jobRouteQueries != 1 {
		t.Fatalf("worker route queries = %d, want 1", jobRouteQueries)
	}
	if !slices.Equal(gotJobIDs, []string{"job-a", "job-b"}) {
		t.Fatalf("worker route job ids = %v, want deduped job-a/job-b", gotJobIDs)
	}
	if generationQueries != 1 {
		t.Fatalf("ready generation queries = %d, want 1", generationQueries)
	}
	if queryRowCalls != 0 {
		t.Fatalf("per-run QueryRow calls = %d, want 0", queryRowCalls)
	}
	if recordCalls != 1 {
		t.Fatalf("ready emit marker calls = %d, want 1", recordCalls)
	}
	if len(sentEvents) != 3 {
		t.Fatalf("sent events = %d, want 3", len(sentEvents))
	}
	wantEvents := map[string]pgQueReadyEvent{
		"run-a": {RunID: "run-a", RouteKey: pgQueWorkerRouteKey("project-a", "default", "prod"), Generation: 11, Priority: 9},
		"run-b": {RunID: "run-b", RouteKey: pgQueWorkerRouteKey("project-a", "critical", "prod"), Generation: 12, Priority: 8},
		"run-c": {RunID: "run-c", RouteKey: pgQueWorkerRouteKey("project-b", "bulk", ""), Generation: 13, Priority: 7},
	}
	for runID, want := range wantEvents {
		if got := sentEvents[runID]; got != want {
			t.Fatalf("ready event for %s = %+v, want %+v", runID, got, want)
		}
	}
}

func TestPgQueReadyRunsForEventsFailsWhenWorkerJobMissing(t *testing.T) {
	ctx := context.Background()
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if !strings.Contains(sql, "FROM jobs") {
				t.Fatalf("unexpected Query SQL = %q", sql)
			}
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
	if err == nil {
		t.Fatal("readyRunsForEvents() error = nil, want missing job error")
	}
	if !strings.Contains(err.Error(), "missing job job-b") {
		t.Fatalf("readyRunsForEvents() error = %v, want missing job-b", err)
	}
}

func TestPgQueSendReadyEventsFailsWhenGenerationMissing(t *testing.T) {
	ctx := context.Background()
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if !strings.Contains(sql, "ready_generation") {
				t.Fatalf("unexpected ready generation query = %q", sql)
			}
			return &pgQueGenerationRows{
				values: []pgQueGenerationRow{
					{runID: "run-a", generation: 11},
				},
			}, nil
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			t.Fatalf("unexpected Exec SQL = %q", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})
	q.routeState(pgQueHTTPRouteKey).configured.Store(true)

	err := q.sendReadyEvents(ctx, db, []*domain.JobRun{
		{ID: "run-a", Status: domain.StatusQueued},
		{ID: "run-b", Status: domain.StatusQueued},
	})
	if err == nil {
		t.Fatal("sendReadyEvents() error = nil, want missing generation")
	}
	if !strings.Contains(err.Error(), "missing run run-b") {
		t.Fatalf("sendReadyEvents() error = %v, want missing run-b", err)
	}
}

func TestPgQueRecordReadyEmitBatchRejectsMismatchedInputs(t *testing.T) {
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			t.Fatalf("unexpected Exec SQL = %q", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

	err := q.recordReadyEmitBatch(context.Background(), db, []string{"run-a"}, nil)
	if err == nil {
		t.Fatal("recordReadyEmitBatch() error = nil, want mismatch error")
	}
	if !strings.Contains(err.Error(), "mismatched id/generation counts") {
		t.Fatalf("recordReadyEmitBatch() error = %v, want mismatch", err)
	}
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
			if !strings.Contains(sql, "ready_generation") {
				t.Fatalf("unexpected QueryRow SQL = %q", sql)
			}
			if len(args) != 1 || args[0] != "run-queued" {
				t.Fatalf("ready generation args = %+v, want run id", args)
			}
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
					t.Fatalf("pgque.send args = %+v, want queue, event type, and payload", args)
				}
				eventType, ok := args[1].(string)
				if !ok || eventType != pgQueReadyEventType {
					t.Fatalf("pgque.send event type = %v, want %s", args[1], pgQueReadyEventType)
				}
				payload, ok := args[2].(string)
				if !ok {
					t.Fatalf("pgque.send payload arg type = %T, want string", args[2])
				}
				sentPayload = payload
			case strings.Contains(sql, "pgque.ticker"):
				if len(args) != 1 {
					t.Fatalf("pgque.ticker args = %+v, want queue", args)
				}
				queueName, ok := args[0].(string)
				if !ok {
					t.Fatalf("pgque.ticker queue arg type = %T, want string", args[0])
				}
				tickedQueue = queueName
			case strings.Contains(sql, "strait_pgque_ready_events"):
				if len(args) != 2 {
					t.Fatalf("ready emit marker args = %+v, want run ids and generations", args)
				}
				runIDs, ok := args[0].([]string)
				if !ok {
					t.Fatalf("ready emit marker run id arg type = %T, want []string", args[0])
				}
				generations, ok := args[1].([]int64)
				if !ok {
					t.Fatalf("ready emit marker generation arg type = %T, want []int64", args[1])
				}
				if !slices.Equal(runIDs, []string{"run-queued"}) || !slices.Equal(generations, []int64{7}) {
					t.Fatalf("ready emit marker = %v/%v, want run-queued generation 7", runIDs, generations)
				}
			default:
				t.Fatalf("unexpected Exec SQL = %q", sql)
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
	if err := q.EnqueueExisting(ctx, run); err != nil {
		t.Fatalf("EnqueueExisting() error = %v", err)
	}

	var event pgQueReadyEvent
	if err := json.Unmarshal([]byte(sentPayload), &event); err != nil {
		t.Fatalf("ready payload is not JSON: %v", err)
	}
	if event.RunID != run.ID || event.RouteKey != pgQueHTTPRouteKey || event.Generation != 7 || event.Priority != 9 {
		t.Fatalf("ready event = %+v, want queued run generation and priority", event)
	}
	if tickedQueue != pgQueQueueName(pgQueHTTPRouteKey) {
		t.Fatalf("ticked queue = %q, want http queue", tickedQueue)
	}
}

func TestPgQueEnqueueExistingIgnoresNonQueuedRun(t *testing.T) {
	ctx := context.Background()
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			t.Fatalf("unexpected QueryRow SQL = %q", sql)
			return &mockRow{}
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			t.Fatalf("unexpected Exec SQL = %q", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresRunWriter(db), PgQueConfig{})

	if err := q.EnqueueExisting(ctx, &domain.JobRun{ID: "run-done", Status: domain.StatusCompleted}); err != nil {
		t.Fatalf("EnqueueExisting(non-queued) error = %v", err)
	}
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
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("ready payload is not JSON: %v", err)
	}
	if got != want {
		t.Fatalf("ready event = %+v, want %+v", got, want)
	}
}
