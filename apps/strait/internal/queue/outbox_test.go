package queue

import (
	"context"
	"testing"
)

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
