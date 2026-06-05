package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTryAcquireIdempotencyKeyRequiresTransactionalHandle(t *testing.T) {
	t.Parallel()

	q := New(&mockDBTX{})

	_, _, _, _, err := q.TryAcquireIdempotencyKey(context.Background(), "proj-1", "key-1", time.Minute)
	if err == nil {
		t.Fatal("expected error for non-transactional idempotency store, got nil")
	}
	if !strings.Contains(err.Error(), "requires transactional database handle") {
		t.Fatalf("error = %q, want transactional handle requirement", err.Error())
	}
}
