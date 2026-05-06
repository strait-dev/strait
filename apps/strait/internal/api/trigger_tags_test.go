package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestHandleTriggerJob_RejectsOversizedTags(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Enabled:     true,
				TimeoutSecs: 60,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	var body strings.Builder
	body.WriteString(`{"tags":{`)
	for i := 0; i < 21; i++ {
		if i > 0 {
			body.WriteByte(',')
		}
		body.WriteString(`"k`)
		body.WriteByte(byte('a' + i))
		body.WriteString(`":"v"`)
	}
	body.WriteString(`}}`)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body.String(), "proj-1"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "too many tags") {
		t.Fatalf("body = %q, want tag validation error", w.Body.String())
	}
}
