package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
)

func TestHandleReplayRun_EnqueueThrottledReturns429(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:         id,
				JobID:      "job-1",
				ProjectID:  "proj-1",
				Status:     domain.StatusFailed,
				CreatedAt:  now.Add(-time.Minute),
				FinishedAt: &now,
			}, nil
		},
		GetJobFunc: func(context.Context, string) (*domain.Job, error) {
			return &domain.Job{
				ID:          "job-1",
				ProjectID:   "proj-1",
				Name:        "job",
				Slug:        "job",
				Enabled:     true,
				TimeoutSecs: 60,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(context.Context, *domain.JobRun) error {
			return &queue.ThrottledError{ProjectID: "proj-1", RetryAfter: 1500 * time.Millisecond}
		},
	}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/runs/run-1/replay", `{}`, "proj-1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want %q", got, "2")
	}

	var body struct {
		Error APIError `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Error.Code != ErrorCodeEnqueueThrottled {
		t.Fatalf("error code = %q, want %q", body.Error.Code, ErrorCodeEnqueueThrottled)
	}
}
