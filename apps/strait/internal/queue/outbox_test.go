package queue

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// fakeTx is a minimal pgx.Tx stub used by the outbox unit tests. The
// unit tests exercise validation only; the integration tests cover the
// real SQL path.
type fakeTx struct{ pgx.Tx }

func (f *fakeTx) Exec(ctx context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil
}

func TestWriteOutboxInTx_EmptyEntries(t *testing.T) {
	if err := WriteOutboxInTx(context.Background(), nil, nil); err != nil {
		t.Errorf("nil entries should pass: %v", err)
	}
	if err := WriteOutboxInTx(context.Background(), nil, []OutboxEntry{}); err != nil {
		t.Errorf("empty slice should pass: %v", err)
	}
}

func TestWriteOutboxInTx_MissingProjectIDRejected(t *testing.T) {
	err := WriteOutboxInTx(context.Background(), nil, []OutboxEntry{{
		JobID: "job1",
	}})
	if err == nil {
		t.Error("missing project_id should error")
	}
}

func TestWriteOutboxInTx_MissingJobIDRejected(t *testing.T) {
	err := WriteOutboxInTx(context.Background(), nil, []OutboxEntry{{
		ProjectID: "p1",
	}})
	if err == nil {
		t.Error("missing job_id should error")
	}
}

func TestThrottledError_Round2Phase11Placeholder(t *testing.T) {
	// Placeholder kept to ensure the outbox tests coexist with the
	// backpressure tests in the same package.
	_ = &ThrottledError{}
}
