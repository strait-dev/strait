package queue

import (
	"context"
	"errors"
	"strings"
	"testing"

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
