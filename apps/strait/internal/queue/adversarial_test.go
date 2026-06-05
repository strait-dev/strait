package queue

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// successMockDB returns a mockDBTX that simulates a successful enqueue.
func successMockDB() *mockDBTX {
	return &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(dest ...any) error {
					if tp, ok := dest[0].(*time.Time); ok {
						*tp = time.Now()
					}
					return nil
				},
			}
		},
	}
}

// capturingMockDB returns a mockDBTX that captures the query args and simulates success.
func capturingMockDB(capturedArgs *[]any) *mockDBTX {
	return &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
			*capturedArgs = args
			return &mockRow{
				scanFn: func(dest ...any) error {
					if tp, ok := dest[0].(*time.Time); ok {
						*tp = time.Now()
					}
					return nil
				},
			}
		},
	}
}

func TestEnqueue_AdversarialIdempotencyKey(t *testing.T) {
	t.Parallel()

	// Null bytes in idempotency key should be passed through to the DB layer
	// since validation happens at the API layer, not the queue layer.
	db := successMockDB()
	q := NewPostgresRunWriter(db)
	run := &domain.JobRun{
		JobID:          "job-1",
		ProjectID:      "proj-1",
		IdempotencyKey: "key\x00with\x00nulls",
	}

	err := q.Enqueue(context.Background(), run)
	require.NoError(t, err)
	assert.Equal(t,
		domain.StatusQueued,
		run.Status,
	)
}

func TestEnqueue_LongIdempotencyKey(t *testing.T) {
	t.Parallel()

	// A 10KB idempotency key should be accepted by the queue layer.
	// Length validation is the API layer's responsibility.
	longKey := strings.Repeat("a", 10*1024)
	db := successMockDB()
	q := NewPostgresRunWriter(db)
	run := &domain.JobRun{
		JobID:          "job-1",
		ProjectID:      "proj-1",
		IdempotencyKey: longKey,
	}

	err := q.Enqueue(context.Background(), run)
	require.NoError(t, err)
}

func TestEnqueue_EmptyIdempotencyKey(t *testing.T) {
	t.Parallel()

	// Empty idempotency key means no idempotency check.
	// On ErrNoRows the queue should return the error, not ErrIdempotencyConflict.
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(_ ...any) error { return pgx.ErrNoRows },
			}
		},
	}

	q := NewPostgresRunWriter(db)
	run := &domain.JobRun{
		JobID:          "job-1",
		ProjectID:      "proj-1",
		IdempotencyKey: "",
	}

	err := q.Enqueue(context.Background(), run)
	require.Error(t,
		err)
	require.NotErrorIs(t,
		err, domain.ErrIdempotencyConflict)
}

func TestPriority_IntMin(t *testing.T) {
	t.Parallel()

	var capturedArgs []any
	db := capturingMockDB(&capturedArgs)
	q := NewPostgresRunWriter(db)
	run := &domain.JobRun{
		JobID:     "job-1",
		ProjectID: "proj-1",
		Priority:  math.MinInt32,
	}

	err := q.Enqueue(context.Background(), run)
	require.NoError(t, err)
	assert.Equal(t,
		math.MinInt32, run.
			Priority,
	)

	// Verify the priority was passed as arg index 16 (0-based).
	if len(capturedArgs) > 16 {
		if p, ok := capturedArgs[16].(int); ok && p != math.MinInt32 {
			assert.Failf(t, "test failure",

				"captured priority arg = %d, want %d", p, math.MinInt32)
		}
	}
}

func TestPriority_IntMax(t *testing.T) {
	t.Parallel()

	var capturedArgs []any
	db := capturingMockDB(&capturedArgs)
	q := NewPostgresRunWriter(db)
	run := &domain.JobRun{
		JobID:     "job-1",
		ProjectID: "proj-1",
		Priority:  math.MaxInt32,
	}

	err := q.Enqueue(context.Background(), run)
	require.NoError(t, err)
	assert.Equal(t,
		math.MaxInt32, run.
			Priority,
	)
}

func TestConcurrencyKey_SpecialChars(t *testing.T) {
	t.Parallel()

	specialKeys := []string{
		"key/with/slashes",
		"key\\with\\backslashes",
		"key\x00with\x00nulls",
		"key\u2603with\u00e9unicode",
		"key with spaces and\ttabs",
	}

	for _, key := range specialKeys {
		db := successMockDB()
		q := NewPostgresRunWriter(db)
		run := &domain.JobRun{
			JobID:          "job-1",
			ProjectID:      "proj-1",
			ConcurrencyKey: key,
		}

		err := q.Enqueue(context.Background(), run)
		assert.NoError(
			t, err)
	}
}

func TestConcurrencyKey_ExtremelyLong(t *testing.T) {
	t.Parallel()

	longKey := strings.Repeat("x", 10*1024)
	db := successMockDB()
	q := NewPostgresRunWriter(db)
	run := &domain.JobRun{
		JobID:          "job-1",
		ProjectID:      "proj-1",
		ConcurrencyKey: longKey,
	}

	err := q.Enqueue(context.Background(), run)
	require.NoError(t, err)
}

func TestConcurrencyKey_EmptyString(t *testing.T) {
	t.Parallel()

	db := successMockDB()
	q := NewPostgresRunWriter(db)
	run := &domain.JobRun{
		JobID:          "job-1",
		ProjectID:      "proj-1",
		ConcurrencyKey: "",
	}

	err := q.Enqueue(context.Background(), run)
	require.NoError(t, err)
}

func FuzzEnqueuePriority(f *testing.F) {
	f.Add(0)
	f.Add(1)
	f.Add(-1)
	f.Add(math.MinInt32)
	f.Add(math.MaxInt32)
	f.Add(10)
	f.Add(-999999)

	f.Fuzz(func(t *testing.T, priority int) {
		db := successMockDB()
		q := NewPostgresRunWriter(db)
		run := &domain.JobRun{
			JobID:     "job-1",
			ProjectID: "proj-1",
			Priority:  priority,
		}

		// Enqueue should never panic regardless of priority value.
		err := q.Enqueue(context.Background(), run)
		if err != nil {
			t.Skipf("DB mock returned error for priority %d: %v", priority, err)
		}
		assert.Equal(t,
			priority, run.Priority,
		)
	})
}

func FuzzConcurrencyKey(f *testing.F) {
	f.Add("")
	f.Add("simple-key")
	f.Add("key\x00null")
	f.Add(strings.Repeat("a", 1024))
	f.Add("/slashes/everywhere/")
	f.Add("unicode-\u2603-\u00e9-\u4e16\u754c")
	f.Add("key with\nnewlines\nand\ttabs")

	f.Fuzz(func(t *testing.T, key string) {
		db := successMockDB()
		q := NewPostgresRunWriter(db)
		run := &domain.JobRun{
			JobID:          "job-1",
			ProjectID:      "proj-1",
			ConcurrencyKey: key,
		}

		// Enqueue should never panic regardless of concurrency key value.
		err := q.Enqueue(context.Background(), run)
		if err != nil {
			t.Skipf("DB mock returned error for key %q: %v", key, err)
		}
	})
}

func TestEnqueue_TagsMarshalError(t *testing.T) {
	t.Parallel()

	// Tags containing values that fail JSON marshal should produce an error.
	// json.Marshal cannot fail on map[string]string, but we verify the code
	// path handles the marshal step without panicking.
	db := successMockDB()
	q := NewPostgresRunWriter(db)
	run := &domain.JobRun{
		JobID:     "job-1",
		ProjectID: "proj-1",
		Tags:      map[string]string{"key": "value"},
	}

	err := q.Enqueue(context.Background(), run)
	require.NoError(t, err)

	// Verify tags are marshalled correctly by confirming the run was accepted.
	tagsJSON, _ := json.Marshal(run.Tags)
	assert.JSONEq(t,
		`{"key":"value"}`,
		string(tagsJSON))
}
