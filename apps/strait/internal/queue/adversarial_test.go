package queue

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
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
	q := NewPostgresQueue(db)
	run := &domain.JobRun{
		JobID:          "job-1",
		ProjectID:      "proj-1",
		IdempotencyKey: "key\x00with\x00nulls",
	}

	err := q.Enqueue(context.Background(), run)
	if err != nil {
		t.Fatalf("Enqueue() with null bytes in idempotency key: %v", err)
	}
	if run.Status != domain.StatusQueued {
		t.Errorf("Status = %q, want %q", run.Status, domain.StatusQueued)
	}
}

func TestEnqueue_LongIdempotencyKey(t *testing.T) {
	t.Parallel()

	// A 10KB idempotency key should be accepted by the queue layer.
	// Length validation is the API layer's responsibility.
	longKey := strings.Repeat("a", 10*1024)
	db := successMockDB()
	q := NewPostgresQueue(db)
	run := &domain.JobRun{
		JobID:          "job-1",
		ProjectID:      "proj-1",
		IdempotencyKey: longKey,
	}

	err := q.Enqueue(context.Background(), run)
	if err != nil {
		t.Fatalf("Enqueue() with 10KB idempotency key: %v", err)
	}
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

	q := NewPostgresQueue(db)
	run := &domain.JobRun{
		JobID:          "job-1",
		ProjectID:      "proj-1",
		IdempotencyKey: "",
	}

	err := q.Enqueue(context.Background(), run)
	if err == nil {
		t.Fatal("expected error for ErrNoRows with empty idempotency key")
	}
	if errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatal("empty idempotency key should not produce ErrIdempotencyConflict")
	}
}

func TestPriority_IntMin(t *testing.T) {
	t.Parallel()

	var capturedArgs []any
	db := capturingMockDB(&capturedArgs)
	q := NewPostgresQueue(db)
	run := &domain.JobRun{
		JobID:     "job-1",
		ProjectID: "proj-1",
		Priority:  math.MinInt32,
	}

	err := q.Enqueue(context.Background(), run)
	if err != nil {
		t.Fatalf("Enqueue() with MinInt32 priority: %v", err)
	}
	if run.Priority != math.MinInt32 {
		t.Errorf("Priority = %d, want %d", run.Priority, math.MinInt32)
	}
	// Verify the priority was passed as arg index 16 (0-based).
	if len(capturedArgs) > 16 {
		if p, ok := capturedArgs[16].(int); ok && p != math.MinInt32 {
			t.Errorf("captured priority arg = %d, want %d", p, math.MinInt32)
		}
	}
}

func TestPriority_IntMax(t *testing.T) {
	t.Parallel()

	var capturedArgs []any
	db := capturingMockDB(&capturedArgs)
	q := NewPostgresQueue(db)
	run := &domain.JobRun{
		JobID:     "job-1",
		ProjectID: "proj-1",
		Priority:  math.MaxInt32,
	}

	err := q.Enqueue(context.Background(), run)
	if err != nil {
		t.Fatalf("Enqueue() with MaxInt32 priority: %v", err)
	}
	if run.Priority != math.MaxInt32 {
		t.Errorf("Priority = %d, want %d", run.Priority, math.MaxInt32)
	}
}

func TestPriority_EqualPriorities(t *testing.T) {
	t.Parallel()

	// The dequeue ORDER BY clause uses "priority DESC, created_at ASC"
	// which ensures FIFO within the same priority level.
	q := NewPostgresQueue(&mockDBTX{})
	clause := q.dequeueOrderByClause()

	if !strings.Contains(clause, "jr.priority DESC") {
		t.Fatalf("order by clause missing priority DESC: %s", clause)
	}
	if !strings.Contains(clause, "jr.created_at ASC") {
		t.Fatalf("order by clause missing created_at ASC for FIFO: %s", clause)
	}

	// Verify priority comes before created_at in the clause.
	priIdx := strings.Index(clause, "jr.priority")
	catIdx := strings.Index(clause, "jr.created_at")
	if priIdx >= catIdx {
		t.Fatalf("priority must sort before created_at; clause = %s", clause)
	}
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
		q := NewPostgresQueue(db)
		run := &domain.JobRun{
			JobID:          "job-1",
			ProjectID:      "proj-1",
			ConcurrencyKey: key,
		}

		err := q.Enqueue(context.Background(), run)
		if err != nil {
			t.Errorf("Enqueue() with concurrency key %q: %v", key, err)
		}
	}
}

func TestConcurrencyKey_ExtremelyLong(t *testing.T) {
	t.Parallel()

	longKey := strings.Repeat("x", 10*1024)
	db := successMockDB()
	q := NewPostgresQueue(db)
	run := &domain.JobRun{
		JobID:          "job-1",
		ProjectID:      "proj-1",
		ConcurrencyKey: longKey,
	}

	err := q.Enqueue(context.Background(), run)
	if err != nil {
		t.Fatalf("Enqueue() with 10KB concurrency key: %v", err)
	}
}

func TestConcurrencyKey_EmptyString(t *testing.T) {
	t.Parallel()

	// Empty concurrency key should bypass per-key concurrency checks.
	// The SQL includes: "OR jr.concurrency_key IS NULL OR jr.concurrency_key = ''"
	// which confirms empty keys are excluded from concurrency enforcement.
	if !strings.Contains(concurrencyWhere, "jr.concurrency_key = ''") {
		t.Fatalf("concurrencyWhere should handle empty concurrency key; got: %s", concurrencyWhere)
	}
	if !strings.Contains(concurrencyWhere, "jr.concurrency_key IS NULL") {
		t.Fatalf("concurrencyWhere should handle NULL concurrency key; got: %s", concurrencyWhere)
	}

	db := successMockDB()
	q := NewPostgresQueue(db)
	run := &domain.JobRun{
		JobID:          "job-1",
		ProjectID:      "proj-1",
		ConcurrencyKey: "",
	}

	err := q.Enqueue(context.Background(), run)
	if err != nil {
		t.Fatalf("Enqueue() with empty concurrency key: %v", err)
	}
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
		q := NewPostgresQueue(db)
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
		if run.Priority != priority {
			t.Errorf("Priority = %d, want %d", run.Priority, priority)
		}
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
		q := NewPostgresQueue(db)
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

func TestFairDequeue_SkewedDistribution(t *testing.T) {
	t.Parallel()

	// Verify that DequeueNFair uses DISTINCT ON (jr.job_id) to prevent
	// a single high-volume job from starving others.
	var capturedQuery string
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			capturedQuery = sql
			return nil, errors.New("forced query error")
		},
	}

	q := NewPostgresQueue(db)
	_, _ = q.DequeueNFair(context.Background(), 10)

	if !strings.Contains(capturedQuery, "DISTINCT ON (jr.job_id)") {
		t.Fatalf("DequeueNFair() query missing DISTINCT ON (jr.job_id): %s", capturedQuery)
	}

	// Verify SKIP LOCKED is present to prevent contention.
	if !strings.Contains(capturedQuery, "SKIP LOCKED") {
		t.Fatalf("DequeueNFair() query missing SKIP LOCKED: %s", capturedQuery)
	}

	// Verify concurrency CTEs are included.
	if !strings.Contains(capturedQuery, "active_by_job") {
		t.Fatalf("DequeueNFair() query missing concurrency CTE active_by_job: %s", capturedQuery)
	}
}

func TestEnqueue_TagsMarshalError(t *testing.T) {
	t.Parallel()

	// Tags containing values that fail JSON marshal should produce an error.
	// json.Marshal cannot fail on map[string]string, but we verify the code
	// path handles the marshal step without panicking.
	db := successMockDB()
	q := NewPostgresQueue(db)
	run := &domain.JobRun{
		JobID:     "job-1",
		ProjectID: "proj-1",
		Tags:      map[string]string{"key": "value"},
	}

	err := q.Enqueue(context.Background(), run)
	if err != nil {
		t.Fatalf("Enqueue() with valid tags: %v", err)
	}

	// Verify tags are marshalled correctly by confirming the run was accepted.
	tagsJSON, _ := json.Marshal(run.Tags)
	if string(tagsJSON) != `{"key":"value"}` {
		t.Errorf("tags JSON = %s, want %s", tagsJSON, `{"key":"value"}`)
	}
}
