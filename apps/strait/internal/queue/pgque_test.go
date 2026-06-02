package queue

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

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
	q := NewPgQueQueue(db, NewPostgresQueue(db), PgQueConfig{})
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
	q := NewPgQueQueue(db, NewPostgresQueue(db), PgQueConfig{})

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
			q := NewPgQueQueue(db, NewPostgresQueue(db), PgQueConfig{})

			err := q.Maintain(ctx)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Maintain() error = %v, want wrapped %v", err, tt.wantErr)
			}
		})
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

func TestPgQueActiveBatchLockedReturnsSentinelForEmptyReceive(t *testing.T) {
	ctx := context.Background()
	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &emptyRows{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresQueue(db), PgQueConfig{})

	batch, err := q.activeBatchLocked(ctx, &pgQueRouteState{}, "stq_empty")
	if !errors.Is(err, errPgQueNoMessages) {
		t.Fatalf("activeBatchLocked() error = %v, want %v", err, errPgQueNoMessages)
	}
	if batch != nil {
		t.Fatalf("activeBatchLocked() batch = %#v, want nil", batch)
	}
}

func TestPgQueEnsureRouteConfiguresRotationPeriod(t *testing.T) {
	ctx := context.Background()
	var rotationPeriod string
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "pgque.set_queue_config") && strings.Contains(sql, "rotation_period") && len(args) == 2 {
				rotationPeriod, _ = args[1].(string)
			}
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewPgQueQueue(db, NewPostgresQueue(db), PgQueConfig{RotationPeriod: 90 * time.Second})

	if err := q.ensureRoute(ctx, db, "http", "stq_test"); err != nil {
		t.Fatalf("ensureRoute() error = %v", err)
	}
	if rotationPeriod != "90000000 microseconds" {
		t.Fatalf("rotation_period = %q, want explicit microsecond interval", rotationPeriod)
	}
}
