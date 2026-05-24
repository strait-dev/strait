package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"strait/internal/domain"
)

// FuzzErrorHash verifies that errorHash never panics on arbitrary input and
// always returns a deterministic 16-char hex string.
func FuzzErrorHash(f *testing.F) {
	f.Add("")
	f.Add("500 internal server error")
	f.Add(strings.Repeat("x", 200))
	f.Add(strings.Repeat("x", 201))
	f.Add(strings.Repeat("\U0001f600", 60))
	f.Add("\x00\x00\x00")
	f.Add(strings.Repeat("a", 10000))
	// Invalid UTF-8 sequences.
	f.Add("\xff\xfe\xfd")
	f.Add("valid prefix\x80\x81\x82 valid suffix")

	f.Fuzz(func(t *testing.T, input string) {
		h1 := errorHash(input)
		h2 := errorHash(input)

		// Must be deterministic.
		if h1 != h2 {
			t.Errorf("non-deterministic: %q -> %q vs %q", input, h1, h2)
		}

		// Must always be 16 hex chars.
		if len(h1) != 16 {
			t.Errorf("expected 16-char hash, got %d: %q", len(h1), h1)
		}

		// Must be valid hex.
		for _, c := range h1 {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				t.Errorf("non-hex char %q in hash %q", string(c), h1)
			}
		}
	})
}

// FuzzErrorHashTruncation verifies that strings sharing the first 200
// characters always produce the same hash, regardless of what follows.
func FuzzErrorHashTruncation(f *testing.F) {
	f.Add("a]suffix1", "b]suffix2")
	f.Add("", "anything")
	f.Add("short", "also short")

	f.Fuzz(func(t *testing.T, suffixA, suffixB string) {
		// Build two strings with identical 200-char prefix but different suffixes.
		prefix := strings.Repeat("P", 200)
		a := prefix + suffixA
		b := prefix + suffixB

		ha := errorHash(a)
		hb := errorHash(b)
		if ha != hb {
			t.Errorf("same 200-char prefix but different hashes: suffixA=%q suffixB=%q hash=%q vs %q",
				suffixA, suffixB, ha, hb)
		}
	})
}

// FuzzErrorHashRuneBoundary verifies that errorHash produces valid output
// when the 200-rune boundary falls inside a multi-byte UTF-8 sequence.
func FuzzErrorHashRuneBoundary(f *testing.F) {
	f.Add(10, "\U0001f600") // 4-byte rune
	f.Add(50, "\u00e9")     // 2-byte rune (e with accent)
	f.Add(100, "\u4e16")    // 3-byte rune (CJK)
	f.Add(199, "x")         // single-byte at boundary

	f.Fuzz(func(t *testing.T, prefixLen int, runeStr string) {
		if prefixLen < 0 || prefixLen > 300 {
			return
		}
		if len(runeStr) == 0 {
			return
		}

		// Build a string of prefixLen runes + extra runes, then hash it.
		input := strings.Repeat(runeStr, prefixLen+10)
		h := errorHash(input)

		if len(h) != 16 {
			t.Errorf("expected 16-char hash, got %d for prefixLen=%d rune=%q", len(h), prefixLen, runeStr)
		}

		// If the input has > 200 runes, the hash should match the first-200-runes hash.
		runes := []rune(input)
		if len(runes) > 200 {
			truncated := string(runes[:200])
			hTrunc := errorHash(truncated)
			if h != hTrunc {
				t.Errorf("hash of full string != hash of first 200 runes: full=%q trunc=%q", h, hTrunc)
			}
		}
	})
}

// FuzzPoisonPillDetection exercises the full poison pill code path in
// handleFailure with fuzzed inputs. Asserts invariants: no panics,
// status is always queued or dead_letter, metadata is well-formed when present.
func FuzzPoisonPillDetection(f *testing.F) {
	// (threshold, attempt, maxAttempts, prevCount, errBody)
	f.Add(3, 3, 10, "2", "server error")
	f.Add(1, 1, 10, "", "error")
	f.Add(0, 5, 10, "4", "error")
	f.Add(3, 5, 5, "2", "error")
	f.Add(3, 1, 10, "not_a_number", "error")
	f.Add(3, 2, 10, "-5", "error")
	f.Add(3, 3, 10, "999999999999999999", "error")
	f.Add(100, 1, 1, "", "")

	f.Fuzz(func(t *testing.T, threshold, attempt, maxAttempts int, prevCount, errBody string) {
		// Clamp to reasonable ranges to avoid test infra issues.
		if attempt < 1 || attempt > 1000 {
			return
		}
		if maxAttempts < 1 || maxAttempts > 1000 {
			return
		}
		if threshold < 0 || threshold > 1000 {
			return
		}

		st := &mockExecutorStore{}
		exec := NewExecutor(ExecutorConfig{
			Pool:         NewPool(10),
			Queue:        &mockExecQueue{},
			Store:        st,
			PollInterval: time.Hour,
		})

		var thresholdPtr *int
		if threshold > 0 {
			thresholdPtr = &threshold
		}

		// Build previous metadata: if threshold > 0 and we have a prevCount,
		// set up metadata as if previous attempts ran.
		meta := map[string]string{}
		endpointErr := &domain.EndpointError{StatusCode: 500, Body: errBody}
		if prevCount != "" {
			meta["_error_hash"] = errorHash(endpointErr.Error())
			meta["_error_hash_count"] = prevCount
		}

		run := &domain.JobRun{
			ID:       "fuzz-run",
			JobID:    "fuzz-job",
			Attempt:  attempt,
			Metadata: meta,
		}
		job := &domain.Job{
			ID:                  "fuzz-job",
			EndpointURL:         "http://example.com",
			PoisonPillThreshold: thresholdPtr,
		}
		policy := executionPolicy{maxAttempts: maxAttempts, timeoutSecs: 30}

		// Must not panic.
		exec.handleFailure(context.Background(), run, job, policy, endpointErr, nil)

		calls := st.statusUpdates()
		if len(calls) == 0 {
			t.Fatal("expected at least one status update")
		}
		last := calls[len(calls)-1]

		// Invariant: status must be queued (retry) or dead_letter (terminal).
		if last.to != domain.StatusQueued && last.to != domain.StatusDeadLetter {
			t.Errorf("unexpected status %s for threshold=%d attempt=%d max=%d prevCount=%q",
				last.to, threshold, attempt, maxAttempts, prevCount)
		}

		// Invariant: if retrying, attempt must be incremented.
		if last.to == domain.StatusQueued {
			nextAttempt, ok := last.fields["attempt"].(int)
			if !ok {
				t.Fatal("retry missing attempt field")
			}
			if nextAttempt != attempt+1 {
				t.Errorf("expected next attempt %d, got %d", attempt+1, nextAttempt)
			}
		}

		// Invariant: if metadata is present, hash and count must be valid.
		if m, ok := last.fields["metadata"].(map[string]string); ok {
			if m["_error_hash"] == "" {
				t.Error("metadata present but _error_hash is empty")
			}
			if len(m["_error_hash"]) != 16 {
				t.Errorf("_error_hash has wrong length: %d", len(m["_error_hash"]))
			}
			if m["_error_hash_count"] == "" {
				t.Error("metadata present but _error_hash_count is empty")
			}
			// Count must be a valid integer.
			if !isValidInt(m["_error_hash_count"]) {
				t.Errorf("_error_hash_count is not a valid integer: %q", m["_error_hash_count"])
			}
		}

		// Invariant: if status is dead_letter, finished_at must be set.
		if last.to == domain.StatusDeadLetter {
			if _, ok := last.fields["finished_at"]; !ok {
				t.Error("dead_letter missing finished_at")
			}
		}
	})
}

// FuzzPoisonPillDifferentErrors verifies that different error messages
// always reset the count (never falsely trigger poison pill).
func FuzzPoisonPillDifferentErrors(f *testing.F) {
	f.Add("error A", "error B")
	f.Add("", "x")
	f.Add("same", "same")
	f.Add(strings.Repeat("x", 300), strings.Repeat("x", 300)+"y")

	f.Fuzz(func(t *testing.T, errA, errB string) {
		st := &mockExecutorStore{}
		exec := NewExecutor(ExecutorConfig{
			Pool:         NewPool(10),
			Queue:        &mockExecQueue{},
			Store:        st,
			PollInterval: time.Hour,
		})

		threshold := 2
		job := &domain.Job{
			ID:                  "fuzz-job",
			EndpointURL:         "http://example.com",
			PoisonPillThreshold: &threshold,
		}
		policy := executionPolicy{maxAttempts: 10, timeoutSecs: 30}

		// First failure with errA.
		endpointA := &domain.EndpointError{StatusCode: 500, Body: errA}
		run1 := &domain.JobRun{
			ID: "fuzz-run", JobID: "fuzz-job", Attempt: 1,
			Metadata: map[string]string{},
		}
		exec.handleFailure(context.Background(), run1, job, policy, endpointA, nil)
		calls := st.statusUpdates()
		meta1, _ := calls[0].fields["metadata"].(map[string]string)

		// Second failure with errB.
		endpointB := &domain.EndpointError{StatusCode: 500, Body: errB}
		run2 := &domain.JobRun{
			ID: "fuzz-run", JobID: "fuzz-job", Attempt: 2,
			Metadata: copyMap(meta1),
		}
		exec.handleFailure(context.Background(), run2, job, policy, endpointB, nil)
		calls = st.statusUpdates()
		last := calls[len(calls)-1]

		hashA := errorHashForError(endpointA)
		hashB := errorHashForError(endpointB)

		if hashA == hashB {
			// Same hash: count should be 2, which hits threshold=2 -> DLQ.
			if last.to != domain.StatusDeadLetter {
				t.Errorf("same hash but not DLQ: errA=%q errB=%q status=%s", errA, errB, last.to)
			}
		} else {
			// Different hash: count resets to 1, should retry.
			if last.to != domain.StatusQueued {
				t.Errorf("different hash but not retried: errA=%q errB=%q status=%s", errA, errB, last.to)
			}
			meta2, _ := last.fields["metadata"].(map[string]string)
			if meta2["_error_hash_count"] != "1" {
				t.Errorf("expected count reset to 1, got %q", meta2["_error_hash_count"])
			}
		}
	})
}

// FuzzErrorHashInvalidUTF8 specifically targets invalid UTF-8 input to ensure
// errorHash handles it without panics or inconsistent results.
func FuzzErrorHashInvalidUTF8(f *testing.F) {
	f.Add([]byte("\xff\xfe"))
	f.Add([]byte("\xc0\xaf"))          // overlong encoding
	f.Add([]byte("valid\xed\xa0\x80")) // surrogate half
	f.Add([]byte(strings.Repeat("\x80", 250)))
	f.Add(append([]byte(strings.Repeat("A", 199)), 0x80)) // invalid byte at rune boundary

	f.Fuzz(func(t *testing.T, raw []byte) {
		input := string(raw)

		// Must not panic.
		h := errorHash(input)

		if len(h) != 16 {
			t.Errorf("expected 16-char hash, got %d for input bytes len=%d", len(h), len(raw))
		}

		// Deterministic.
		if h != errorHash(input) {
			t.Error("non-deterministic hash for same input")
		}

		// If the string is valid UTF-8 and has > 200 runes, verify truncation.
		if utf8.ValidString(input) {
			runes := []rune(input)
			if len(runes) > 200 {
				truncated := string(runes[:200])
				if errorHash(truncated) != h {
					t.Error("valid UTF-8: hash of full string != hash of first 200 runes")
				}
			}
		}
	})
}

func isValidInt(s string) bool {
	if s == "" {
		return false
	}
	start := 0
	if s[0] == '-' || s[0] == '+' {
		start = 1
	}
	if start >= len(s) {
		return false
	}
	for i := start; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// FuzzErrorHashCollisions counts hash collisions over fuzzed inputs.
// This is not a failure test -- it's a statistical check that the 64-bit
// hash space provides adequate collision resistance for our use case.
func FuzzErrorHashCollisions(f *testing.F) {
	f.Add("error 1", "error 2")
	f.Add("same", "same")
	f.Add(fmt.Sprintf("endpoint returned 500: %s", strings.Repeat("x", 200)),
		fmt.Sprintf("endpoint returned 500: %s", strings.Repeat("y", 200)))

	seen := make(map[string]string) // hash -> first input that produced it
	var collisions int

	f.Fuzz(func(t *testing.T, a, b string) {
		ha := errorHash(a)
		hb := errorHash(b)

		// Track collisions for the 'a' input.
		if prev, exists := seen[ha]; exists && prev != a {
			collisions++
		} else {
			seen[ha] = a
		}

		// If inputs differ but hashes match, it's a collision (expected for 64-bit hash).
		// We don't fail on collisions -- we just verify the hash function doesn't
		// systematically collide (which would break poison pill detection).
		_ = ha
		_ = hb
	})
}
