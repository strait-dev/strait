package worker

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
)

// endpointErrHash returns the error hash for an EndpointError, matching the
// internal poison-pill hash while keeping the public error string redacted.
func endpointErrHash(statusCode int, body string) string {
	return errorHashForError(&domain.EndpointError{StatusCode: statusCode, Body: body})
}

// errorHash unit tests

func TestErrorHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		a, b   string
		expect string // "same" or "different"
	}{
		{
			name:   "deterministic",
			a:      "500 internal server error",
			b:      "500 internal server error",
			expect: "same",
		},
		{
			name:   "different input yields different hash",
			a:      "500 internal server error",
			b:      "connection refused",
			expect: "different",
		},
		{
			name:   "truncation at 200 chars: 201-char string matches first 200",
			a:      strings.Repeat("x", 201),
			b:      strings.Repeat("x", 200),
			expect: "same",
		},
		{
			name:   "exactly 200 chars: no truncation",
			a:      strings.Repeat("a", 200),
			b:      strings.Repeat("a", 200),
			expect: "same",
		},
		{
			name:   "199 chars vs 200 chars differ",
			a:      strings.Repeat("a", 199),
			b:      strings.Repeat("a", 200),
			expect: "different",
		},
		{
			name:   "empty string produces valid hash",
			a:      "",
			b:      "",
			expect: "same",
		},
		{
			name:   "whitespace sensitivity: trailing space matters",
			a:      "error",
			b:      "error ",
			expect: "different",
		},
		{
			name:   "unicode: multi-byte chars at boundary",
			a:      strings.Repeat("\U0001f600", 50), // 200 bytes of emoji
			b:      strings.Repeat("\U0001f600", 50),
			expect: "same",
		},
		{
			name:   "very long identical prefix: differ after 200 chars",
			a:      strings.Repeat("z", 200) + "AAA",
			b:      strings.Repeat("z", 200) + "BBB",
			expect: "same", // by design: only first 200 chars are hashed
		},
		{
			name:   "rune-based truncation: 201 multi-byte runes truncated at rune boundary",
			a:      strings.Repeat("\U0001f600", 201), // 201 runes, 804 bytes
			b:      strings.Repeat("\U0001f600", 200), // 200 runes, 800 bytes
			expect: "same",                            // truncated to 200 runes, not 200 bytes
		},
		{
			name:   "rune-based: 199 multi-byte runes differ from 200",
			a:      strings.Repeat("\U0001f600", 199),
			b:      strings.Repeat("\U0001f600", 200),
			expect: "different", // 199 runes != 200 runes
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ha := errorHash(tc.a)
			hb := errorHash(tc.b)

			if ha == "" {
				t.Fatal("errorHash returned empty string")
			}
			if len(ha) != 16 {
				t.Errorf("expected 16-char hex hash, got %d chars: %q", len(ha), ha)
			}

			if tc.expect == "same" && ha != hb {
				t.Errorf("expected same hash, got %q vs %q", ha, hb)
			}
			if tc.expect == "different" && ha == hb {
				t.Errorf("expected different hashes for %q vs %q, both got %q", tc.a, tc.b, ha)
			}
		})
	}
}

// Table-driven poison pill detection tests

func TestHandleFailure_PoisonPillDetection(t *testing.T) {
	t.Parallel()

	longError := strings.Repeat("X", 300)

	tests := []struct {
		name           string
		threshold      *int
		attempt        int
		maxAttempts    int
		prevHash       string
		prevCount      string
		errInput       error
		expectStatus   domain.RunStatus
		expectPoisoned bool   // expect "poison pill detected" in error field
		expectCount    string // expected _error_hash_count after processing
	}{
		{
			name:           "same error hits threshold=3",
			threshold:      new(3),
			attempt:        3,
			maxAttempts:    10,
			prevHash:       endpointErrHash(500, "500 internal server error"),
			prevCount:      "2",
			errInput:       &domain.EndpointError{StatusCode: 500, Body: "500 internal server error"},
			expectStatus:   domain.StatusDeadLetter,
			expectPoisoned: true,
			expectCount:    "3",
		},
		{
			name:         "same error below threshold",
			threshold:    new(3),
			attempt:      2,
			maxAttempts:  10,
			prevHash:     endpointErrHash(500, "500 err"),
			prevCount:    "1",
			errInput:     &domain.EndpointError{StatusCode: 500, Body: "500 err"},
			expectStatus: domain.StatusQueued,
			expectCount:  "2",
		},
		{
			name:         "different error resets count",
			threshold:    new(3),
			attempt:      3,
			maxAttempts:  10,
			prevHash:     endpointErrHash(500, "500 err"),
			prevCount:    "2",
			errInput:     &domain.EndpointError{StatusCode: 500, Body: "connection refused"},
			expectStatus: domain.StatusQueued,
			expectCount:  "1",
		},
		{
			name:         "threshold nil (disabled)",
			threshold:    nil,
			attempt:      5,
			maxAttempts:  10,
			prevHash:     endpointErrHash(500, "err"),
			prevCount:    "4",
			errInput:     &domain.EndpointError{StatusCode: 500, Body: "err"},
			expectStatus: domain.StatusQueued,
		},
		{
			name:         "threshold 0 (disabled)",
			threshold:    new(0),
			attempt:      5,
			maxAttempts:  10,
			prevHash:     endpointErrHash(500, "err"),
			prevCount:    "4",
			errInput:     &domain.EndpointError{StatusCode: 500, Body: "err"},
			expectStatus: domain.StatusQueued,
		},
		{
			name:           "threshold=1 first failure DLQs immediately",
			threshold:      new(1),
			attempt:        1,
			maxAttempts:    10,
			prevHash:       "",
			prevCount:      "",
			errInput:       &domain.EndpointError{StatusCode: 500, Body: "any error"},
			expectStatus:   domain.StatusDeadLetter,
			expectPoisoned: true,
			expectCount:    "1",
		},
		{
			name:         "threshold=2 first attempt: count starts at 1",
			threshold:    new(2),
			attempt:      1,
			maxAttempts:  10,
			prevHash:     "",
			prevCount:    "",
			errInput:     &domain.EndpointError{StatusCode: 500, Body: "err"},
			expectStatus: domain.StatusQueued,
			expectCount:  "1",
		},
		{
			name:         "max attempts exhausted before threshold",
			threshold:    new(3),
			attempt:      5,
			maxAttempts:  5,
			prevHash:     endpointErrHash(500, "err"),
			prevCount:    "2",
			errInput:     &domain.EndpointError{StatusCode: 500, Body: "err"},
			expectStatus: domain.StatusDeadLetter,
			// DLQ'd via max attempts (shouldRetry=false before poison pill runs),
			// metadata is not modified so not included in update fields
			expectPoisoned: false,
		},
		{
			name:         "non-retryable class (client 400) skips poison pill",
			threshold:    new(3),
			attempt:      1,
			maxAttempts:  10,
			prevHash:     "",
			prevCount:    "",
			errInput:     &domain.EndpointError{StatusCode: 400, Body: "bad request"},
			expectStatus: domain.StatusDeadLetter,
			// DLQ'd via class, not poison pill
			expectPoisoned: false,
		},
		{
			name:           "error > 200 chars: hash matches truncated prefix",
			threshold:      new(3),
			attempt:        3,
			maxAttempts:    10,
			prevHash:       endpointErrHash(500, longError),
			prevCount:      "2",
			errInput:       &domain.EndpointError{StatusCode: 500, Body: longError},
			expectStatus:   domain.StatusDeadLetter,
			expectPoisoned: true,
			expectCount:    "3",
		},
		{
			name:           "threshold exactly at boundary (count=3, threshold=3)",
			threshold:      new(3),
			attempt:        3,
			maxAttempts:    10,
			prevHash:       endpointErrHash(500, "err"),
			prevCount:      "2",
			errInput:       &domain.EndpointError{StatusCode: 500, Body: "err"},
			expectStatus:   domain.StatusDeadLetter,
			expectPoisoned: true,
			expectCount:    "3",
		},
		{
			name:         "one below boundary (count=2, threshold=3)",
			threshold:    new(3),
			attempt:      3,
			maxAttempts:  10,
			prevHash:     endpointErrHash(500, "err"),
			prevCount:    "1",
			errInput:     &domain.EndpointError{StatusCode: 500, Body: "err"},
			expectStatus: domain.StatusQueued,
			expectCount:  "2",
		},
		{
			name:         "metadata persists count in retry fields",
			threshold:    new(3),
			attempt:      2,
			maxAttempts:  10,
			prevHash:     endpointErrHash(500, "err"),
			prevCount:    "1",
			errInput:     &domain.EndpointError{StatusCode: 500, Body: "err"},
			expectStatus: domain.StatusQueued,
			expectCount:  "2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			st := &mockExecutorStore{}
			exec := NewExecutor(ExecutorConfig{
				Pool:         NewPool(10),
				Queue:        &mockExecQueue{},
				Store:        st,
				PollInterval: time.Hour,
			})

			meta := map[string]string{}
			if tc.prevHash != "" {
				meta["_error_hash"] = tc.prevHash
			}
			if tc.prevCount != "" {
				meta["_error_hash_count"] = tc.prevCount
			}

			run := &domain.JobRun{
				ID:       "run-1",
				JobID:    "job-1",
				Attempt:  tc.attempt,
				Metadata: meta,
			}
			job := &domain.Job{
				ID:                  "job-1",
				EndpointURL:         "http://example.com",
				PoisonPillThreshold: tc.threshold,
			}
			policy := executionPolicy{maxAttempts: tc.maxAttempts, timeoutSecs: 30}
			exec.handleFailure(context.Background(), run, job, policy, tc.errInput, nil)

			calls := st.statusUpdates()
			if len(calls) == 0 {
				t.Fatal("expected at least one status update")
			}
			last := calls[len(calls)-1]
			if last.to != tc.expectStatus {
				t.Errorf("expected status %s, got %s", tc.expectStatus, last.to)
			}

			if tc.expectPoisoned {
				errField, _ := last.fields["error"].(string)
				if !strings.Contains(errField, "poison pill detected") {
					t.Errorf("expected 'poison pill detected' in error field, got %q", errField)
				}
			}

			if tc.expectCount != "" {
				meta, ok := last.fields["metadata"].(map[string]string)
				if !ok {
					t.Fatal("expected metadata in fields")
				}
				if meta["_error_hash_count"] != tc.expectCount {
					t.Errorf("expected _error_hash_count=%q, got %q", tc.expectCount, meta["_error_hash_count"])
				}
			}
		})
	}
}

func TestHandleFailure_PoisonPillDetected(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})
	threshold := 3
	errBody := "fail"
	endpointErr := &domain.EndpointError{StatusCode: 500, Body: errBody}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 3, Metadata: map[string]string{
		"_error_hash":       errorHashForError(endpointErr),
		"_error_hash_count": "2",
	}}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: &threshold}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, endpointErr, nil)

	requireLastStatusUpdateTo(t, store.statusUpdates(), domain.StatusDeadLetter)
}

func TestHandleFailure_PoisonPillNotTriggeredOnDifferentError(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})
	threshold := 3
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 3, Metadata: map[string]string{
		"_error_hash":       errorHash("different error"),
		"_error_hash_count": "2",
	}}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: &threshold}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

	requireLastStatusUpdateTo(t, store.statusUpdates(), domain.StatusQueued)
}

func TestHandleFailure_PoisonPillNotTriggeredWhenDisabled(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool:         NewPool(10),
		Queue:        &mockExecQueue{},
		Store:        store,
		PollInterval: time.Hour,
	})
	errMsg := "fail"
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 3, Metadata: map[string]string{
		"_error_hash":       errorHash(errMsg),
		"_error_hash_count": "2",
	}}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com"}
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: errMsg}, nil)

	requireLastStatusUpdateTo(t, store.statusUpdates(), domain.StatusQueued)
}

// Adversarial / edge case tests

func TestPoisonPill_DisabledDoesNotWriteMetadata(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 2,
		Metadata: map[string]string{"user_key": "preserved"},
	}
	// No poison pill threshold set.
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com"}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "err"}, nil)

	calls := st.statusUpdates()
	last := calls[len(calls)-1]
	if last.to != domain.StatusQueued {
		t.Fatalf("expected queued, got %s", last.to)
	}
	// metadata must NOT be in the update fields when poison pill is disabled,
	// otherwise a nil/empty metadata would overwrite existing DB metadata.
	if _, exists := last.fields["metadata"]; exists {
		t.Error("metadata should not be in update fields when poison pill is disabled")
	}
}

func TestPoisonPill_DisabledNilMetadataDoesNotOverwrite(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	// nil metadata with disabled poison pill must not overwrite DB metadata.
	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 2,
		Metadata: nil,
	}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com"}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "err"}, nil)

	calls := st.statusUpdates()
	last := calls[len(calls)-1]
	if _, exists := last.fields["metadata"]; exists {
		t.Error("nil metadata should not be sent to DB when poison pill is disabled")
	}
}

func TestPoisonPill_DLQWithoutPoisonPillDoesNotWriteMetadata(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	// Client error -> DLQ via class, no poison pill involved.
	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 1,
		Metadata: nil,
	}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com"}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 400, Body: "bad"}, nil)

	calls := st.statusUpdates()
	last := calls[len(calls)-1]
	if last.to != domain.StatusDeadLetter {
		t.Fatalf("expected dead_letter, got %s", last.to)
	}
	if _, exists := last.fields["metadata"]; exists {
		t.Error("metadata should not be in DLQ fields when poison pill was not active")
	}
}

func TestPoisonPill_CorruptMetadataCount(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	errBody := "500 internal server error"
	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 3,
		Metadata: map[string]string{
			"_error_hash":       endpointErrHash(500, errBody),
			"_error_hash_count": "not_a_number", // corrupt
		},
	}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: new(3)}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: errBody}, nil)

	calls := st.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected status update")
	}
	last := calls[len(calls)-1]
	// corrupt count should reset to 1, so still below threshold=3 -> retry
	if last.to != domain.StatusQueued {
		t.Errorf("expected queued (retry after corrupt count reset), got %s", last.to)
	}
	meta, _ := last.fields["metadata"].(map[string]string)
	if meta["_error_hash_count"] != "1" {
		t.Errorf("expected count reset to 1, got %q", meta["_error_hash_count"])
	}
}

func TestPoisonPill_NegativeCount(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	errBody := "server error"
	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 3,
		Metadata: map[string]string{
			"_error_hash":       endpointErrHash(500, errBody),
			"_error_hash_count": "-5", // negative
		},
	}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: new(3)}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: errBody}, nil)

	calls := st.statusUpdates()
	last := calls[len(calls)-1]
	// -5 + 1 = -4, still < 3, so retries normally
	if last.to != domain.StatusQueued {
		t.Errorf("expected queued (negative count incremented), got %s", last.to)
	}
	meta, _ := last.fields["metadata"].(map[string]string)
	if meta["_error_hash_count"] != "-4" {
		t.Errorf("expected count -4, got %q", meta["_error_hash_count"])
	}
}

func TestPoisonPill_VeryLargeCount(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	errBody := "error"
	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 3,
		Metadata: map[string]string{
			"_error_hash":       endpointErrHash(500, errBody),
			"_error_hash_count": "2147483646", // near max int32
		},
	}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: new(3)}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: errBody}, nil)

	calls := st.statusUpdates()
	last := calls[len(calls)-1]
	// 2147483647 >= 3, so poison pill triggers
	if last.to != domain.StatusDeadLetter {
		t.Errorf("expected dead_letter (large count exceeds threshold), got %s", last.to)
	}
}

func TestPoisonPill_EmptyErrorString(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	// Empty error string: hash should be deterministic and count should work
	emptyHash := errorHash("")
	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 3,
		Metadata: map[string]string{
			"_error_hash":       emptyHash,
			"_error_hash_count": "2",
		},
	}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: new(3)}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	// Use a fmt error that produces empty Error() string
	exec.handleFailure(context.Background(), run, job, policy, fmt.Errorf(""), nil)

	calls := st.statusUpdates()
	last := calls[len(calls)-1]
	if last.to != domain.StatusDeadLetter {
		t.Errorf("expected dead_letter for empty error hitting threshold, got %s", last.to)
	}
}

func TestPoisonPill_NilMetadataMap(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 1,
		Metadata: nil, // nil map
	}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: new(3)}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "fail"}, nil)

	calls := st.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected status update")
	}
	last := calls[len(calls)-1]
	if last.to != domain.StatusQueued {
		t.Errorf("expected queued (first attempt), got %s", last.to)
	}
	meta, ok := last.fields["metadata"].(map[string]string)
	if !ok {
		t.Fatal("expected metadata to be initialized in fields")
	}
	if meta["_error_hash_count"] != "1" {
		t.Errorf("expected count 1, got %q", meta["_error_hash_count"])
	}
}

func TestPoisonPill_HashCollisionByDesign(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	// Two endpoint errors that share the same first 200 chars of Error() but differ after.
	// "endpoint returned 500: " is 23 chars, so body prefix of 177 chars fills to 200.
	bodyPrefix := strings.Repeat("E", 177)
	errA := bodyPrefix + "AAA"
	errB := bodyPrefix + "BBB"

	// First run with errA
	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 2,
		Metadata: map[string]string{
			"_error_hash":       endpointErrHash(500, errA),
			"_error_hash_count": "1",
		},
	}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: new(3)}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: errB}, nil)

	calls := st.statusUpdates()
	last := calls[len(calls)-1]
	// By design, same first 200 chars = same hash, so count increments
	meta, _ := last.fields["metadata"].(map[string]string)
	if meta["_error_hash_count"] != "2" {
		t.Errorf("expected count 2 (same hash by design), got %q", meta["_error_hash_count"])
	}
}

func TestPoisonPill_MetadataPreservesExistingKeys(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 2,
		Metadata: map[string]string{
			"user_tag": "important",
			"region":   "us-east",
		},
	}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: new(3)}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "err"}, nil)

	calls := st.statusUpdates()
	last := calls[len(calls)-1]
	meta, _ := last.fields["metadata"].(map[string]string)
	if meta["user_tag"] != "important" {
		t.Errorf("expected user_tag preserved, got %q", meta["user_tag"])
	}
	if meta["region"] != "us-east" {
		t.Errorf("expected region preserved, got %q", meta["region"])
	}
	if meta["_error_hash"] == "" {
		t.Error("expected _error_hash to be set")
	}
	if meta["_error_hash_count"] != "1" {
		t.Errorf("expected count 1, got %q", meta["_error_hash_count"])
	}
}

func TestPoisonPill_SameClassDifferentMessage(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	// Previous error was "500: db timeout" (server class)
	// Current error is "500: null pointer" (also server class, different message)
	prevMsg := "500: db timeout"
	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 3,
		Metadata: map[string]string{
			"_error_hash":       endpointErrHash(500, prevMsg),
			"_error_hash_count": "2",
		},
	}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: new(3)}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "500: null pointer"}, nil)

	calls := st.statusUpdates()
	last := calls[len(calls)-1]
	// Different messages -> different hashes -> count resets -> retries
	if last.to != domain.StatusQueued {
		t.Errorf("expected queued (different message resets count), got %s", last.to)
	}
	meta, _ := last.fields["metadata"].(map[string]string)
	if meta["_error_hash_count"] != "1" {
		t.Errorf("expected count reset to 1, got %q", meta["_error_hash_count"])
	}
}

func TestPoisonPill_DLQFieldsCorrect(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	errBody := "500 internal server error"
	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 3,
		Metadata: map[string]string{
			"_error_hash":       endpointErrHash(500, errBody),
			"_error_hash_count": "2",
		},
	}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: new(3)}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: errBody}, nil)

	calls := st.statusUpdates()
	last := calls[len(calls)-1]

	if last.from != domain.StatusExecuting {
		t.Errorf("expected from=executing, got %s", last.from)
	}
	if last.to != domain.StatusDeadLetter {
		t.Errorf("expected to=dead_letter, got %s", last.to)
	}

	errField, _ := last.fields["error"].(string)
	if !strings.Contains(errField, "poison pill detected (same error 3 times)") {
		t.Errorf("expected poison pill message with count, got %q", errField)
	}
	if !strings.Contains(errField, "endpoint returned 500") {
		t.Errorf("expected redacted endpoint status in message, got %q", errField)
	}
	if strings.Contains(errField, errBody) {
		t.Errorf("expected endpoint body to be redacted from message, got %q", errField)
	}

	errClass, _ := last.fields["error_class"].(string)
	if errClass != domain.ErrorClassServer {
		t.Errorf("expected error_class=server, got %q", errClass)
	}

	if _, ok := last.fields["finished_at"]; !ok {
		t.Error("expected finished_at in DLQ fields")
	}

	meta, _ := last.fields["metadata"].(map[string]string)
	if meta["_error_hash"] == "" {
		t.Error("expected _error_hash in DLQ metadata")
	}
	if meta["_error_hash_count"] != "3" {
		t.Errorf("expected _error_hash_count=3 in DLQ metadata, got %q", meta["_error_hash_count"])
	}
}

func TestPoisonPill_DoesNotInterfereWithCircuitBreaker(t *testing.T) {
	t.Parallel()
	var circuitFailureCalled bool
	st := &mockExecutorStore{
		recordFailureFn: func(_ context.Context, _ string, _ time.Time, _ int, _ time.Duration) error {
			circuitFailureCalled = true
			return nil
		},
	}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	errBody := "server error"
	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 3,
		Metadata: map[string]string{
			"_error_hash":       endpointErrHash(500, errBody),
			"_error_hash_count": "2",
		},
	}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: new(3)}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: errBody}, nil)

	// Poison pill triggered DLQ, but circuit breaker should still have been called
	calls := st.statusUpdates()
	if calls[len(calls)-1].to != domain.StatusDeadLetter {
		t.Error("expected dead_letter from poison pill")
	}
	if !circuitFailureCalled {
		t.Error("expected circuit breaker failure to be recorded even when poison pill triggers")
	}
}

func TestPoisonPill_TimeoutBypassesPoisonPill(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	// Run with metadata that would trigger poison pill in handleFailure
	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 3,
		Metadata: map[string]string{
			"_error_hash":       errorHash("execution timed out"),
			"_error_hash_count": "2",
		},
	}
	job := &domain.Job{ID: "job-1", EndpointURL: "http://example.com", PoisonPillThreshold: new(3)}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	// handleTimeout does NOT use poison pill logic
	exec.handleTimeout(context.Background(), run, job, policy, nil)

	calls := st.statusUpdates()
	last := calls[len(calls)-1]
	// Should retry normally, not DLQ
	if last.to != domain.StatusQueued {
		t.Errorf("expected queued (timeout bypasses poison pill), got %s", last.to)
	}
}

func TestPoisonPill_RetryPriorityBoostPreserved(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 2, Priority: 3,
		Metadata: map[string]string{
			"_error_hash":       endpointErrHash(500, "err"),
			"_error_hash_count": "1",
		},
	}
	job := &domain.Job{
		ID: "job-1", EndpointURL: "http://example.com",
		PoisonPillThreshold: new(5),
		RetryPriorityBoost:  2,
	}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: "err"}, nil)

	calls := st.statusUpdates()
	last := calls[len(calls)-1]
	if last.to != domain.StatusQueued {
		t.Fatalf("expected queued, got %s", last.to)
	}

	// Check priority boost is present
	priority, ok := last.fields["priority"].(int)
	if !ok {
		t.Fatal("expected priority field in retry")
	}
	if priority != 5 { // 3 + 2
		t.Errorf("expected priority 5, got %d", priority)
	}

	// Check metadata is also present
	meta, _ := last.fields["metadata"].(map[string]string)
	if meta["_error_hash_count"] != "2" {
		t.Errorf("expected count 2, got %q", meta["_error_hash_count"])
	}
}

// Integration-style tests

func TestPoisonPill_Integration_SameErrorDLQ(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	job := &domain.Job{
		ID: "job-1", EndpointURL: "http://example.com",
		PoisonPillThreshold: new(3),
	}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}
	errBody := "internal database error"
	endpointErr := &domain.EndpointError{StatusCode: 500, Body: errBody}

	// Attempt 1: first failure
	run := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 1,
		Metadata: map[string]string{},
	}
	exec.handleFailure(context.Background(), run, job, policy, endpointErr, nil)
	calls := st.statusUpdates()
	if calls[0].to != domain.StatusQueued {
		t.Fatalf("attempt 1: expected queued, got %s", calls[0].to)
	}
	meta1, _ := calls[0].fields["metadata"].(map[string]string)
	if meta1["_error_hash_count"] != "1" {
		t.Fatalf("attempt 1: expected count=1, got %q", meta1["_error_hash_count"])
	}

	// Attempt 2: second failure with metadata from attempt 1
	run2 := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 2,
		Metadata: map[string]string{
			"_error_hash":       meta1["_error_hash"],
			"_error_hash_count": meta1["_error_hash_count"],
		},
	}
	exec.handleFailure(context.Background(), run2, job, policy, endpointErr, nil)
	calls = st.statusUpdates()
	if calls[1].to != domain.StatusQueued {
		t.Fatalf("attempt 2: expected queued, got %s", calls[1].to)
	}
	meta2, _ := calls[1].fields["metadata"].(map[string]string)
	if meta2["_error_hash_count"] != "2" {
		t.Fatalf("attempt 2: expected count=2, got %q", meta2["_error_hash_count"])
	}

	// Attempt 3: third failure -> should DLQ
	run3 := &domain.JobRun{
		ID: "run-1", JobID: "job-1", Attempt: 3,
		Metadata: map[string]string{
			"_error_hash":       meta2["_error_hash"],
			"_error_hash_count": meta2["_error_hash_count"],
		},
	}
	exec.handleFailure(context.Background(), run3, job, policy, endpointErr, nil)
	calls = st.statusUpdates()
	if calls[2].to != domain.StatusDeadLetter {
		t.Fatalf("attempt 3: expected dead_letter, got %s", calls[2].to)
	}
	errField, _ := calls[2].fields["error"].(string)
	if !strings.Contains(errField, "poison pill detected") {
		t.Errorf("attempt 3: expected poison pill message, got %q", errField)
	}
}

func TestPoisonPill_Integration_VaryingErrorsRetryNormally(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	job := &domain.Job{
		ID: "job-1", EndpointURL: "http://example.com",
		PoisonPillThreshold: new(3),
	}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}

	meta := map[string]string{}
	for i := 1; i <= 5; i++ {
		errBody := fmt.Sprintf("error variant %d", i)
		run := &domain.JobRun{
			ID: "run-1", JobID: "job-1", Attempt: i,
			Metadata: copyMap(meta),
		}
		exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: errBody}, nil)

		calls := st.statusUpdates()
		last := calls[len(calls)-1]
		if last.to != domain.StatusQueued {
			t.Fatalf("attempt %d: expected queued, got %s", i, last.to)
		}
		// Update meta for next iteration
		meta, _ = last.fields["metadata"].(map[string]string)
		if meta["_error_hash_count"] != "1" {
			t.Fatalf("attempt %d: expected count=1 (reset), got %q", i, meta["_error_hash_count"])
		}
	}
}

func TestPoisonPill_Integration_ErrorThenRecoveryThenSameError(t *testing.T) {
	t.Parallel()
	st := &mockExecutorStore{}
	exec := NewExecutor(ExecutorConfig{
		Pool: NewPool(10), Queue: &mockExecQueue{}, Store: st, PollInterval: time.Hour,
	})

	job := &domain.Job{
		ID: "job-1", EndpointURL: "http://example.com",
		PoisonPillThreshold: new(3),
	}
	policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}

	// Sequence: error A x2, error B x1, error A x2
	// Expected counts: 1, 2, 1(reset), 1, 2
	errors := []string{"error A", "error A", "error B", "error A", "error A"}
	expectedCounts := []string{"1", "2", "1", "1", "2"}

	meta := map[string]string{}
	for i, errBody := range errors {
		run := &domain.JobRun{
			ID: "run-1", JobID: "job-1", Attempt: i + 1,
			Metadata: copyMap(meta),
		}
		exec.handleFailure(context.Background(), run, job, policy, &domain.EndpointError{StatusCode: 500, Body: errBody}, nil)

		calls := st.statusUpdates()
		last := calls[len(calls)-1]
		if last.to != domain.StatusQueued {
			t.Fatalf("step %d (%q): expected queued, got %s", i, errBody, last.to)
		}
		meta, _ = last.fields["metadata"].(map[string]string)
		if meta["_error_hash_count"] != expectedCounts[i] {
			t.Fatalf("step %d (%q): expected count=%s, got %q", i, errBody, expectedCounts[i], meta["_error_hash_count"])
		}
	}
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	maps.Copy(out, m)
	return out
}
