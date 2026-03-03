//go:build integration

package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"
)

func AssertRunStatus(t testing.TB, ctx context.Context, s store.Store, runID string, expected domain.RunStatus) {
	t.Helper()

	run, err := s.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun(%q) error = %v", runID, err)
	}

	if run.Status != expected {
		t.Fatalf("run %q status = %q, want %q", runID, run.Status, expected)
	}
}

func AssertRunCompleted(t testing.TB, ctx context.Context, s store.Store, runID string) {
	t.Helper()
	AssertRunStatus(t, ctx, s, runID, domain.StatusCompleted)
}

func AssertRunFailed(t testing.TB, ctx context.Context, s store.Store, runID string) {
	t.Helper()
	AssertRunStatus(t, ctx, s, runID, domain.StatusFailed)
}

func WaitForStatus(t testing.TB, ctx context.Context, s store.Store, runID string, status domain.RunStatus, timeout time.Duration) *domain.JobRun {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		run, err := s.GetRun(ctx, runID)
		if err == nil && run.Status == status {
			return run
		}

		select {
		case <-ctx.Done():
			t.Fatalf("WaitForStatus(%q) canceled waiting for %q: %v", runID, status, ctx.Err())
		case <-deadline.C:
			if err != nil {
				t.Fatalf("WaitForStatus(%q) timed out after %s waiting for %q; last GetRun error: %v", runID, timeout, status, err)
			}
			t.Fatalf("WaitForStatus(%q) timed out after %s: last status = %q, want %q", runID, timeout, run.Status, status)
		case <-ticker.C:
		}
	}
}

func AssertEventExists(t testing.TB, ctx context.Context, s store.Store, runID string, eventType string) {
	t.Helper()

	events, err := s.ListEvents(ctx, runID)
	if err != nil {
		t.Fatalf("ListEvents(%q) error = %v", runID, err)
	}

	for _, event := range events {
		if string(event.Type) == eventType {
			return
		}
	}

	t.Fatalf("expected event type %q for run %q, got %d events", eventType, runID, len(events))
}

func JSONEqual(t testing.TB, a, b []byte) {
	t.Helper()

	var va any
	if err := json.Unmarshal(a, &va); err != nil {
		t.Fatalf("invalid JSON A: %v\nA: %s", err, string(a))
	}

	var vb any
	if err := json.Unmarshal(b, &vb); err != nil {
		t.Fatalf("invalid JSON B: %v\nB: %s", err, string(b))
	}

	ca, err := json.Marshal(va)
	if err != nil {
		t.Fatalf("marshal normalized JSON A: %v", err)
	}
	cb, err := json.Marshal(vb)
	if err != nil {
		t.Fatalf("marshal normalized JSON B: %v", err)
	}

	if !bytes.Equal(ca, cb) {
		t.Fatalf("JSON mismatch:\nA: %s\nB: %s\nNormalized A: %s\nNormalized B: %s", string(a), string(b), string(ca), string(cb))
	}
}

func AssertHTTPStatus(t testing.TB, w *httptest.ResponseRecorder, expected int) {
	t.Helper()

	if w.Code != expected {
		t.Fatalf("HTTP status = %d, want %d; body: %s", w.Code, expected, w.Body.String())
	}
}
